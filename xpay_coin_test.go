package wechat_virtualpay

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// 代币类 9 个结构体的文档字段表比对：字段名 / 类型 / **行序**，一项不多一项不少。
//
// 用的是 xpay_order_test.go 的 orderDocFields 与 notify_test.go 的 fieldSpec（那张表
// 的语义就是「结构体字段表 = 文档表的一行行」，与类无关，别在本文件里再造一份）。
// 更不能把这些用例并进 TestOrderFieldsCoverDoc：那边结尾的 `total != 70` 是它那 11 个
// 结构体的账，混进来两个数字就都失去意义了。
//
// ⚠️ 这张表钉的是**本包的抄写**，钉不住「有没有抄错官方页面」——两边都是我们写下的。
// 它的价值在于：谁改动了 tag、类型或顺序，必须连同这张表一起改，改动因此是显式的、
// 要签字画押的（比如 payitem，见下面那一行的注释）。
func TestCoinFieldsCoverDoc(t *testing.T) {
	tStr := reflect.TypeOf("")
	tI64 := reflect.TypeOf(int64(0))
	tInt := reflect.TypeOf(int(0))
	tBool := reflect.TypeOf(false)

	cases := []struct {
		what string
		val  any
		doc  []fieldSpec
	}{
		{"QueryUserBalanceRequest", QueryUserBalanceRequest{}, []fieldSpec{
			{"openid", tStr}, {"user_ip", tStr}, {"env", tInt},
		}},
		{"QueryUserBalanceResponse", QueryUserBalanceResponse{}, []fieldSpec{
			// 公共头。它在结构体里是**内嵌**的匿名字段，提升后就是直接可见的字段，所以
			// 也进这张表（每个响应表都有这两行，不是只有某一个有）。
			//
			// ⚠️ 放在**最前**是照官方响应体样例（errcode/errmsg 打头）取的，没有逐页核对
			// 过字段表的行序。若某页的响应表把这两行放在表尾，就把对应结构体里的
			// ResponseHeader 也挪到最后——顺序是照文档抄的，这条不算例外。
			{"errcode", tInt}, {"errmsg", tStr},
			{"balance", tI64}, {"present_balance", tI64},
			{"sum_save", tI64}, {"sum_present", tI64},
			{"sum_balance", tI64}, {"sum_cost", tI64},
			// ⚠️ 类型列写 boolean，说明列却写「0:不满足。1:满足」。这里钉的是「本包按类型列
			// 取了 bool」这个决定——改了它就是改了决定，不是改一行抄写。
			{"first_save_flag", tBool},
		}},
		{"PayItem", PayItem{}, []fieldSpec{
			{"productid", tStr}, {"unit_price", tI64}, {"quantity", tI64},
		}},
		{"CurrencyPayRequest", CurrencyPayRequest{}, []fieldSpec{
			{"openid", tStr}, {"user_ip", tStr}, {"amount", tI64}, {"order_id", tStr},
			// ⚠️ 单数、无下划线，照官方字段表。它的生成函数却叫 MarshalPayItems（复数）
			// ——两者不一致。若官方改版或实测报「字段非法」，第一件要试的是改成 pay_items，
			// 改的时候连这行一起改（这一行存在的意义就是让那次改动必须显式发生）。
			{"payitem", tStr},
			{"remark", tStr}, {"env", tInt},
		}},
		{"CurrencyPayResponse", CurrencyPayResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
			{"order_id", tStr}, {"balance", tI64}, {"used_present_amount", tI64},
		}},
		{"CancelCurrencyPayRequest", CancelCurrencyPayRequest{}, []fieldSpec{
			{"openid", tStr}, {"user_ip", tStr},
			{"pay_order_id", tStr}, {"order_id", tStr},
			{"amount", tI64}, {"env", tInt},
		}},
		{"CancelCurrencyPayResponse", CancelCurrencyPayResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"order_id", tStr},
		}},
		{"PresentCurrencyRequest", PresentCurrencyRequest{}, []fieldSpec{
			// 没有 user_ip：赠送不是用户自己发起的动作，另外三个用户态接口有，它没有。
			{"openid", tStr}, {"order_id", tStr}, {"amount", tI64}, {"env", tInt},
		}},
		{"PresentCurrencyResponse", PresentCurrencyResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
			{"balance", tI64}, {"order_id", tStr}, {"present_balance", tI64},
		}},
	}

	total := 0
	for _, c := range cases {
		got := orderDocFields(reflect.TypeOf(c.val))
		total += len(c.doc)

		byName := make(map[string]reflect.Type, len(got))
		for _, f := range got {
			byName[f.name] = f.typ
		}
		for _, want := range c.doc {
			typ, ok := byName[want.name]
			if !ok {
				t.Errorf("%s 缺文档里的字段 %q", c.what, want.name)
				continue
			}
			if typ != want.typ {
				t.Errorf("%s 的字段 %q 类型不对：结构体是 %s，文档是 %s", c.what, want.name, typ, want.typ)
			}
		}
		inDoc := make(map[string]bool, len(c.doc))
		for _, f := range c.doc {
			inDoc[f.name] = true
		}
		for _, f := range got {
			if !inDoc[f.name] {
				t.Errorf("%s 多出文档里没有的字段 %q（%s）", c.what, f.name, f.typ)
			}
		}

		// **行序**也要对：官方字段表是有序的，结构体照表逐行抄下来，读代码时才能一眼
		// 对上文档。spec 本身就是按文档行序写的，直接拿它当期望。
		wantOrder := make([]string, 0, len(c.doc))
		for _, f := range c.doc {
			wantOrder = append(wantOrder, f.name)
		}
		gotOrder := make([]string, 0, len(got))
		for _, f := range got {
			gotOrder = append(gotOrder, f.name)
		}
		if !reflect.DeepEqual(gotOrder, wantOrder) {
			t.Errorf("%s 的字段顺序与文档字段表不一致\n文档: %v\n结构体: %v", c.what, wantOrder, gotOrder)
		}
	}
	if total != 45 {
		t.Fatalf("上面 %d 个结构体的文档字段总数应为 45，实际 %d（期望集合可能抄漏）", len(cases), total)
	}
}

