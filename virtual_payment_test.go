package wechat_virtualpay_go

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

const (
	testOfferID    = "123"
	testAppKey     = "appkey"
	testSessionKey = "a-session-key"
)

// 道具直购的入参。官方示例用的就是这一条——它的 outTradeNo 是 'xxxxxx'（6 位）不合
// 它自己的规范，这里换成 8 位，其余字段取值照抄。
func goodsReq() PaymentRequest {
	return PaymentRequest{
		Mode:       ModeShortSeriesGoods,
		ProductID:  "testproductId",
		GoodsPrice: 10,
		Quantity:   1,
		OutTradeNo: "xxxxxx12",
		Attach:     "testdata",
	}
}

func coinReq() PaymentRequest {
	return PaymentRequest{
		Mode:       ModeShortSeriesCoin,
		Quantity:   100,
		OutTradeNo: "COIN20260101",
		Attach:     "testdata",
	}
}

func build(t *testing.T, r PaymentRequest) *VirtualPaymentParams {
	t.Helper()
	p, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// 道具：signData 与官方示例的字段与顺序逐字一致
func TestGoodsSignDataMatchesOfficialExample(t *testing.T) {
	p := build(t, goodsReq())
	const want = `{"offerId":"123","buyQuantity":1,"env":0,"currencyType":"CNY","productId":"testproductId","goodsPrice":10,"outTradeNo":"xxxxxx12","attach":"testdata"}`
	if p.SignData != want {
		t.Fatalf("signData 与官方示例不一致（got %s want %s）", p.SignData, want)
	}
	if p.Mode != ModeShortSeriesGoods {
		t.Fatalf("Mode 不对: %s", p.Mode)
	}
	if strings.Contains(p.SignData, "mode") {
		t.Fatalf("mode 是顶层参数，不该进 signData: %s", p.SignData)
	}
	t.Logf("道具: %s", p.SignData)
}

// 代币：不带任何道具专有字段
func TestCoinSignDataShape(t *testing.T) {
	p := build(t, coinReq())
	const want = `{"offerId":"123","buyQuantity":100,"env":0,"currencyType":"CNY","outTradeNo":"COIN20260101","attach":"testdata"}`
	if p.SignData != want {
		t.Fatalf("signData 不对（got %s want %s）", p.SignData, want)
	}
	if p.Mode != ModeShortSeriesCoin {
		t.Fatalf("Mode 不对: %s", p.Mode)
	}
	for _, f := range []string{"productId", "goodsPrice", "activitySellingPrice", "mode"} {
		if strings.Contains(p.SignData, f) {
			t.Errorf("代币的 signData 里不该有 %s: %s", f, p.SignData)
		}
	}
	t.Logf("代币: %s", p.SignData)
}

// 两种模式确实走的是两条形状
func TestGoodsAndCoinDiffer(t *testing.T) {
	g := build(t, goodsReq())
	c := build(t, coinReq())
	if g.Mode == c.Mode || g.SignData == c.SignData {
		t.Fatal("两种模式的 mode 与 signData 都应当不同")
	}
}

// 两个签名都必须能由**返回的 SignData** 独立算出（两种模式都验）
func TestSignatures(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  PaymentRequest
	}{{"道具", goodsReq()}, {"代币", coinReq()}} {
		t.Run(tc.name, func(t *testing.T) {
			p := build(t, tc.req)

			mac := hmac.New(sha256.New, []byte(testAppKey))
			mac.Write([]byte("requestVirtualPayment&" + p.SignData))
			if want := hex.EncodeToString(mac.Sum(nil)); p.PaySig != want {
				t.Errorf("paySig 复算不一致（got %s want %s）", p.PaySig, want)
			}
			mac = hmac.New(sha256.New, []byte(testSessionKey))
			mac.Write([]byte(p.SignData))
			if want := hex.EncodeToString(mac.Sum(nil)); p.Signature != want {
				t.Errorf("signature 复算不一致（got %s want %s）", p.Signature, want)
			}
		})
	}
}

