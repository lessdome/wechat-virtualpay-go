package wechat_virtualpay_go

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func coinReq() CoinPaymentRequest {
	return CoinPaymentRequest{
		Quantity:   100,
		OutTradeNo: "COIN20260101",
		Attach:     "testdata",
	}
}

// 字段与顺序：沿用官方字段表的相对次序，且**不带**道具专有的那几个。
func TestCoinSignDataShape(t *testing.T) {
	p, err := BuildCoinPayment(testOfferID, testAppKey, testSessionKey, coinReq())
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"offerId":"123","buyQuantity":100,"env":0,"currencyType":"CNY","outTradeNo":"COIN20260101","attach":"testdata"}`
	if p.SignData != want {
		t.Fatalf("signData 不对（got %s want %s）", p.SignData, want)
	}
	if p.Mode != ModeShortSeriesCoin {
		t.Fatalf("Mode 不对: %s", p.Mode)
	}
	// 道具专有的字段一个都不该出现
	for _, field := range []string{"productId", "goodsPrice", "activitySellingPrice", "mode"} {
		if strings.Contains(p.SignData, field) {
			t.Errorf("代币的 signData 里不该有 %s: %s", field, p.SignData)
		}
	}
	t.Logf("signData: %s", p.SignData)
}

// 签名自洽：两个签名都能由**返回的 SignData** 独立算出。
func TestCoinSignatures(t *testing.T) {
	p, err := BuildCoinPayment(testOfferID, testAppKey, testSessionKey, coinReq())
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
		t.Errorf("signature 复算不一致（got %s want %s）", p.Signature, want)
	}
}

// 与道具直购一样是纯拼装，一次请求都不发。
func TestCoinDoesNotTouchNetwork(t *testing.T) {
	rt := &sessionRT{body: `{"openid":"o","session_key":"s"}`}
	swapClient(t, rt)

	if _, err := BuildCoinPayment(testOfferID, testAppKey, testSessionKey, coinReq()); err != nil {
		t.Fatal(err)
	}
	if rt.calls != 0 {
		t.Fatalf("BuildCoinPayment 不该发请求，实际发了 %d 次", rt.calls)
	}
}

// 两条流程确实是分开的：同样的凭据下，signData 与 mode 都不同。
func TestCoinDiffersFromGoods(t *testing.T) {
	goods, err := BuildGoodsPayment(testOfferID, testAppKey, testSessionKey, goodsReq())
	if err != nil {
		t.Fatal(err)
	}
	coin, err := BuildCoinPayment(testOfferID, testAppKey, testSessionKey, coinReq())
	if err != nil {
		t.Fatal(err)
	}
	if goods.Mode == coin.Mode {
		t.Fatalf("两条流程的 mode 应当不同，都是 %s", goods.Mode)
	}
	if goods.SignData == coin.SignData {
		t.Fatal("两条流程的 signData 不该相同")
	}
	// 代币那条里不该出现道具的字段
	if strings.Contains(coin.SignData, "productId") || strings.Contains(coin.SignData, "goodsPrice") {
		t.Fatalf("代币的 signData 混进了道具字段: %s", coin.SignData)
	}
}

// 数量缺省按 1
func TestCoinQuantityDefault(t *testing.T) {
	r := coinReq()
	r.Quantity = 0
	p, err := BuildCoinPayment(testOfferID, testAppKey, testSessionKey, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.SignData, `"buyQuantity":1`) {
		t.Errorf("未填数量应按 1，实际: %s", p.SignData)
	}
}

// 校验：必填项与订单号格式
func TestCoinValidation(t *testing.T) {
	base := coinReq()
	cases := []struct {
		name string
		f    func(*CoinPaymentRequest)
		want string
	}{
		{"OutTradeNo 空", func(r *CoinPaymentRequest) { r.OutTradeNo = "" }, "OutTradeNo"},
		{"OutTradeNo 非法字符", func(r *CoinPaymentRequest) { r.OutTradeNo = "abc#1234567" }, "OutTradeNo"},
		{"OutTradeNo 下划线开头", func(r *CoinPaymentRequest) { r.OutTradeNo = "_abc12345" }, "下划线"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.f(&r)
			if _, err := BuildCoinPayment(testOfferID, testAppKey, testSessionKey, r); err == nil || !strings.Contains(err.Error(), tc.want) {
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
			if _, err := BuildCoinPayment(tc.oid, tc.key, tc.sk, base); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
			}
		})
	}
}