// 代币类的 4 个请求结构体也都要有 env，而且同样是**裸 int**（理由见
// TestOrderRequestsExposeEnv：值域只有两个数，包一层类型挡不住任何东西）。
//
// 与订单类分开写、不并进那一张表：那是订单类的账，将来哪一类少一个字段，分开的表指得
// 出是哪一类。
func TestCoinRequestsExposeEnv(t *testing.T) {
	want := reflect.TypeOf(int(0))
	requests := map[string]any{
		"QueryUserBalanceRequest":  QueryUserBalanceRequest{},
		"CurrencyPayRequest":       CurrencyPayRequest{},
		"CancelCurrencyPayRequest": CancelCurrencyPayRequest{},
		"PresentCurrencyRequest":   PresentCurrencyRequest{},
	}
	for name, req := range requests {
		got := map[string]reflect.Type{}
		collectJSONFieldTypes(reflect.TypeOf(req), got)
		typ, ok := got["env"]
		if !ok {
			t.Errorf("%s 缺 env 字段（官方标必填，零值即现网，必须显式带上）", name)
			continue
		}
		if typ != want {
			t.Errorf("%s 的 env 应当是 %s，实际 %s", name, want, typ)
		}
	}
}

// 四个接口各自的**档位**：前三个是用户态接口，pay_sig 与 signature 都得有，而且各签各的
// 原文；present_currency 一个签名都不带。
//
// 钉的是「集合本身」与「原文的形状」，不是签名值算得对不对（那由 sign_test.go 的官方
// 样例管）。多带一个签名微信不会报错（多余的 query 参数它不看），少带一个却会让这一趟
// 白白失败，而**少带 signature 时 err 仍然是 nil**——所以只能从 query 上看，看不了 err。
//
// 按官方 query 参数表实现：present_currency 只有 access_token。若真机回 268490003，就把
// 它的调用改成 PostWithPaySig，那时这一条也要跟着改（正是它让那次改动留痕）。
func TestCoinEndpointsSendTheirSignatures(t *testing.T) {
	const (
		accessTok  = "TOKEN"
		appKey     = "APPKEY"
		sessionKey = "SESSIONKEY"
	)
	ctx := context.Background()

	userSig := []struct {
		name string
		uri  string
		call func() error
	}{
		{"query_user_balance", "/xpay/query_user_balance", func() error {
			_, err := QueryUserBalance(ctx, accessTok, appKey, sessionKey,
				QueryUserBalanceRequest{OpenID: "o", UserIP: "1.2.3.4"})
			return err
		}},
		{"currency_pay", "/xpay/currency_pay", func() error {
			_, err := CurrencyPay(ctx, accessTok, appKey, sessionKey, CurrencyPayRequest{
				OpenID: "o", UserIP: "1.2.3.4", Amount: 100, OrderID: "c1"})
			return err
		}},
		{"cancel_currency_pay", "/xpay/cancel_currency_pay", func() error {
			_, err := CancelCurrencyPay(ctx, accessTok, appKey, sessionKey, CancelCurrencyPayRequest{
				OpenID: "o", UserIP: "1.2.3.4", PayOrderID: "c1", OrderID: "r1", Amount: 100})
			return err
		}},
	}
	for _, c := range userSig {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			if err := c.call(); err != nil {
				t.Fatal(err)
			}
			if rt.path != c.uri {
				t.Errorf("路径不对: %s", rt.path)
			}
			if got := rt.query.Get("access_token"); got != accessTok {
				t.Errorf("access_token 不对: %s", got)
			}

			body := string(rt.body)
			if got, want := rt.query.Get("pay_sig"), testPaySig(appKey, c.uri, body); got != want {
				t.Errorf("pay_sig 不对（原文是 uri + & + 请求体）\n实际: %s\n期望: %s", got, want)
			}
			if got, want := rt.query.Get("signature"), testSignature(sessionKey, body); got != want {
				t.Errorf("signature 不对（原文只有请求体，不拼 uri）\n实际: %s\n期望: %s", got, want)
			}
			// signature 要是也把 uri 签进去了，它的值就该等于这一串。
			if banned := testSignature(sessionKey, c.uri+"&"+body); rt.query.Get("signature") == banned {
				t.Error("signature 把 uri 也签进去了——官方算法里 signature 只签请求体")
			}
		})
	}

	t.Run("present_currency 一个签名都不带", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0}`}
		swapXpay(t, rt)

		if _, err := PresentCurrency(ctx, accessTok,
			PresentCurrencyRequest{OpenID: "o", OrderID: "p1", Amount: 5}); err != nil {
			t.Fatal(err)
		}
		if rt.path != "/xpay/present_currency" {
			t.Errorf("路径不对: %s", rt.path)
		}
		if rt.query.Has("pay_sig") {
			t.Errorf("这一档不该有 pay_sig，实际 %s", rt.query.Get("pay_sig"))
		}
		if rt.query.Has("signature") {
			t.Errorf("这一档不该有 signature，实际 %s", rt.query.Get("signature"))
		}
		if got := rt.query.Get("access_token"); got != accessTok {
			t.Errorf("access_token 不对: %s", got)
		}
	})
}

// env 是真发出去的：不填时四个接口的请求体里都得是 0（现网），填了 1 就得是 1，而且
// 签名要盖住那份带 env=1 的字节——签名与请求体始终是同一份，换个环境不需要另算签名。
//
// 订单类只钉了 0（TestAllOrderEndpointsSendEnvZero），这里连沙箱那一半一起钉：兜底那条路
// （requestBody 补 env）只保证「有 env」，保不住「调用方填的 1 没被兜底覆盖成 0」。
func TestAllCoinEndpointsSendEnv(t *testing.T) {
	ctx := context.Background()
	// 每个接口以「一份合法请求」为底，只改 Env——失败时必然是这条规则先被触发。
	calls := map[string]struct {
		uri  string
		call func(env int) error
	}{
		"query_user_balance": {"/xpay/query_user_balance", func(env int) error {
			_, err := QueryUserBalance(ctx, "T", "K", "S",
				QueryUserBalanceRequest{OpenID: "o", UserIP: "1.2.3.4", Env: env})
			return err
		}},
		"currency_pay": {"/xpay/currency_pay", func(env int) error {
			_, err := CurrencyPay(ctx, "T", "K", "S", CurrencyPayRequest{
				OpenID: "o", UserIP: "1.2.3.4", Amount: 100, OrderID: "c1", Env: env})
			return err
		}},
		"cancel_currency_pay": {"/xpay/cancel_currency_pay", func(env int) error {
			_, err := CancelCurrencyPay(ctx, "T", "K", "S", CancelCurrencyPayRequest{
				OpenID: "o", UserIP: "1.2.3.4", PayOrderID: "c1", OrderID: "r1", Amount: 100, Env: env})
			return err
		}},
		"present_currency": {"/xpay/present_currency", func(env int) error {
			_, err := PresentCurrency(ctx, "T",
				PresentCurrencyRequest{OpenID: "o", OrderID: "p1", Amount: 5, Env: env})
			return err
		}},
	}

	for name, c := range calls {
		t.Run(name, func(t *testing.T) {
			for _, want := range []float64{0, 1} {
				rt := &xpayRT{resp: `{"errcode":0}`}
				swapXpay(t, rt)

				if err := c.call(int(want)); err != nil {
					t.Fatal(err)
				}
				if rt.calls != 1 {
					t.Fatalf("应当只发一次请求，实际 %d", rt.calls)
				}
				env, ok := bodyEnv(t, rt.body)["env"]
				if !ok {
					t.Fatalf("请求体里没有 env: %s", rt.body)
				}
				if env != want {
					t.Fatalf("env 应当是 %v，实际 %v：%s", want, env, rt.body)
				}
				if rt.path != c.uri {
					t.Fatalf("路径不对: %s", rt.path)
				}
				// 签名盖住的必须就是这份带 env 的字节。present_currency 没有签名，跳过。
				if !rt.query.Has("pay_sig") {
					continue
				}
				if got, want := rt.query.Get("pay_sig"), testPaySig("K", c.uri, string(rt.body)); got != want {
					t.Fatalf("pay_sig 与带 env=%v 的请求体对不上\n实际: %s\n期望: %s", env, got, want)
				}
			}
		})
	}
}

// MarshalPayItems 的输出要**逐字节**钉住：它生成的那段字符串会原样进请求体，微信那边
// 再解一次 JSON。用 json.Unmarshal 后比对象是不够的——紧凑与带空格的两种写法解出来一样，
// 但只有一种是我们真正发出去的（这份字节还要参与签名）。
func TestMarshalPayItemsBytes(t *testing.T) {
	const one = `[{"productid":"A","unit_price":100,"quantity":2}]`
	const two = `[{"productid":"A","unit_price":100,"quantity":2},` +
		`{"productid":"B","unit_price":30,"quantity":1}]`

	t.Run("空参给空数组，不是 null", func(t *testing.T) {
		got, err := MarshalPayItems()
		if err != nil {
			t.Fatal(err)
		}
		if got != "[]" {
			t.Errorf("不传参数应当得到 []（json.Marshal 对 nil 切片给的是 null，那是我们要避开的）\n实际: %s", got)
		}
	})

	t.Run("一项", func(t *testing.T) {
		got, err := MarshalPayItems(PayItem{ProductID: "A", UnitPrice: 100, Quantity: 2})
		if err != nil {
			t.Fatal(err)
		}
		if got != one {
			t.Errorf("实际: %s\n期望: %s", got, one)
		}
	})

	t.Run("两项，顺序即传参顺序", func(t *testing.T) {
		got, err := MarshalPayItems(
			PayItem{ProductID: "A", UnitPrice: 100, Quantity: 2},
			PayItem{ProductID: "B", UnitPrice: 30, Quantity: 1})
		if err != nil {
			t.Fatal(err)
		}
		if got != two {
			t.Errorf("实际: %s\n期望: %s", got, two)
		}
	})

	// 物品 ID 里有引号 / 反斜杠 / 尖括号 / & 时也得能原样还原：payitem 是一段**字符串
	// 值**，微信解一次 JSON 就拿到这些字符本身。转义（含 & 被写成 Unicode 转义）是
	// 正确的、且是全包统一的序列化口径——这条防止有人为了「看起来干净」去关掉它。
	t.Run("特殊字符能原样还原", func(t *testing.T) {
		item := PayItem{ProductID: `A"B\C&D<E`, UnitPrice: 1, Quantity: 1}
		s, err := MarshalPayItems(item)
		if err != nil {
			t.Fatal(err)
		}
		var back []PayItem
		if err := json.Unmarshal([]byte(s), &back); err != nil {
			t.Fatalf("生成的 payitem 自己就该是合法 JSON: %v（%s）", err, s)
		}
		if !reflect.DeepEqual(back, []PayItem{item}) {
			t.Errorf("解回来与原值不一致\n实际: %+v\n期望: %+v", back, item)
		}

		// 再套一层：进到请求体里也还能原样还原（payitem 是被当成**字符串值**编码的）。
		raw, err := requestBody(CurrencyPayRequest{
			OpenID: "o", UserIP: "1.2.3.4", Amount: 1, OrderID: "c1", PayItem: s})
		if err != nil {
			t.Fatal(err)
		}
		var outer struct {
			PayItem string `json:"payitem"`
		}
		if err := json.Unmarshal(raw, &outer); err != nil {
			t.Fatal(err)
		}
		if outer.PayItem != s {
			t.Errorf("请求体里的 payitem 与生成的不一致\n实际: %s\n期望: %s", outer.PayItem, s)
		}
	})
}