// env 是下单唯一能选环境的地方：零值即现网（"env":0），沙箱要显式传 1。
//
// 只钉「发出去的是哪个数」——**AppKey 与环境配不配套，本包分辨不了**（两把都是普通
// 字符串），那条只能靠调用方保证，测不了。
func TestPaymentEnv(t *testing.T) {
	// 零值=现网。上面两条逐字比对里已经钉了 "env":0，这里再挡一道「有人把零值改没了」。
	if p := build(t, goodsReq()); !strings.Contains(p.SignData, `"env":0`) {
		t.Fatalf("不填 Env 应当是现网 0: %s", p.SignData)
	}

	r := goodsReq()
	r.Env = 1 // 沙箱
	p := build(t, r)
	if !strings.Contains(p.SignData, `"env":1`) {
		t.Fatalf("Env=沙箱 时 signData 里应当是 1: %s", p.SignData)
	}
	// 签名必须由**改过 env 的**那份 signData 算出——独立复算，不复用 CalcPaySig。
	mac := hmac.New(sha256.New, []byte(testAppKey))
	mac.Write([]byte("requestVirtualPayment&" + p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.PaySig != want {
		t.Errorf("paySig 复算不一致（got %s want %s）", p.PaySig, want)
	}
}

// env 非法（官方只有 0/1）本地就拒，别带着它去算签名；沙箱下缺 AppKey 时，报错要点出
// 该配的是沙箱那把——与 /xpay/* 那边的报错一致。
func TestPaymentEnvValidation(t *testing.T) {
	r := goodsReq()
	r.Env = 2
	if _, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), "Env 2 非法") {
		t.Fatalf("Env 非法应当本地报错，实际: %v", err)
	}

	r = goodsReq()
	r.Env = 1 // 沙箱
	if _, err := BuildPayment(testOfferID, "", testSessionKey, r); err == nil || !strings.Contains(err.Error(), "沙箱 AppKey") {
		t.Fatalf("沙箱下缺 appKey 要指明沙箱那把，实际: %v", err)
	}
}

// 它是纯拼装：一次网络请求都不发
func TestDoesNotTouchNetwork(t *testing.T) {
	rt := &sessionRT{body: `{"openid":"o","session_key":"s"}`}
	swapClient(t, rt)

	build(t, goodsReq())
	build(t, coinReq())
	if rt.calls != 0 {
		t.Fatalf("BuildPayment 不该发请求，实际发了 %d 次", rt.calls)
	}
}

// 两步组合：Code2Session 换登录态 → BuildPayment，signature 用换来的那个 key
func TestWithCode2Session(t *testing.T) {
	const generated = "generated-session-key"
	swapClient(t, &sessionRT{body: `{"openid":"o1","session_key":"` + generated + `"}`})

	sess, err := Code2Session(context.Background(), "appid", "secret", "logincode")
	if err != nil {
		t.Fatal(err)
	}
	p, err := BuildPayment(testOfferID, testAppKey, sess.SessionKey, goodsReq())
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte(generated))
	mac.Write([]byte(p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.Signature != want {
		t.Fatalf("signature 不是用换来的 session_key 算的（got %s want %s）", p.Signature, want)
	}
}

// buyQuantity：未填（<=0）按 1，显式填了就用它——两种模式都是
func TestQuantity(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  PaymentRequest
	}{{"道具", goodsReq()}, {"代币", coinReq()}} {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.req
			r.Quantity = 0
			if s := build(t, r).SignData; !strings.Contains(s, `"buyQuantity":1`) {
				t.Errorf("未填数量应按 1，实际: %s", s)
			}
			r.Quantity = 3
			if s := build(t, r).SignData; !strings.Contains(s, `"buyQuantity":3`) {
				t.Errorf("数量没带上: %s", s)
			}
		})
	}
}

// 优惠价：传了才出现，且位置在 goodsPrice 之后、outTradeNo 之前
func TestActivitySellingPrice(t *testing.T) {
	r := goodsReq()
	r.GoodsPrice, r.ActivitySellingPrice = 100, 60
	p := build(t, r)
	iGoods := strings.Index(p.SignData, `"goodsPrice"`)
	iAct := strings.Index(p.SignData, `"activitySellingPrice":60`)
	iOrder := strings.Index(p.SignData, `"outTradeNo"`)
	if iAct < 0 {
		t.Fatalf("优惠价没带上: %s", p.SignData)
	}
	if !(iGoods < iAct && iAct < iOrder) {
		t.Fatalf("优惠价位置不对（应在 goodsPrice 之后、outTradeNo 之前）: %s", p.SignData)
	}

	r.ActivitySellingPrice = 0
	if s := build(t, r).SignData; strings.Contains(s, "activitySellingPrice") {
		t.Errorf("没传优惠价却出现在 signData 里: %s", s)
	}
}

// 优惠价有下限：不得低于道具价的 40%。边界用整数比较，正好 40% 要能过。
func TestActivitySellingPriceLowerBound(t *testing.T) {
	cases := []struct {
		name            string
		goods, activity int64
		wantErr         bool
	}{
		{"正好 40% 放行", 100, 40, false},
		{"高于 40% 放行", 100, 41, false},
		{"低于 40% 报错", 100, 39, true},
		{"远低于 40% 报错", 100, 1, true},
		{"分币粒度下的 40% 边界", 10, 4, false},
		{"不传优惠价则不校验", 100, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := goodsReq()
			r.GoodsPrice, r.ActivitySellingPrice = c.goods, c.activity
			_, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r)
			if c.wantErr {
				if err == nil || !strings.Contains(err.Error(), "40%") {
					t.Fatalf("期望报错含 40%%，实际: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("不该报错: %v", err)
			}
		})
	}
}

