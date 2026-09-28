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

func goodsReq() GoodsPaymentRequest {
	return GoodsPaymentRequest{
		ProductID:  "testproductId",
		GoodsPrice: 10,
		Quantity:   1,
		OutTradeNo: "xxxxxx12",
		Attach:     "testdata",
	}
}

// 官方示例用的就是道具直购。它的 outTradeNo 是 'xxxxxx'（6 位）不合它自己的规范，
// 这里换成 8 位，其余字段与顺序完全照抄。
func TestGoodsSignDataMatchesOfficialExample(t *testing.T) {
	p, err := BuildGoodsPayment(testOfferID, testAppKey, testSessionKey, goodsReq())
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

// 自洽 + 可独立复算：两个签名都必须能由**返回的 SignData** 独立算出。
func TestGoodsSignatures(t *testing.T) {
	p, err := BuildGoodsPayment(testOfferID, testAppKey, testSessionKey, goodsReq())
	if err != nil {
		t.Fatal(err)
	}

	// paySig = hex(hmac(appKey, "requestVirtualPayment" + "&" + signData))
	mac := hmac.New(sha256.New, []byte(testAppKey))
	mac.Write([]byte("requestVirtualPayment&" + p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.PaySig != want {
		t.Errorf("paySig 复算不一致（got %s want %s）", p.PaySig, want)
	}
	// signature = hex(hmac(sessionKey, signData))，不带 uri
	mac = hmac.New(sha256.New, []byte(testSessionKey))
	mac.Write([]byte(p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.Signature != want {
		t.Errorf("signature 复算不一致（got %s want %s）", p.Signature, want)
	}
}

// 它是纯拼装：一次网络请求都不该发。
func TestGoodsDoesNotTouchNetwork(t *testing.T) {
	rt := &sessionRT{body: `{"openid":"o","session_key":"s"}`}
	swapClient(t, rt)

	if _, err := BuildGoodsPayment(testOfferID, testAppKey, testSessionKey, goodsReq()); err != nil {
		t.Fatal(err)
	}
	if rt.calls != 0 {
		t.Fatalf("BuildGoodsPayment 不该发请求，实际发了 %d 次", rt.calls)
	}
}

// 两步组合的完整用法：Code2Session 换登录态 → BuildGoodsPayment 拼参数，
// 且 signature 必须是用**换来的**那个 session_key 算的。
func TestGoodsWithCode2Session(t *testing.T) {
	const generated = "generated-session-key"
	rt := &sessionRT{body: `{"openid":"o1","session_key":"` + generated + `"}`}
	swapClient(t, rt)

	sess, err := Code2Session(context.Background(), "appid", "secret", "logincode")
	if err != nil {
		t.Fatal(err)
	}

	p, err := BuildGoodsPayment(testOfferID, testAppKey, sess.SessionKey, goodsReq())
	if err != nil {
		t.Fatal(err)
	}

	mac := hmac.New(sha256.New, []byte(generated))
	mac.Write([]byte(p.SignData))
	if want := hex.EncodeToString(mac.Sum(nil)); p.Signature != want {
		t.Fatalf("signature 不是用换来的 session_key 算的（got %s want %s）", p.Signature, want)
	}
	t.Logf("signature: %s", p.Signature)
}

// buyQuantity：未填（<=0）按 1，显式填了就用它
func TestGoodsQuantity(t *testing.T) {
	base := goodsReq()

	p, err := BuildGoodsPayment(testOfferID, testAppKey, testSessionKey, base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.SignData, `"buyQuantity":1`) {
		t.Errorf("未填数量应按 1，实际: %s", p.SignData)
	}

	q := base
	q.Quantity = 3
	p, err = BuildGoodsPayment(testOfferID, testAppKey, testSessionKey, q)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.SignData, `"buyQuantity":3`) {
		t.Errorf("数量没带上: %s", p.SignData)
	}
}

// 优惠价：传了才出现，且位置在 goodsPrice 之后、outTradeNo 之前
func TestGoodsActivitySellingPrice(t *testing.T) {
	r := goodsReq()
	r.GoodsPrice, r.ActivitySellingPrice = 100, 60
	p, err := BuildGoodsPayment(testOfferID, testAppKey, testSessionKey, r)
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
	p2, err := BuildGoodsPayment(testOfferID, testAppKey, testSessionKey, r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p2.SignData, "activitySellingPrice") {
		t.Errorf("没传优惠价却出现在 signData 里: %s", p2.SignData)
	}
}

// 必填项与格式的校验
func TestGoodsValidation(t *testing.T) {
	base := goodsReq()
	cases := []struct {
		name string
		f    func(*GoodsPaymentRequest)
		want string
	}{
		{"ProductID", func(r *GoodsPaymentRequest) { r.ProductID = "" }, "ProductID"},
		{"GoodsPrice", func(r *GoodsPaymentRequest) { r.GoodsPrice = 0 }, "GoodsPrice"},
		{"OutTradeNo 空", func(r *GoodsPaymentRequest) { r.OutTradeNo = "" }, "OutTradeNo"},
		{"OutTradeNo 太短", func(r *GoodsPaymentRequest) { r.OutTradeNo = "abc" }, "OutTradeNo"},
		{"OutTradeNo 非法字符", func(r *GoodsPaymentRequest) { r.OutTradeNo = "abc#1234567" }, "OutTradeNo"},
		{"OutTradeNo 下划线开头", func(r *GoodsPaymentRequest) { r.OutTradeNo = "_abc12345" }, "下划线"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.f(&r)
			if _, err := BuildGoodsPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
			}
		})
	}

	for _, tc := range []struct{ name, oid, key, sk, want string }{
		{"offerID", "", testAppKey, testSessionKey, "offerID"},
		{"appKey", testOfferID, "", testSessionKey, "appKey"},
		{"sessionKey", testOfferID, testAppKey, "", "sessionKey"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildGoodsPayment(tc.oid, tc.key, tc.sk, base); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
			}
		})
	}
}

// 生成的订单号必须过本包自己的校验，且长度固定、不重复。
func TestNewOutTradeNo(t *testing.T) {
	seen := make(map[string]bool, 2000)
	prefix := time.Now().Format("20060102")

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
		if !strings.HasPrefix(v, prefix) {
			t.Fatalf("时间前缀不对：%s", v)
		}
		if seen[v] {
			t.Fatalf("2000 次里出现重复：%s", v)
		}
		seen[v] = true
	}
	one, _ := NewOutTradeNo()
	t.Logf("样例: %s（长度 %d）", one, len(one))
}