// 本包序列化出来的 query_user_balance 请求体，与官方《签名详解》里那段参考脚本的
// post_body 必须是**同一个对象**（只差空白：官方样例的冒号后面带空格）。
//
// 这条把两头接上了：sign_test.go 用 docBody 钉住了签名算法，这里钉住「我们真实发出去的
// 字节就是那个 docBody 的形状」——中间只要有人改了 tag 或加了 omitempty，签名样例仍然
// 是对的，但它已经不再是我们要发的那个请求了。
func TestQueryUserBalanceBodyMatchesOfficialSample(t *testing.T) {
	got, err := requestBody(QueryUserBalanceRequest{OpenID: "xxx", UserIP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"openid":"xxx","user_ip":"127.0.0.1","env":0}`
	if string(got) != want {
		t.Errorf("请求体不对\n实际: %s\n期望: %s", got, want)
	}

	var gotAny, docAny any
	if err := json.Unmarshal(got, &gotAny); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(docBody), &docAny); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotAny, docAny) {
		t.Errorf("与官方参考脚本的 post_body 不是同一个对象\n本包: %s\n官方: %s", got, docBody)
	}
}

// 四个请求体的**字节**形状：键序照结构体声明顺序（也就是官方字段表行序），可选字段为空
// 时整个键都不出现。
//
// 逐字节比对而不是解成 map 再比——map 比不出 omitempty：`"remark":""` 与不含 remark 在
// map 上都能比过，而发出去的是两份不同的字节、签名也随之不同。payitem 更是如此：空串
// 送过去不是一个「无害的省略」，是一个参数错误。
func TestCoinRequestBodies(t *testing.T) {
	t.Run("query_user_balance", func(t *testing.T) {
		got, err := requestBody(QueryUserBalanceRequest{OpenID: "o", UserIP: "1.2.3.4"})
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"openid":"o","user_ip":"1.2.3.4","env":0}`
		if string(got) != want {
			t.Errorf("实际: %s\n期望: %s", got, want)
		}
	})

	t.Run("currency_pay 带明细", func(t *testing.T) {
		items, err := MarshalPayItems(PayItem{ProductID: "A", UnitPrice: 100, Quantity: 2})
		if err != nil {
			t.Fatal(err)
		}
		got, err := requestBody(CurrencyPayRequest{
			OpenID: "o", UserIP: "1.2.3.4", Amount: 100, OrderID: "c1",
			PayItem: items, Remark: "首充礼包",
		})
		if err != nil {
			t.Fatal(err)
		}
		// payitem 是**字符串值**：里面那层引号要再转义一次，用 strconv.Quote 拼出期望
		// 文本（内容全是 ASCII，与 JSON 的转义规则一致），比手写一串 \" 好读也好核。
		want := `{"openid":"o","user_ip":"1.2.3.4","amount":100,"order_id":"c1",` +
			`"payitem":` + strconv.Quote(items) + `,"remark":"首充礼包","env":0}`
		if string(got) != want {
			t.Errorf("实际: %s\n期望: %s", got, want)
		}
	})

	t.Run("currency_pay 不带明细与备注时，两个键都不出现", func(t *testing.T) {
		got, err := requestBody(CurrencyPayRequest{
			OpenID: "o", UserIP: "1.2.3.4", Amount: 100, OrderID: "c1"})
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"openid":"o","user_ip":"1.2.3.4","amount":100,"order_id":"c1","env":0}`
		if string(got) != want {
			t.Errorf("实际: %s\n期望: %s（payitem/remark 靠 omitempty 整个键省掉，不是发空串）", got, want)
		}
	})

	t.Run("cancel_currency_pay", func(t *testing.T) {
		got, err := requestBody(CancelCurrencyPayRequest{
			OpenID: "o", UserIP: "1.2.3.4", PayOrderID: "c1", OrderID: "r1", Amount: 100})
		if err != nil {
			t.Fatal(err)
		}
		// pay_order_id 在 order_id **前面**：一个是原单、一个是本次退款单，发出去之后
		// 就只剩这份字节了，行序与文档一致才核得回来。
		const want = `{"openid":"o","user_ip":"1.2.3.4","pay_order_id":"c1","order_id":"r1","amount":100,"env":0}`
		if string(got) != want {
			t.Errorf("实际: %s\n期望: %s", got, want)
		}
	})

	t.Run("present_currency 没有 user_ip", func(t *testing.T) {
		got, err := requestBody(PresentCurrencyRequest{OpenID: "o", OrderID: "p1", Amount: 5})
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"openid":"o","order_id":"p1","amount":5,"env":0}`
		if string(got) != want {
			t.Errorf("实际: %s\n期望: %s", got, want)
		}
	})
}