// 订单号格式
func TestOutTradeNoValidation(t *testing.T) {
	cases := []struct {
		name, no, want string
	}{
		{"空", "", "OutTradeNo"},
		{"太短", "abc", "OutTradeNo"},
		{"非法字符", "abc#1234567", "OutTradeNo"},
		{"下划线开头", "_abc12345", "下划线"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := goodsReq()
			r.OutTradeNo = tc.no
			if _, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
			}
		})
	}
}

// 三个凭据各缺一个都要报错
func TestCredentialValidation(t *testing.T) {
	for _, tc := range []struct{ name, oid, key, sk, want string }{
		{"offerID", "", testAppKey, testSessionKey, "offerID"},
		{"appKey", testOfferID, "", testSessionKey, "appKey"},
		{"sessionKey", testOfferID, testAppKey, "", "sessionKey"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildPayment(tc.oid, tc.key, tc.sk, goodsReq()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
			}
		})
	}
}

// Attach 必填（官方 signData 字段表的必填列标「是」），两种模式都不许空
func TestAttachRequired(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  PaymentRequest
	}{{"道具", goodsReq()}, {"代币", coinReq()}} {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.req
			r.Attach = ""
			if _, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), "Attach") {
				t.Fatalf("空 Attach 应当报错，实际: %v", err)
			}
		})
	}
}

// 模式相关的校验：模式必填且要合法，道具字段按模式该有则有、该无则报错
func TestModeValidation(t *testing.T) {
	t.Run("Mode 为空", func(t *testing.T) {
		r := goodsReq()
		r.Mode = ""
		if _, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), "非法") {
			t.Fatalf("模式为空应当报错，实际: %v", err)
		}
	})
	t.Run("Mode 非法", func(t *testing.T) {
		r := goodsReq()
		r.Mode = "whatever"
		if _, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), "非法") {
			t.Fatalf("非法模式应当报错，实际: %v", err)
		}
	})
	t.Run("道具缺 ProductID", func(t *testing.T) {
		r := goodsReq()
		r.ProductID = ""
		if _, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), "ProductID") {
			t.Fatalf("期望报错含 ProductID，实际: %v", err)
		}
	})
	t.Run("道具缺 GoodsPrice", func(t *testing.T) {
		r := goodsReq()
		r.GoodsPrice = 0
		if _, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), "GoodsPrice") {
			t.Fatalf("期望报错含 GoodsPrice，实际: %v", err)
		}
	})
	t.Run("代币不该带道具字段", func(t *testing.T) {
		for _, f := range []func(*PaymentRequest){
			func(r *PaymentRequest) { r.ProductID = "p" },
			func(r *PaymentRequest) { r.GoodsPrice = 100 },
			func(r *PaymentRequest) { r.ActivitySellingPrice = 60 },
		} {
			r := coinReq()
			f(&r)
			if _, err := BuildPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), "道具直购专用") {
				t.Fatalf("代币带上道具字段应当报错，实际: %v", err)
			}
		}
	})
}

// 生成的订单号必须过本包自己的校验，且长度固定、不重复
func TestNewOutTradeNo(t *testing.T) {
	seen := make(map[string]bool, 2000)

	for i := 0; i < 2000; i++ {
		v, err := NewOutTradeNo()
		if err != nil {
			t.Fatal(err)
		}
		if err := checkOutTradeNo(v); err != nil {
			t.Fatalf("生成的单号没通过校验: %v（%s）", err, v)
		}
		if len(v) != 30 {
			t.Fatalf("长度应为 30，实际 %d：%s", len(v), v)
		}
		// 前 14 位是生成时刻的「年月日时分秒」。解析回来与当前时刻比，
		// 而不是比 time.Now().Format 出来的字符串：前缀取自生成时、字符串取自断言时，
		// 恰好跨零点的情况下两者会差一天，报一个与代码正确性无关的假失败。
		ts, err := time.ParseInLocation("20060102150405", v[:14], time.Local)
		if err != nil {
			t.Fatalf("时间前缀不是合法的 14 位时间戳：%s（%v）", v, err)
		}
		if d := time.Since(ts); d < -time.Minute || d > time.Minute {
			t.Fatalf("时间前缀偏离当前时刻 %v：%s", d, v)
		}
		if seen[v] {
			t.Fatalf("2000 次里出现重复：%s", v)
		}
		seen[v] = true
	}
	one, _ := NewOutTradeNo()
	t.Logf("样例: %s（长度 %d）", one, len(one))
}
