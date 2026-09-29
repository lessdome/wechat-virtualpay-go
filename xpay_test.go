package wechat_virtualpay_go

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// xpayRT 是记录用的 RoundTripper：把发出去的请求原样留下来，不做任何加工。
type xpayRT struct {
	mu    sync.Mutex
	calls int
	meth  string
	path  string
	query url.Values
	body  []byte
	ctype string

	status int
	resp   string
}

func (f *xpayRT) RoundTrip(r *http.Request) (*http.Response, error) {
	raw, _ := io.ReadAll(r.Body)

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.meth = r.Method
	f.path = r.URL.Path
	f.query = r.URL.Query()
	f.body = raw
	f.ctype = r.Header.Get("Content-Type")

	st := f.status
	if st == 0 {
		st = http.StatusOK
	}
	return &http.Response{
		StatusCode: st,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(f.resp)),
	}, nil
}

// swapXpay 把包级的基础地址与 client 一起换成测试用的，结束后恢复。
func swapXpay(t *testing.T, rt *xpayRT) {
	t.Helper()
	oldBase, oldClient := xpayAPIBase, xpayHTTPClient
	xpayAPIBase = "http://xpay.test"
	xpayHTTPClient = &http.Client{Transport: rt, Timeout: 5 * time.Second}
	t.Cleanup(func() { xpayAPIBase, xpayHTTPClient = oldBase, oldClient })
}

// testPaySig 独立算一遍 pay_sig——不复用 CalcPaySig，免得用被测代码给自己签发通行证。
func testPaySig(appKey, uri, body string) string {
	mac := hmac.New(sha256.New, []byte(appKey))
	mac.Write([]byte(uri + "&" + body))
	return hex.EncodeToString(mac.Sum(nil))
}

// testSignature 独立算一遍用户态签名。与 testPaySig 一样刻意不复用被测代码——区别只在
// 原文：signature 只签请求体，不拼 uri。
func testSignature(sessionKey, body string) string {
	mac := hmac.New(sha256.New, []byte(sessionKey))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

// bodyEnv 取出请求体顶层，校验它是 JSON 对象并返回。
func bodyEnv(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		t.Fatalf("请求体不是 JSON 对象: %s", raw)
	}
	return obj
}

// ---------------------------------------------------------------------------
// requestBody：env 的兜底
// ---------------------------------------------------------------------------