// 本地校验：不合法的输入一个字节都不该发出去。**故意的**那几条不查的也要有正面的用例
// 守着——否则日后有人「顺手」补上一条 UserIP 正则，这里不会红。
func TestCoinValidate(t *testing.T) {
	ctx := context.Background()

	// 每个接口一份合法请求，只改坏其中一个字段——失败时必然是它想验的那条规则先触发。
	okBalance := QueryUserBalanceRequest{OpenID: "o", UserIP: "1.2.3.4"}
	okPay := CurrencyPayRequest{OpenID: "o", UserIP: "1.2.3.4", Amount: 100, OrderID: "c1"}
	okCancel := CancelCurrencyPayRequest{OpenID: "o", UserIP: "1.2.3.4", PayOrderID: "c1", OrderID: "r1", Amount: 100}
	okPresent := PresentCurrencyRequest{OpenID: "o", OrderID: "p1", Amount: 5}

	t.Run("不合法", func(t *testing.T) {
		cases := []struct {
			name string
			want string
			call func() error
		}{
			{"缺 accessToken", "accessToken 不能为空", func() error {
				_, err := QueryUserBalance(ctx, "", "K", "S", okBalance)
				return err
			}},
			{"缺 appKey", "appKey 不能为空", func() error {
				_, err := QueryUserBalance(ctx, "T", "", "S", okBalance)
				return err
			}},
			{"沙箱下缺 appKey，报错要指明该配沙箱 key", "沙箱 AppKey", func() error {
				r := okBalance
				r.Env = 1
				_, err := QueryUserBalance(ctx, "T", "", "S", r)
				return err
			}},
			{"缺 sessionKey", "sessionKey 不能为空", func() error {
				_, err := QueryUserBalance(ctx, "T", "K", "", okBalance)
				return err
			}},
			{"env 取值非法", "Env 2 非法", func() error {
				r := okBalance
				r.Env = 2
				_, err := QueryUserBalance(ctx, "T", "K", "S", r)
				return err
			}},
			{"QueryUserBalance 缺 OpenID", "OpenID 不能为空", func() error {
				r := okBalance
				r.OpenID = ""
				_, err := QueryUserBalance(ctx, "T", "K", "S", r)
				return err
			}},
			{"QueryUserBalance 缺 UserIP", "UserIP 不能为空", func() error {
				r := okBalance
				r.UserIP = ""
				_, err := QueryUserBalance(ctx, "T", "K", "S", r)
				return err
			}},
			{"CurrencyPay 缺 OpenID", "OpenID 不能为空", func() error {
				r := okPay
				r.OpenID = ""
				_, err := CurrencyPay(ctx, "T", "K", "S", r)
				return err
			}},
			{"CurrencyPay 缺 UserIP", "UserIP 不能为空", func() error {
				r := okPay
				r.UserIP = ""
				_, err := CurrencyPay(ctx, "T", "K", "S", r)
				return err
			}},
			// 官方把本接口所有字段的必填列都标成「否」，我们按实际语义拦——这两条就是
			// 那次判断的落点（子串取「本单」是为了与 PayOrderID 的报错区分开）。
			{"CurrencyPay 缺 OrderID", "OrderID 不能为空（本单", func() error {
				r := okPay
				r.OrderID = ""
				_, err := CurrencyPay(ctx, "T", "K", "S", r)
				return err
			}},
			{"CurrencyPay 金额为 0", "Amount 必须大于 0", func() error {
				r := okPay
				r.Amount = 0
				_, err := CurrencyPay(ctx, "T", "K", "S", r)
				return err
			}},
			{"CurrencyPay 金额为负", "Amount 必须大于 0", func() error {
				r := okPay
				r.Amount = -1
				_, err := CurrencyPay(ctx, "T", "K", "S", r)
				return err
			}},
			{"缺 sessionKey（CurrencyPay）", "sessionKey 不能为空", func() error {
				_, err := CurrencyPay(ctx, "T", "K", "", okPay)
				return err
			}},
			{"缺 sessionKey（CancelCurrencyPay）", "sessionKey 不能为空", func() error {
				_, err := CancelCurrencyPay(ctx, "T", "K", "", okCancel)
				return err
			}},
			{"CancelCurrencyPay 缺 PayOrderID", "PayOrderID 不能为空（原代币支付单号", func() error {
				r := okCancel
				r.PayOrderID = ""
				_, err := CancelCurrencyPay(ctx, "T", "K", "S", r)
				return err
			}},
			{"CancelCurrencyPay 缺 OrderID", "OrderID 不能为空（本次退款单", func() error {
				r := okCancel
				r.OrderID = ""
				_, err := CancelCurrencyPay(ctx, "T", "K", "S", r)
				return err
			}},
			{"CancelCurrencyPay 金额为 0", "Amount 必须大于 0", func() error {
				r := okCancel
				r.Amount = 0
				_, err := CancelCurrencyPay(ctx, "T", "K", "S", r)
				return err
			}},
			{"PresentCurrency 缺 OpenID", "OpenID 不能为空", func() error {
				r := okPresent
				r.OpenID = ""
				_, err := PresentCurrency(ctx, "T", r)
				return err
			}},
			{"PresentCurrency 缺 OrderID", "OrderID 不能为空", func() error {
				r := okPresent
				r.OrderID = ""
				_, err := PresentCurrency(ctx, "T", r)
				return err
			}},
			{"PresentCurrency 金额为 0", "Amount 必须大于 0", func() error {
				r := okPresent
				r.Amount = 0
				_, err := PresentCurrency(ctx, "T", r)
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
				// 校验失败只有一句给人读的文案，没有类型可断言——这是**故意的**（本包不产生
				// 错误类型）。所以子串匹配不是偷懒，是这里唯一能断言的东西；真正要钉住的是
				// 下面这个：一个字节都不该发出去。
				if rt.calls != 0 {
					t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
				}
			})
		}
	})

	// 上面那张表可能只是「什么都拦」，所以合法输入必须逐条放行。这里挑的是几个**刻意
	// 不查**的边界：UserIP 的格式、Quantity/UnitPrice 为 0、Amount 恰好为 1（下限是 0
	// 不是 1）。它们没有别的护栏，只有这几条用例挡着「顺手加约束」。
	t.Run("合法输入必须放行（含刻意不查的那几条）", func(t *testing.T) {
		cases := []struct {
			name string
			call func() error
		}{
			{"UserIP 不是 IP 也放行（不查格式）", func() error {
				_, err := QueryUserBalance(ctx, "T", "K", "S",
					QueryUserBalanceRequest{OpenID: "o", UserIP: "not-an-ip"})
				return err
			}},
			{"Amount 为 1 放行（下限是 0，不是 1）", func() error {
				_, err := CurrencyPay(ctx, "T", "K", "S", CurrencyPayRequest{
					OpenID: "o", UserIP: "1.2.3.4", Amount: 1, OrderID: "c1"})
				return err
			}},
			{"明细里的单价与数量为 0 也放行（不查取值范围）", func() error {
				items, err := MarshalPayItems(PayItem{ProductID: "A", UnitPrice: 0, Quantity: 0})
				if err != nil {
					return err
				}
				_, err = CurrencyPay(ctx, "T", "K", "S", CurrencyPayRequest{
					OpenID: "o", UserIP: "1.2.3.4", Amount: 1, OrderID: "c1", PayItem: items})
				return err
			}},
			{"单号随便什么形态都放行（不查格式）", func() error {
				_, err := CancelCurrencyPay(ctx, "T", "K", "S", CancelCurrencyPayRequest{
					OpenID: "o", UserIP: "1.2.3.4", PayOrderID: "a b/c", OrderID: "d", Amount: 1})
				return err
			}},
			{"沙箱 + 沙箱 key 放行", func() error {
				r := okPay
				r.Env = 1
				_, err := CurrencyPay(ctx, "T", "K", "S", r)
				return err
			}},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				rt := &xpayRT{resp: `{"errcode":0}`}
				swapXpay(t, rt)

				if err := c.call(); err != nil {
					t.Fatalf("应当放行，实际: %v", err)
				}
				if rt.calls != 1 {
					t.Fatalf("应当正好发一次请求，实际 %d", rt.calls)
				}
			})
		}
	})
}

