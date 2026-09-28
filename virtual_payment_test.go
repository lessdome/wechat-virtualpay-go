package wechat_virtualpay_go

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// 官方示例用的就是道具直购。它的 outTradeNo 是 'xxxxxx'（6 位）不合规范，
// 这里换成 8 位，其余字段与顺序完全照抄。
func TestGoodsSignDataMatchesOfficialExample(t *testing.T) {
	p, err := BuildGoodsPayment("123", "appkey", GoodsPaymentRequest{
		ProductID:  "testproductId",
		GoodsPrice: 10,
		Quantity:   1,
		OutTradeNo: "xxxxxx12",
		Attach:     "testdata",
		SessionKey: "sk",
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"offerId":"123","buyQuantity":1,"env":0,"currencyType":"CNY","productId":"testproductId","goodsPrice":10,"outTradeNo":"xxxxxx12","attach":"testdata"}`
	if p.SignData != want {
		t.Fatalf("signData 与官方示例的字段/顺序不一致:\n got %s\nwant %s", p.SignData, want)
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

// 自洽 + 可独立复算：两个签名都必须能由**返回的 SignData** 独立算出
func TestSignaturesAreComputedOverReturnedSignData(t *testing.T) {
	const appKey, sessionKey = "k1", "sk1"
	p, err := BuildGoodsPayment("offerX", appKey, GoodsPaymentRequest{
		ProductID: "p1", GoodsPrice: 100, OutTradeNo: "ORDER20260101", Attach: "a",
		SessionKey: sessionKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 独立实现一遍：paySig = hex(hmac(appKey, "requestVirtualPayment" + "&" + signData))
	mac := hmac.New(sha256.New, []byte(appKey))
	mac.Write([]byte("requestVirtualPayment&" + p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.PaySig != want {
		t.Errorf("paySig 复算不一致:\n got %s\nwant %s", p.PaySig, want)
	}
	// signature = hex(hmac(sessionKey, signData))，不带 uri
	mac = hmac.New(sha256.New, []byte(sessionKey))
	mac.Write([]byte(p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.Signature != want {
		t.Errorf("signature 复算不一致:\n got %s\nwant %s", p.Signature, want)
	}
}

// buyQuantity：未填（<=0）按 1，显式填了就用它
func TestGoodsQuantity(t *testing.T) {
	base := GoodsPaymentRequest{ProductID: "p", GoodsPrice: 1, OutTradeNo: "ORDER1234", SessionKey: "sk"}

	p, err := BuildGoodsPayment("o", "k", base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.SignData, `"buyQuantity":1`) {
		t.Errorf("未填数量应按 1，实际: %s", p.SignData)
	}

	q := base
	q.Quantity = 3
	p, err = BuildGoodsPayment("o", "k", q)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.SignData, `"buyQuantity":3`) {
		t.Errorf("数量没带上: %s", p.SignData)
	}
}

// 优惠价：传了才出现，且位置在 goodsPrice 之后、outTradeNo 之前
func TestGoodsActivitySellingPrice(t *testing.T) {
	p, err := BuildGoodsPayment("o", "k", GoodsPaymentRequest{
		ProductID: "p", GoodsPrice: 100, ActivitySellingPrice: 60,
		OutTradeNo: "ORDER1234", SessionKey: "sk",
	})
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

	// 不传就不出现
	p2, err := BuildGoodsPayment("o", "k", GoodsPaymentRequest{
		ProductID: "p", GoodsPrice: 100, OutTradeNo: "ORDER1234", SessionKey: "sk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p2.SignData, "activitySellingPrice") {
		t.Errorf("没传优惠价却出现在 signData 里: %s", p2.SignData)
	}
}

// 必填项的校验
func TestGoodsValidation(t *testing.T) {
	ok := GoodsPaymentRequest{
		ProductID: "p", GoodsPrice: 1, OutTradeNo: "ORDER1234", SessionKey: "sk",
	}
	cases := []struct {
		name string
		f    func(*GoodsPaymentRequest)
		want string
	}{
		{"ProductID", func(r *GoodsPaymentRequest) { r.ProductID = "" }, "ProductID"},
		{"GoodsPrice", func(r *GoodsPaymentRequest) { r.GoodsPrice = 0 }, "GoodsPrice"},
		{"SessionKey", func(r *GoodsPaymentRequest) { r.SessionKey = "" }, "SessionKey"},
		{"OutTradeNo 空", func(r *GoodsPaymentRequest) { r.OutTradeNo = "" }, "OutTradeNo"},
		{"OutTradeNo 太短", func(r *GoodsPaymentRequest) { r.OutTradeNo = "abc" }, "OutTradeNo"},
		{"OutTradeNo 非法字符", func(r *GoodsPaymentRequest) { r.OutTradeNo = "abc#1234567" }, "OutTradeNo"},
		{"OutTradeNo 下划线开头", func(r *GoodsPaymentRequest) { r.OutTradeNo = "_abc12345" }, "下划线"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ok
			tc.f(&r)
			if _, err := BuildGoodsPayment("o", "k", r); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
			}
		})
	}

	if _, err := BuildGoodsPayment("", "k", ok); err == nil || !strings.Contains(err.Error(), "offerID") {
		t.Errorf("offerID 为空应当报错: %v", err)
	}
	if _, err := BuildGoodsPayment("o", "", ok); err == nil || !strings.Contains(err.Error(), "appKey") {
		t.Errorf("appKey 为空应当报错: %v", err)
	}
}

// 端到端：signature 必须是用 Code2Session **生成出来的** SessionKey 算的。
// 这条把「换登录态」与「拼下单参数」两步接起来验一次。
func TestGoodsUsesGeneratedSessionKey(t *testing.T) {
	rt := &sessionRT{body: `{"openid":"o1","session_key":"generated-kk"}`}
	swapClient(t, rt)

	sess, err := Code2Session(context.Background(), "appid", "secret", "logincode")
	if err != nil {
		t.Fatal(err)
	}
	if sess.SessionKey != "generated-kk" {
		t.Fatalf("换来的 session_key 不对: %+v", sess)
	}

	p, err := BuildGoodsPayment("offerX", "appKey1", GoodsPaymentRequest{
		ProductID:  "p1",
		GoodsPrice: 100,
		OutTradeNo: "ORDER20260101",
		Attach:     "a",
		SessionKey: sess.SessionKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	mac := hmac.New(sha256.New, []byte("generated-kk"))
	mac.Write([]byte(p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.Signature != want {
		t.Fatalf("signature 不是用生成出来的 session_key 算的（got %s want %s）", p.Signature, want)
	}
	t.Logf("signData:  %s", p.SignData)
	t.Logf("signature: %s（用 Code2Session 换来的 session_key 算出）", p.Signature)
}
