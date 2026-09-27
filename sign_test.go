package virtualpay

import "testing"

// 本文件的期望值均由 scripts/gen_vectors.py（独立的 Python HMAC-SHA256 实现）生成，
// 代码与脚本互相印证，避免"用同一个错误的实现自证正确"。

func TestCalcPaySig(t *testing.T) {
	cases := []struct {
		name     string
		appKey   string
		method   string
		signData string
		want     string
	}{
		{
			name:     "拉起支付（method 固定值，普通参数）",
			appKey:   "test_app_key_1234567890",
			method:   "requestVirtualPayment",
			signData: `{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY","platform":"android","productId":"prod_001","goodsPrice":100,"outTradeNo":"ORDER20260101001","attach":"test"}`,
			want:     "62a944504dbabe280e5a559280baecb11ee2b92237bb87eadd338a3bdf429d54",
		},
		{
			name:     "拉起支付（参数含 HTML 特殊字符 < > &）",
			appKey:   "test_app_key_1234567890",
			method:   "requestVirtualPayment",
			signData: `{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY","platform":"ios","productId":"a<b>&c","goodsPrice":100,"outTradeNo":"ORDER20260101002","attach":"x&y"}`,
			want:     "f9c7c448b71263a3b8b302e095cd62a169e4090ec5160b563fa75ad982491ba9",
		},
		{
			name:     "道具上传（method 为接口路径）",
			appKey:   "test_app_key_1234567890",
			method:   "/xpay/start_upload_goods",
			signData: `{"productId":"prod_001","price":100}`,
			want:     "a81508a31914fccf57699c96e5207ca1c93739b942ecd891aaec469caa41b97e",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CalcPaySig(c.appKey, c.method, c.signData)
			if got != c.want {
				t.Fatalf("CalcPaySig() = %s\nwant         = %s", got, c.want)
			}
			if len(got) != 64 {
				t.Fatalf("签名长度应为 64，实际 %d", len(got))
			}
		})
	}
}

func TestCalcSignature(t *testing.T) {
	const (
		sessionKey = "test_session_key_abcdef"
		signData   = `{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY","platform":"android","productId":"prod_001","goodsPrice":100,"outTradeNo":"ORDER20260101001","attach":"test"}`
		want       = "4a375c7d1497aae1a17ba060589add5f65a65351f048716818d957ff6434952f"
	)
	if got := CalcSignature(sessionKey, signData); got != want {
		t.Fatalf("CalcSignature() = %s\nwant             = %s", got, want)
	}
}

// 关键差异回归：pay_sig 会拼 method，signature 不拼。
// 若误把两者实现成同一个函数，此测试会失败。
func TestSignatureDiffersFromPaySig(t *testing.T) {
	const (
		appKey   = "test_app_key_1234567890"
		session  = "test_session_key_abcdef"
		method   = "requestVirtualPayment"
		signData = `{"a":1}`
	)
	if CalcPaySig(appKey, method, signData) == CalcSignature(session, signData) {
		t.Fatal("pay_sig 与 signature 不应相同（两者算法不同）")
	}
}

func TestCalcPayEventSigAndVerify(t *testing.T) {
	const (
		appKey  = "test_app_key_1234567890"
		event   = "xpay_goods_deliver_notify"
		payload = `{"outTradeNo":"ORDER20260101001","productId":"prod_001"}`
		want    = "fda68ccc3f2ac30a11aee40584ea021e1999026fbbb432875070ef1ce46074f0"
	)

	sig := CalcPayEventSig(appKey, event, payload)
	if sig != want {
		t.Fatalf("CalcPayEventSig() = %s\nwant               = %s", sig, want)
	}

	if !VerifyPayEventSig(appKey, event, payload, sig) {
		t.Fatal("正确签名应校验通过")
	}
	if VerifyPayEventSig(appKey, event, payload, "deadbeef") {
		t.Fatal("错误签名不应校验通过")
	}
	if VerifyPayEventSig(appKey, event, payload+" ", sig) {
		t.Fatal("payload 被篡改后不应校验通过")
	}
	if VerifyPayEventSig("wrong_key", event, payload, sig) {
		t.Fatal("appKey 不同时不应校验通过")
	}
}
