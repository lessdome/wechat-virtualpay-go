package wechat_virtualpay_go

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

const (
	testAppID      = "wxappid"
	testAppSecret  = "appsecret"
	testOfferID    = "123"
	testAppKey     = "appkey"
	testSessionKey = "generated-session-key"
)

// 让 Code2Session 返回一个固定的 session_key，于是测试不碰真网络。
func sessionOK(t *testing.T) *sessionRT {
	rt := &sessionRT{body: `{"openid":"o1","session_key":"` + testSessionKey + `"}`}
	swapClient(t, rt)
	return rt
}

func goodsReq() GoodsPaymentRequest {
	return GoodsPaymentRequest{
		ProductID:  "testproductId",
		GoodsPrice: 10,
		Quantity:   1,
		OutTradeNo: "xxxxxx12",
		Attach:     "testdata",
		Code:       "logincode",
	}
}

// 官方示例用的就是道具直购。它的 outTradeNo 是 'xxxxxx'（6 位）不合它自己的规范，
// 这里换成 8 位，其余字段与顺序完全照抄。
func TestGoodsSignDataMatchesOfficialExample(t *testing.T) {
	sessionOK(t)
	p, err := BuildGoodsPayment(context.Background(), testAppID, testAppSecret, testOfferID, testAppKey, goodsReq())
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"offerId":"123","buyQuantity":1,"env":0,"currencyType":"CNY","productId":"testproductId","goodsPrice":10,"outTradeNo":"xxxxxx12","attach":"testdata"}`
	if p.SignData != want {
		t.Fatalf("signData 与官方示例的字段/顺序不一致（got %s want %s）", p.SignData, want)
	}
	if p.Mode != ModeShortSeriesGoods {
		t.Fatalf("Mode 不对: %s", p.Mode)
	}
	// mode 是**顶层参数**，不能混进 signData
	if strings.Contains(p.SignData, "mode") {
		t.Fatalf("signData 里混进了 mode: %s", p.SignData)
	}
	t.Logf("signData: %s", p.SignData)
}

// 自洽 + 可独立复算：两个签名都必须能由**返回的 SignData** 独立算出，
// 且用户态签名用的是**程序自己换来的** session_key。
func TestGoodsSignatures(t *testing.T) {
	sessionOK(t)
	p, err := BuildGoodsPayment(context.Background(), testAppID, testAppSecret, testOfferID, testAppKey, goodsReq())
	if err != nil {
		t.Fatal(err)
	}

	mac := hmac.New(sha256.New, []byte(testAppKey))
	mac.Write([]byte("requestVirtualPayment&" + p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.PaySig != want {
		t.Errorf("paySig 复算不一致（got %s want %s）", p.PaySig, want)
	}

	mac = hmac.New(sha256.New, []byte(testSessionKey))
	mac.Write([]byte(p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.Signature != want {
		t.Errorf("signature 不是用换来的 session_key 算的（got %s want %s）", p.Signature, want)
	}
}

// 每次下单都要现换一次 session_key（code 五分钟有效、只能用一次）
func TestGoodsFetchesSessionEachTime(t *testing.T) {
	rt := sessionOK(t)
	if _, err := BuildGoodsPayment(context.Background(), testAppID, testAppSecret, testOfferID, testAppKey, goodsReq()); err != nil {
		t.Fatal(err)
	}
	if rt.calls != 1 {
		t.Fatalf("应当调用一次 code2Session，实际 %d 次", rt.calls)
	}
}

// code2Session 失败时要把错误透出来
func TestGoodsPropagatesSessionError(t *testing.T) {
	rt := &sessionRT{body: `{"errcode":40029,"errmsg":"invalid code"}`}
	swapClient(t, rt)

	_, err := BuildGoodsPayment(context.Background(), testAppID, testAppSecret, testOfferID, testAppKey, goodsReq())
	if err == nil || !strings.Contains(err.Error(), "40029") {
		t.Fatalf("期望透出 code2Session 的错误，实际: %v", err)
	}
}

// buyQuantity：未填（<=0）按 1，显式填了就用它
func TestGoodsQuantity(t *testing.T) {
	sessionOK(t)
	base := goodsReq()

	p, err := BuildGoodsPayment(context.Background(), testAppID, testAppSecret, testOfferID, testAppKey, base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.SignData, `"buyQuantity":1`) {
		t.Errorf("未填数量应按 1，实际: %s", p.SignData)
	}

	q := base
	q.Quantity = 3
	p, err = BuildGoodsPayment(context.Background(), testAppID, testAppSecret, testOfferID, testAppKey, q)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.SignData, `"buyQuantity":3`) {
		t.Errorf("数量没带上: %s", p.SignData)
	}
}

// 优惠价：传了才出现，且位置在 goodsPrice 之后、outTradeNo 之前
func TestGoodsActivitySellingPrice(t *testing.T) {
	sessionOK(t)

	r := goodsReq()
	r.GoodsPrice, r.ActivitySellingPrice = 100, 60
	p, err := BuildGoodsPayment(context.Background(), testAppID, testAppSecret, testOfferID, testAppKey, r)
	if err != nil {
		t.Fatal(err)
	}
	iGoods := strings.Index(p.SignData, `"goodsPrice"`)
	iAct := strings.Index(p.SignData, `"activitySellingPrice":60`)
	iOrder := strings.Index(p.SignData, `"outTradeNo"`)
	if iAct < 0 {
		t.Fatalf("优惠价没带上: %s", p.SignData)
	}
	if !(iGoods < iAct && iAct < iOrder) {
		t.Fatalf("优惠价的位置不对（应在 goodsPrice 之后、outTradeNo 之前）: %s", p.SignData)
	}

	r.ActivitySellingPrice = 0
	p2, err := BuildGoodsPayment(context.Background(), testAppID, testAppSecret, testOfferID, testAppKey, r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p2.SignData, "activitySellingPrice") {
		t.Errorf("没传优惠价却出现在 signData 里: %s", p2.SignData)
	}
}

// 必填项与格式的校验；**关键**：本地校验不过时绝不能去换登录态——
// code 只能用一次，为一条不合法的请求烧掉它，用户就得重新 wx.login。
func TestGoodsValidationFailsBeforeNetwork(t *testing.T) {
	base := goodsReq()
	cases := []struct {
		name string
		f    func(*GoodsPaymentRequest)
		want string
	}{
		{"ProductID", func(r *GoodsPaymentRequest) { r.ProductID = "" }, "ProductID"},
		{"GoodsPrice", func(r *GoodsPaymentRequest) { r.GoodsPrice = 0 }, "GoodsPrice"},
		{"Code", func(r *GoodsPaymentRequest) { r.Code = "" }, "Code"},
		{"OutTradeNo 空", func(r *GoodsPaymentRequest) { r.OutTradeNo = "" }, "OutTradeNo"},
		{"OutTradeNo 太短", func(r *GoodsPaymentRequest) { r.OutTradeNo = "abc" }, "OutTradeNo"},
		{"OutTradeNo 非法字符", func(r *GoodsPaymentRequest) { r.OutTradeNo = "abc#1234567" }, "OutTradeNo"},
		{"OutTradeNo 下划线开头", func(r *GoodsPaymentRequest) { r.OutTradeNo = "_abc12345" }, "下划线"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := sessionOK(t)
			r := base
			tc.f(&r)
			_, err := BuildGoodsPayment(context.Background(), testAppID, testAppSecret, testOfferID, testAppKey, r)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
			}
			if rt.calls != 0 {
				t.Errorf("本地校验不过，却已经去换登录态了（%d 次）——code 被白烧掉", rt.calls)
			}
		})
	}

	// 凭据为空同样不该发请求
	for _, tc := range []struct{ name, oid, key, want string }{
		{"offerID", "", testAppKey, "offerID"},
		{"appKey", testOfferID, "", "appKey"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := sessionOK(t)
			if _, err := BuildGoodsPayment(context.Background(), testAppID, testAppSecret, tc.oid, tc.key, base); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
			}
			if rt.calls != 0 {
				t.Errorf("凭据不全却发出了请求")
			}
		})
	}
}