// 响应反序列化：字段读得出来、int64 不吃精度、first_save_flag 两种取值都对，以及
// **业务失败不是 error**（err == nil、响应原值带出）——最后这条是本包契约的根，其它
// 接口各有一份，代币类也得有。
func TestCoinResponseParsing(t *testing.T) {
	ctx := context.Background()

	t.Run("query_user_balance 的全部字段", func(t *testing.T) {
		// balance 取 30 亿：超过 int32 的范围，抄成 int 在 64 位上照样能过，所以这里真正
		// 钉住 int64 的是 TestCoinFieldsCoverDoc；这条钉的是**值**读得出来。
		rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok","balance":3000000000,` +
			`"present_balance":10,"sum_save":1,"sum_present":2,"sum_balance":3,"sum_cost":4,` +
			`"first_save_flag":true}`}
		swapXpay(t, rt)

		resp, err := QueryUserBalance(ctx, "T", "K", "S",
			QueryUserBalanceRequest{OpenID: "o", UserIP: "1.2.3.4"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.ErrCode != 0 || resp.ErrMsg != "ok" {
			t.Errorf("公共头没读出来: errcode=%d errmsg=%q", resp.ErrCode, resp.ErrMsg)
		}
		if resp.Balance != 3000000000 {
			t.Errorf("balance 不对: %d", resp.Balance)
		}
		if resp.PresentBalance != 10 || resp.SumSave != 1 || resp.SumPresent != 2 ||
			resp.SumBalance != 3 || resp.SumCost != 4 {
			t.Errorf("某个统计字段没读出来: %+v", resp)
		}
		if !resp.FirstSaveFlag {
			t.Error("first_save_flag 应当是 true")
		}
	})

	t.Run("first_save_flag 为 false 时别读成默认值", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"first_save_flag":false}`}
		swapXpay(t, rt)

		resp, err := QueryUserBalance(ctx, "T", "K", "S",
			QueryUserBalanceRequest{OpenID: "o", UserIP: "1.2.3.4"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.FirstSaveFlag {
			t.Error("first_save_flag 应当是 false")
		}
	})

	// errcode 非 0 时**不是 error**：响应原值返回，调用方自己看 errcode。码与 errmsg 都
	// 断言**原值**，不匹配文案：文案是本包写的、随时可改，码是微信给的、一个字都不能丢。
	t.Run("业务失败留在响应里", func(t *testing.T) {
		calls := map[string]struct {
			code int
			call func() (errcode int, errmsg string, respIsNil bool, err error)
		}{
			"currency_pay": {268490006, func() (int, string, bool, error) {
				r, err := CurrencyPay(ctx, "T", "K", "S", CurrencyPayRequest{
					OpenID: "o", UserIP: "1.2.3.4", Amount: 100, OrderID: "c1"})
				if r == nil {
					return 0, "", true, err
				}
				return r.ErrCode, r.ErrMsg, false, err
			}},
			"present_currency": {268490004, func() (int, string, bool, error) {
				r, err := PresentCurrency(ctx, "T",
					PresentCurrencyRequest{OpenID: "o", OrderID: "p1", Amount: 5})
				if r == nil {
					return 0, "", true, err
				}
				return r.ErrCode, r.ErrMsg, false, err
			}},
		}

		for name, c := range calls {
			t.Run(name, func(t *testing.T) {
				rt := &xpayRT{resp: `{"errcode":` + strconv.Itoa(c.code) + `,"errmsg":"业务失败"}`}
				swapXpay(t, rt)

				code, msg, isNil, err := c.call()
				if err != nil {
					t.Fatalf("业务失败不该变成 error——微信的错误响应也是合法 JSON: %v", err)
				}
				if isNil {
					t.Fatal("业务失败时响应也必须非 nil")
				}
				if code != c.code {
					t.Errorf("errcode 必须是微信给的原值 %d，实际 %d", c.code, code)
				}
				if msg != "业务失败" {
					t.Errorf("errmsg 必须是原值，实际 %q", msg)
				}
			})
		}
	})
}