// 请求结构体不填 env 时，发出去的 body 里必须带着 env=0（现网）——官方标必填，而 env
// 这个字段的零值正是现网。第一例走的就是兜底那条路：结构体里根本没有 env 字段，也得
// 补上，将来新加的请求结构体要是忘了声明，靠的就是它。
func TestRequestBodyEnvDefaultsToZero(t *testing.T) {
	cases := []struct {
		name string
		req  any
		want string
	}{
		{"没有 env 字段的结构体（走兜底）", struct{}{}, `{"env":0}`},
		{
			"query_order",
			QueryOrderRequest{OpenID: "o1", OrderID: "x1"},
			`{"openid":"o1","env":0,"order_id":"x1"}`,
		},
		{
			"只有一个字段的请求",
			QueryDownloadOrderRequest{TaskID: "t1"},
			`{"task_id":"t1","env":0}`,
		},
		{
			"显式填了沙箱，兜底不许覆盖它",
			QueryOrderRequest{Env: 1, OpenID: "o1", OrderID: "x1"},
			`{"openid":"o1","env":1,"order_id":"x1"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := requestBody(c.req)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("请求体不对\n实际: %s\n期望: %s", got, c.want)
			}
		})
	}
}

// 环境是**真发出去**的：填了沙箱，body 里就得是 1，而且 pay_sig 要盖住这份带 env=1 的
// 字节——签名与请求体始终是同一份，换个环境不需要另算签名。
func TestSandboxEnvIsSentAndSigned(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0,"order":{}}`}
	swapXpay(t, rt)

	if _, err := QueryOrder(context.Background(), "T", "K",
		QueryOrderRequest{Env: 1, OpenID: "o", OrderID: "x"}); err != nil {
		t.Fatal(err)
	}
	if got := bodyEnv(t, rt.body)["env"]; got != float64(1) {
		t.Fatalf("env 应当是 1，实际 %v：%s", got, rt.body)
	}
	if want := testPaySig("K", "/xpay/query_order", string(rt.body)); rt.query.Get("pay_sig") != want {
		t.Fatalf("pay_sig 与带 env=1 的请求体对不上\n实际: %s\n期望: %s\n请求体: %s",
			rt.query.Get("pay_sig"), want, rt.body)
	}
}

// 不是 JSON 对象的输入必须硬失败。放过去就会发出一个 body 与签名语义对不上的请求。
func TestRequestBodyRejectsNonObject(t *testing.T) {
	for _, req := range []any{nil, 42, "s", []int{1, 2}, (*QueryOrderRequest)(nil)} {
		if got, err := requestBody(req); err == nil {
			t.Errorf("%#v 不是 JSON 对象，应当报错，却得到 %s", req, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 请求形状：地址、query、请求体、签名
// ---------------------------------------------------------------------------

// 钉住那条最要紧的不变量：**签名原文 = uri + "&" + 真正发出去的请求体**。
//
// 这里在测试里独立算一遍 HMAC，再和 query 上的 pay_sig 比——只要包内部把请求体二次
// 序列化（换了字段顺序、多一次转义），或者 uri 上多带了 query string，这条就会红。
func TestPaySigMatchesSentBody(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0,"order":{"status":4,"left_fee":100}}`}
	swapXpay(t, rt)

	if _, err := QueryOrder(context.Background(), "TOKEN", "APPKEY",
		QueryOrderRequest{OpenID: "o1", OrderID: "x1"}); err != nil {
		t.Fatal(err)
	}

	if rt.meth != http.MethodPost {
		t.Errorf("应当用 POST，实际 %s", rt.meth)
	}
	if rt.path != "/xpay/query_order" {
		t.Errorf("路径不对: %s", rt.path)
	}
	if rt.ctype != "application/json" {
		t.Errorf("Content-Type 不对: %s", rt.ctype)
	}
	if got := rt.query.Get("access_token"); got != "TOKEN" {
		t.Errorf("access_token 不对: %s", got)
	}
	if got := rt.query.Get("signature"); got != "" {
		t.Errorf("这一级不该有 signature，实际 %s", got)
	}

	want := testPaySig("APPKEY", "/xpay/query_order", string(rt.body))
	if got := rt.query.Get("pay_sig"); got != want {
		t.Fatalf("pay_sig 与发出去的请求体对不上\n实际: %s\n期望: %s\n请求体: %s", got, want, rt.body)
	}
	// 签名原文里的 uri 不含 "?" 及其后部分：把整个 URL 拿去签就会得出另一个值。
	if other := testPaySig("APPKEY", "/xpay/query_order?"+rt.query.Encode(), string(rt.body)); other == rt.query.Get("pay_sig") {
		t.Error("uri 里似乎带上了 query string")
	}
}

// 这一档一个签名都不带：notify_provide_goods 的官方参数表里只有 access_token。
func TestNotifyProvideGoodsHasNoPaySig(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0}`}
	swapXpay(t, rt)

	resp, err := NotifyProvideGoods(context.Background(), "TOKEN",
		NotifyProvideGoodsRequest{WxOrderID: "wx1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ErrCode != 0 {
		t.Errorf("这一趟应当成功，实际 errcode=%d（%s）", resp.ErrCode, resp.ErrMsg)
	}
	if rt.path != "/xpay/notify_provide_goods" {
		t.Errorf("路径不对: %s", rt.path)
	}
	if got := rt.query.Get("pay_sig"); got != "" {
		t.Errorf("这一级不该有 pay_sig，实际 %s", got)
	}
	if got := rt.query.Get("access_token"); got != "TOKEN" {
		t.Errorf("access_token 不对: %s", got)
	}
}

// ---------------------------------------------------------------------------
// 三档鉴权：三个 PostXxx 各带哪些签名
// ---------------------------------------------------------------------------

// probeResponse 是测试用的最小响应结构体：只内嵌公共头，够满足 xpayResponse 约束。
// 单独立一个，是为了这一节不跟着订单模块的字段变。
type probeResponse struct{ ResponseHeader }

// 三档的差别只在 query 里带哪个签名。这里钉的不是「签名值算得对不对」，而是**集合本身**：
// 该带的一个不少，不该带的一个不多。多带一个微信不会报错（多余的 query 参数它不看），
// 但那说明「这个接口要不要用户签名」被弄错了，而错的那天没人会发现。
//
// 表按凭据从少到多排：只带凭证 → 加支付签名 → 再加用户态签名。
func TestPostLevelsSendExactlyTheirSignatures(t *testing.T) {
	const (
		uri        = "/xpay/query_order"
		accessTok  = "TOKEN"
		appKey     = "APPKEY"
		sessionKey = "SESSIONKEY"
	)
	cases := []struct {
		name string
		call func() error
		want string // 期望出现在 query 里的签名参数，逗号分隔
	}{
		{
			"只带调用凭证",
			func() error {
				var out probeResponse
				return PostTokenOnly(context.Background(), accessTok, uri, struct{}{}, &out)
			},
			"",
		},
		{
			"凭证 + 支付签名",
			func() error {
				var out probeResponse
				return PostWithPaySig(context.Background(), accessTok, appKey, uri, struct{}{}, &out)
			},
			"pay_sig",
		},
		{
			"凭证 + 支付签名 + 用户态签名",
			func() error {
				var out probeResponse
				return PostWithUserSig(context.Background(), accessTok, appKey, sessionKey, uri, struct{}{}, &out)
			},
			"pay_sig,signature",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{}`}
			swapXpay(t, rt)
			if err := c.call(); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, k := range []string{"pay_sig", "signature"} {
				if rt.query.Has(k) {
					got = append(got, k)
				}
			}
			if strings.Join(got, ",") != c.want {
				t.Errorf("签名参数不对\n实际: %v\n期望: %s", got, c.want)
			}
			// 凭证本身每一档都得在，而且不许被签名顶掉。
			if rt.query.Get("access_token") != accessTok {
				t.Errorf("access_token 丢了: %q", rt.query.Get("access_token"))
			}
		})
	}
}

// 两把钥匙各签各的：pay_sig 用 appKey 且**拼上 uri**，signature 用 sessionKey 且**不拼**。
// 这两条搞混（signature 也拼了 uri，或者两把钥匙传反），微信只回一个签名错误码，而那个码
// 要真机才看得见——所以在本地把原文的形状钉死。
func TestPostWithUserSigSignsWithBothKeys(t *testing.T) {
	const (
		uri        = "/xpay/currency_pay"
		appKey     = "APPKEY"
		sessionKey = "SESSIONKEY"
	)
	rt := &xpayRT{resp: `{}`}
	swapXpay(t, rt)

	var out probeResponse
	req := struct {
		OpenID string `json:"openid"`
	}{OpenID: "o1"}
	if err := PostWithUserSig(context.Background(), "TOKEN", appKey, sessionKey, uri, req, &out); err != nil {
		t.Fatal(err)
	}

	body := string(rt.body)
	if got, want := rt.query.Get("pay_sig"), testPaySig(appKey, uri, body); got != want {
		t.Errorf("pay_sig 不对\n实际: %s\n期望: %s", got, want)
	}
	if got, want := rt.query.Get("signature"), testSignature(sessionKey, body); got != want {
		t.Errorf("signature 不对\n实际: %s\n期望: %s", got, want)
	}
	// signature 要是也把 uri 签进去了，它的值就该等于这一串。
	if banned := testSignature(sessionKey, uri+"&"+body); rt.query.Get("signature") == banned {
		t.Error("signature 把 uri 也签进去了——官方算法里 signature 只签请求体")
	}
}

// 不填 env 时，5 个接口的请求体里都必须是 env=0——包括那个不带签名的。
func TestAllOrderEndpointsSendEnvZero(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0,"order":{},"task_id":"t"}`}
	swapXpay(t, rt)

	provided := true
	calls := map[string]func() error{
		"query_order": func() error {
			_, err := QueryOrder(context.Background(), "T", "K", QueryOrderRequest{OpenID: "o", OrderID: "x"})
			return err
		},
		"refund_order": func() error {
			_, err := RefundOrder(context.Background(), "T", "K", RefundOrderRequest{
				OpenID: "o", OrderID: "x", RefundOrderID: "R1234567",
				LeftFee: 100, RefundFee: 100,
				RefundReason: RefundReasonOther, RefundFrom: RefundFromUser,
			})
			return err
		},
		"notify_provide_goods": func() error {
			_, err := NotifyProvideGoods(context.Background(), "T", NotifyProvideGoodsRequest{OrderID: "x"})
			return err
		},
		"start_download_order": func() error {
			_, err := StartDownloadOrder(context.Background(), "T", "K", StartDownloadOrderRequest{
				BeginDs: 20260420, EndDs: 20260420, OrderType: DownloadOrderCoin,
				IsProvided: &provided, PayChannel: PayChannelNormal,
			})
			return err
		},
		"query_download_order": func() error {
			_, err := QueryDownloadOrder(context.Background(), "T", "K", QueryDownloadOrderRequest{TaskID: "t"})
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			rt.calls = 0
			if err := call(); err != nil {
				t.Fatal(err)
			}
			if rt.calls != 1 {
				t.Fatalf("应当只发一次请求，实际 %d", rt.calls)
			}
			env, ok := bodyEnv(t, rt.body)["env"]
			if !ok {
				t.Fatalf("请求体里没有 env: %s", rt.body)
			}
			if env != float64(0) {
				t.Fatalf("env 应当固定为 0，实际 %v：%s", env, rt.body)
			}
			if rt.path != "/xpay/"+name {
				t.Fatalf("路径不对: %s", rt.path)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 响应解析与错误
// ---------------------------------------------------------------------------

func TestQueryOrderParsesOrder(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok","order":{"status":4,"left_fee":30,"env_type":1,"order_type":0}}`}
	swapXpay(t, rt)

	resp, err := QueryOrder(context.Background(), "T", "K", QueryOrderRequest{OpenID: "o", OrderID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Order == nil {
		t.Fatal("order 应当解析出来")
	}
	if resp.Order.Status != OrderStatusProvided || resp.Order.LeftFee != 30 || resp.Order.EnvType != OrderEnvTypeProduction {
		t.Fatalf("字段没填对: %+v", resp.Order)
	}
	// 公共头是**内嵌**的匿名字段：包外要能像自己的字段一样读（resp.ErrCode），否则
	// 「err == nil 不等于成功」这条契约就没法履行。
	if resp.ErrCode != 0 || resp.ErrMsg != "ok" {
		t.Errorf("公共头也要原值带出来，实际 errcode=%d errmsg=%q", resp.ErrCode, resp.ErrMsg)
	}
}

// 查不到订单：微信回一个没有 order 的响应。不是错误，Order 为 nil 留给调用方判断。
func TestQueryOrderNotFoundIsNotAnError(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok"}`}
	swapXpay(t, rt)

	resp, err := QueryOrder(context.Background(), "T", "K", QueryOrderRequest{OpenID: "o", OrderID: "x"})
	if err != nil {
		t.Fatalf("查不到不该报错: %v", err)
	}
	if resp.Order != nil {
		t.Fatalf("查不到时 Order 应当是 nil，实际 %+v", resp.Order)
	}
}

// 业务失败**不是 error**：errcode/errmsg 原值随响应结构体带出来，err 是 nil。
//
// 这条钉的是整套设计的关键一步，也是最容易踩空的地方——谁哪天又加回「errcode 非 0 就
// 返回 error」，这里会红。断言码与 errmsg 的**原值**，不匹配文案：文案是本包写的、随时
// 可改，码和 errmsg 是微信给的、一个字都不能丢。
func TestBusinessErrorStaysInResponse(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":268490003,"errmsg":"pay sig error"}`}
	swapXpay(t, rt)

	resp, err := QueryOrder(context.Background(), "T", "K", QueryOrderRequest{OpenID: "o", OrderID: "x"})
	if err != nil {
		t.Fatalf("业务失败不该变成 error——微信的错误响应也是合法 JSON，会被正常解析: %v", err)
	}
	if resp.ErrCode != 268490003 {
		t.Errorf("errcode 必须是微信给的原值 268490003，实际 %d", resp.ErrCode)
	}
	if resp.ErrMsg != "pay sig error" {
		t.Errorf("errmsg 必须是原值 %q，实际 %q", "pay sig error", resp.ErrMsg)
	}
	if resp.Order != nil {
		t.Errorf("失败响应里不该有 order，实际 %+v", resp.Order)
	}
}

// access_token 失效的那几个码（40001/40014/42001）**也一视同仁**：原值带出，本包不做任
// 何判断。换不换 token、要不要重发是调用方的策略——本包连「这是不是令牌问题」都不替它
// 认：认了就等于把「该怎么办」也一起揽过来了。
func TestTokenErrorsAreNotSpecialCased(t *testing.T) {
	for _, code := range []int{40001, 40014, 42001} {
		rt := &xpayRT{resp: `{"errcode":` + strconv.Itoa(code) + `,"errmsg":"invalid credential"}`}
		swapXpay(t, rt)

		resp, err := QueryOrder(context.Background(), "T", "K", QueryOrderRequest{OpenID: "o", OrderID: "x"})
		if err != nil {
			t.Fatalf("errcode=%d 也是业务失败，不该变成 error: %v", code, err)
		}
		if resp.ErrCode != code {
			t.Errorf("errcode 应当是原值 %d，实际 %d", code, resp.ErrCode)
		}
	}
}

// 非 200：这时响应体不是微信的报文（可能是网关的），没有 errcode 可交给调用方，所以
// 这是**本包的错误**（err != nil），只能给一句文案。状态码、接口名、原始响应都要在里
// 面——网关的 502 响应里常常写着真正的原因。
func TestHTTPErrorIsPlainError(t *testing.T) {
	rt := &xpayRT{status: 502, resp: `{"errcode":-1,"errmsg":"system error"}`}
	swapXpay(t, rt)

	// 响应这里用 _ 接住：报错时它必然是零值（每个方法都写死 return X{}, err），这一点
	// 从方法外面断言是**证伪不了**的，所以交给下面直接测 xpaySend 的那条。
	_, err := QueryOrder(context.Background(), "T", "K", QueryOrderRequest{OpenID: "o", OrderID: "x"})
	if err == nil {
		t.Fatal("非 200 必须报错：这种响应里没有微信的 errcode 能给调用方看")
	}
	for _, want := range []string{"502", "system error", "/xpay/query_order"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("文案里应当有 %q，实际: %v", want, err)
		}
	}
}

// 失败时返回 **nil** 响应。用处不是「顺便能判个 nil」，而是让**漏看 err 的人当场发现**
// ——碰 nil 响应立刻 panic；要是返回值类型，他会拿到一个 errcode=0 的零值响应，而那恰好
// 是本包契约里「成功」的样子，一路静默错下去。
//
// 11 个方法各写各的 `return nil, err`，漏一个就少一个护栏，所以逐个过一遍；失败源取三种：
// 参数没过本地校验（一个字节都没发出去）、非 200、网络失败。
func TestErrorReturnsNilResponse(t *testing.T) {
	provided := true
	calls := map[string]func(token string) (isNil bool, err error){
		"QueryOrder": func(tk string) (bool, error) {
			r, err := QueryOrder(context.Background(), tk, "K", QueryOrderRequest{OpenID: "o", OrderID: "x"})
			return r == nil, err
		},
		"RefundOrder": func(tk string) (bool, error) {
			r, err := RefundOrder(context.Background(), tk, "K", RefundOrderRequest{
				OpenID: "o", OrderID: "x", RefundOrderID: "R1234567",
				LeftFee: 100, RefundFee: 100, RefundReason: RefundReasonOther, RefundFrom: RefundFromUser,
			})
			return r == nil, err
		},
		"NotifyProvideGoods": func(tk string) (bool, error) {
			r, err := NotifyProvideGoods(context.Background(), tk, NotifyProvideGoodsRequest{OrderID: "x"})
			return r == nil, err
		},
		"StartDownloadOrder": func(tk string) (bool, error) {
			r, err := StartDownloadOrder(context.Background(), tk, "K", StartDownloadOrderRequest{
				BeginDs: 20260420, EndDs: 20260420, OrderType: DownloadOrderCoin,
				IsProvided: &provided, PayChannel: PayChannelNormal,
			})
			return r == nil, err
		},
		"QueryDownloadOrder": func(tk string) (bool, error) {
			r, err := QueryDownloadOrder(context.Background(), tk, "K", QueryDownloadOrderRequest{TaskID: "t"})
			return r == nil, err
		},
		// 代币类 4 个。三个用户态接口的 appKey/sessionKey 是常量，注入的失败源只有 token。
		"QueryUserBalance": func(tk string) (bool, error) {
			r, err := QueryUserBalance(context.Background(), tk, "K", "S",
				QueryUserBalanceRequest{OpenID: "o", UserIP: "1.2.3.4"})
			return r == nil, err
		},
		"CurrencyPay": func(tk string) (bool, error) {
			r, err := CurrencyPay(context.Background(), tk, "K", "S", CurrencyPayRequest{
				OpenID: "o", UserIP: "1.2.3.4", Amount: 100, OrderID: "c1"})
			return r == nil, err
		},
		"CancelCurrencyPay": func(tk string) (bool, error) {
			r, err := CancelCurrencyPay(context.Background(), tk, "K", "S", CancelCurrencyPayRequest{
				OpenID: "o", UserIP: "1.2.3.4", PayOrderID: "c1", OrderID: "r1", Amount: 100})
			return r == nil, err
		},
		"PresentCurrency": func(tk string) (bool, error) {
			r, err := PresentCurrency(context.Background(), tk,
				PresentCurrencyRequest{OpenID: "o", OrderID: "p1", Amount: 5})
			return r == nil, err
		},
		// 账单类 2 个：它们没有 env，但**有** appKey（pay_sig 照算），所以 appKey 那一路
		// 校验也在这两个改动里能测到。
		"DownloadBill": func(tk string) (bool, error) {
			r, err := DownloadBill(context.Background(), tk, "K",
				DownloadBillRequest{BeginDs: 20230801, EndDs: 20230810})
			return r == nil, err
		},
		"DownloadIOSBill": func(tk string) (bool, error) {
			r, err := DownloadIOSBill(context.Background(), tk, "K",
				DownloadIOSBillRequest{StartMonth: "202601", EndMonth: "202603"})
			return r == nil, err
		},
	}

	t.Run("参数校验没过（没发请求）", func(t *testing.T) {
		for name, call := range calls {
			isNil, err := call("")
			if err == nil {
				t.Errorf("%s: accessToken 为空应当报错", name)
			}
			if !isNil {
				t.Errorf("%s: 校验失败时响应必须是 nil", name)
			}
		}
	})

	t.Run("非 200", func(t *testing.T) {
		swapXpay(t, &xpayRT{status: 502, resp: `{"errcode":-1,"errmsg":"system error"}`})

		for name, call := range calls {
			isNil, err := call("T")
			if err == nil {
				t.Errorf("%s: 非 200 应当报错", name)
			}
			if !isNil {
				t.Errorf("%s: 非 200 时响应必须是 nil", name)
			}
		}
	})

	t.Run("网络失败", func(t *testing.T) {
		oldBase, oldClient := xpayAPIBase, xpayHTTPClient
		xpayAPIBase = "http://xpay.test"
		xpayHTTPClient = &http.Client{Transport: errRT{err: context.DeadlineExceeded}}
		t.Cleanup(func() { xpayAPIBase, xpayHTTPClient = oldBase, oldClient })

		for name, call := range calls {
			isNil, err := call("T")
			if err == nil {
				t.Errorf("%s: 网络失败应当报错", name)
			}
			if !isNil {
				t.Errorf("%s: 网络失败时响应必须是 nil", name)
			}
		}
	})
}

// out 必须是响应结构体的指针，而且**传 nil 也必须报错**——不能静默。
//
// 静默就是最坏的结果：调用方拿到一个零值响应，而零值响应在本包契约里恰好等于「成功」
// （errcode=0）。传值的写法已经被泛型 *T 挡在编译期了，所以这里只能测 nil 这一路。
func TestXpaySendRejectsNilOut(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok"}`}
	swapXpay(t, rt)

	var resp *QueryOrderResponse // nil
	err := xpaySend(context.Background(), "T", "/xpay/query_order", []byte(`{}`), url.Values{}, resp)
	if err == nil {
		t.Fatal("out 是 nil 时必须报错，否则调用方会拿着一个零值响应当成功")
	}
	if !strings.Contains(err.Error(), "/xpay/query_order") {
		t.Errorf("文案里要说清是哪个接口，实际: %v", err)
	}
}

// 非 200 时**不许动 out**。这话在方法那一层断不出来（报错时方法返回的是零值，半填的
// 局部变量到不了调用方），所以直接把传输层拎出来测：给它一个**合法 JSON** 的 502 响应
// 体，看它会不会把网关的 errcode 解析进调用方的响应里——那样调用方手里就会有一个
// 「既报错、又带着一个来路不明的 errcode」的结构，比什么都不给更坏。
func TestXpaySendLeavesOutUntouchedOnHTTPError(t *testing.T) {
	rt := &xpayRT{status: 502, resp: `{"errcode":-1,"errmsg":"system error"}`}
	swapXpay(t, rt)

	var out QueryOrderResponse
	err := xpaySend(context.Background(), "T", "/xpay/query_order", []byte(`{}`), url.Values{}, &out)
	if err == nil {
		t.Fatal("非 200 必须报错")
	}
	if out != (QueryOrderResponse{}) {
		t.Errorf("非 200 的报文不许解析进 out（那是网关的，不是微信的），实际 %+v", out)
	}
}

// 网络失败（连不上、超时）：**底层原值必须活着**——errors.Is 要能认出
// context.DeadlineExceeded 这类标准库错误。否则调用方判断「是不是超时」又得回到对文案做
// 子串匹配，那是最脆的一种判断。
func TestTransportErrorKeepsCause(t *testing.T) {
	oldBase, oldClient := xpayAPIBase, xpayHTTPClient
	xpayAPIBase = "http://xpay.test"
	xpayHTTPClient = &http.Client{Transport: errRT{err: context.DeadlineExceeded}}
	t.Cleanup(func() { xpayAPIBase, xpayHTTPClient = oldBase, oldClient })

	_, err := QueryOrder(context.Background(), "T", "K", QueryOrderRequest{OpenID: "o", OrderID: "x"})
	if err == nil {
		t.Fatal("网络失败必须报错")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("底层原值要能被 errors.Is 认出来（用 %%w 包住它），实际: %v", err)
	}
	if !strings.Contains(err.Error(), "/xpay/query_order") {
		t.Errorf("失败的是哪个接口要能看出来，实际: %v", err)
	}
}

// errRT 是一个只会失败的 RoundTripper，用来把网络层错误注入进来。
type errRT struct{ err error }

func (e errRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

// ---------------------------------------------------------------------------
// 本地校验：不合法时一个字节都不该发出去
// ---------------------------------------------------------------------------

// uri 是唯一一个**裸着进 URL** 的调用方入参（其余入参要么进请求体，要么被 url.Values
// 转义），所以形态由本包把着。三种拼错的形态都要在**发出去之前**拦下来——它们都会让
// 微信回一个跟真正原因对不上的错误码（见 checkURI）。
func TestPostRejectsBadURIWithoutRequest(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want string
	}{
		{"空", "", "uri 不能为空"},
		{"少了开头的斜杠", "xpay/query_order", "必须以"},
		{"带了 query", "/xpay/query_order?foo=1", `不能含 "?"`},
		{"带了 fragment", "/xpay/query_order#x", `不能含 "#"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			var out NotifyProvideGoodsResponse
			err := PostWithPaySig(context.Background(), "T", "K", c.uri, struct{}{}, &out)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("期望报错含 %q，实际: %v", c.want, err)
			}
			if rt.calls != 0 {
				t.Fatalf("uri 不合法却发出了 %d 次请求", rt.calls)
			}
		})
	}
}

func TestOrderEndpointsRejectBadInputWithoutRequest(t *testing.T) {
	ctx := context.Background()
	provided := true
	okDownload := StartDownloadOrderRequest{
		BeginDs: 20260420, EndDs: 20260420, OrderType: DownloadOrderCoin,
		IsProvided: &provided, PayChannel: PayChannelNormal,
	}
	okRefund := RefundOrderRequest{
		OpenID: "o", OrderID: "x", RefundOrderID: "R1234567",
		LeftFee: 100, RefundFee: 100, RefundReason: RefundReasonOther, RefundFrom: RefundFromUser,
	}
	// dl 以「一份合法的下载请求」为底，只改坏其中一个字段——这样每条用例失败时
	// 报错必然是它想验的那条规则，而不是别的字段先被拦。
	dl := func(mut func(*StartDownloadOrderRequest)) StartDownloadOrderRequest {
		r := okDownload
		mut(&r)
		return r
	}

	cases := []struct {
		name string
		want string
		call func() error
	}{
		{"缺 accessToken", "accessToken", func() error {
			_, err := QueryOrder(ctx, "", "K", QueryOrderRequest{OpenID: "o", OrderID: "x"})
			return err
		}},
		{"缺 appKey", "appKey", func() error {
			_, err := QueryOrder(ctx, "T", "", QueryOrderRequest{OpenID: "o", OrderID: "x"})
			return err
		}},
		{"env 取值非法", "Env 2 非法", func() error {
			_, err := QueryOrder(ctx, "T", "K", QueryOrderRequest{Env: 2, OpenID: "o", OrderID: "x"})
			return err
		}},
		{"沙箱下缺 appKey，报错要指明该配沙箱 key", "沙箱 AppKey", func() error {
			_, err := QueryOrder(ctx, "T", "", QueryOrderRequest{Env: 1, OpenID: "o", OrderID: "x"})
			return err
		}},
		{"QueryOrder 缺 OpenID", "OpenID", func() error {
			_, err := QueryOrder(ctx, "T", "K", QueryOrderRequest{OrderID: "x"})
			return err
		}},
		{"QueryOrder 两个单号都没传", "二选一", func() error {
			_, err := QueryOrder(ctx, "T", "K", QueryOrderRequest{OpenID: "o"})
			return err
		}},
		{"QueryOrder 两个单号都传了", "只能传一个", func() error {
			_, err := QueryOrder(ctx, "T", "K", QueryOrderRequest{OpenID: "o", OrderID: "x", WxOrderID: "w"})
			return err
		}},
		{"退款单号太短", "RefundOrderID", func() error {
			r := okRefund
			r.RefundOrderID = "R123456"
			_, err := RefundOrder(ctx, "T", "K", r)
			return err
		}},
		{"退款单号含非法字符", "RefundOrderID", func() error {
			r := okRefund
			r.RefundOrderID = "R1234567*"
			_, err := RefundOrder(ctx, "T", "K", r)
			return err
		}},
		{"退款金额超过剩余可退", "LeftFee", func() error {
			r := okRefund
			r.RefundFee = 101
			_, err := RefundOrder(ctx, "T", "K", r)
			return err
		}},
		{"退款金额为 0", "LeftFee", func() error {
			r := okRefund
			r.RefundFee = 0
			_, err := RefundOrder(ctx, "T", "K", r)
			return err
		}},
		{"退款原因非法", "RefundReason", func() error {
			r := okRefund
			r.RefundReason = "9"
			_, err := RefundOrder(ctx, "T", "K", r)
			return err
		}},
		{"退款来源非法", "RefundFrom", func() error {
			r := okRefund
			r.RefundFrom = "0"
			_, err := RefundOrder(ctx, "T", "K", r)
			return err
		}},
		{"发货通知两个单号都没传", "二选一", func() error {
			_, err := NotifyProvideGoods(ctx, "T", NotifyProvideGoodsRequest{})
			return err
		}},
		{"日期不是合法日期", "BeginDs", func() error {
			_, err := StartDownloadOrder(ctx, "T", "K", dl(func(r *StartDownloadOrderRequest) {
				r.BeginDs = 20261320
			}))
			return err
		}},
		{"结束日期早于开始日期", "早于", func() error {
			_, err := StartDownloadOrder(ctx, "T", "K", dl(func(r *StartDownloadOrderRequest) {
				r.EndDs = 20260419
			}))
			return err
		}},
		{"日期跨度超过 31 天", "31 天", func() error {
			_, err := StartDownloadOrder(ctx, "T", "K", dl(func(r *StartDownloadOrderRequest) {
				r.EndDs = 20260522
			}))
			return err
		}},
		{"订单类型非法", "OrderType", func() error {
			_, err := StartDownloadOrder(ctx, "T", "K", dl(func(r *StartDownloadOrderRequest) {
				r.OrderType = 5
			}))
			return err
		}},
		{"道具订单没传发货状态", "IsProvided", func() error {
			_, err := StartDownloadOrder(ctx, "T", "K", dl(func(r *StartDownloadOrderRequest) {
				r.OrderType = DownloadOrderGoods
				r.IsProvided = nil
			}))
			return err
		}},
		{"非退款订单却筛退款状态", "RefundStatus", func() error {
			_, err := StartDownloadOrder(ctx, "T", "K", dl(func(r *StartDownloadOrderRequest) {
				r.RefundStatus = RefundStatusFilterRefunded
			}))
			return err
		}},
		{"退款状态取值非法", "RefundStatus", func() error {
			_, err := StartDownloadOrder(ctx, "T", "K", dl(func(r *StartDownloadOrderRequest) {
				r.OrderType = DownloadOrderRefund
				r.RefundStatus = 3
			}))
			return err
		}},
		{"支付渠道非法", "PayChannel", func() error {
			_, err := StartDownloadOrder(ctx, "T", "K", dl(func(r *StartDownloadOrderRequest) {
				r.PayChannel = 3
			}))
			return err
		}},
		{"缺 TaskID", "TaskID", func() error {
			_, err := QueryDownloadOrder(ctx, "T", "K", QueryDownloadOrderRequest{})
			return err
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			err := c.call()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("期望报错含 %q，实际: %v", c.want, err)
			}

			// 校验失败只有一句给人读的文案，没有类型可断言——这是**故意的**：本包不产生
			// 错误类型，调用方也就不必为了处理失败去记「有几种类型」。所以上面那句对文案
			// 的子串匹配不是偷懒，是这里唯一能断言的东西；真正要钉住的是下面这个：
			// 校验拦下来时，一个字节都不该发出去。
			if rt.calls != 0 {
				t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
			}
		})
	}
}

// 合法输入的边界必须放行——否则上面那张表可能只是「什么都拦」。
func TestStartDownloadOrderDateRangeBoundary(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0,"task_id":"t"}`}
	swapXpay(t, rt)
	provided := true

	for _, tc := range []struct {
		name         string
		begin, end   int64
		wantRejected bool
	}{
		{"同一天", 20260420, 20260420, false},
		{"正好 31 天", 20260420, 20260521, false},
		{"32 天", 20260420, 20260522, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := StartDownloadOrder(context.Background(), "T", "K", StartDownloadOrderRequest{
				BeginDs: tc.begin, EndDs: tc.end, OrderType: DownloadOrderCoin,
				IsProvided: &provided, PayChannel: PayChannelNormal,
			})
			if tc.wantRejected && err == nil {
				t.Fatal("应当被拦下来")
			}
			if !tc.wantRejected && err != nil {
				t.Fatalf("应当放行，实际: %v", err)
			}
		})
	}
}
