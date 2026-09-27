package virtualpay

import "testing"

// 本文件的期望值均由 scripts/gen_vectors.py（独立的 Python HMAC-SHA256 实现）生成，
// 代码与脚本互相印证，避免"用同一个错误的实现自证正确"。
//
// 该脚本启动时**先对着微信官方文档正文里自带的 assert 样例自检**，自检不过就终止，
// 因此它生成的向量可信。官方样例本身也固化成了 TestOfficialVectors。

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
			signData: `{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY","productId":"prod_001","goodsPrice":100,"outTradeNo":"ORDER20260101001","attach":"test"}`,
			want:     "101204af6c65f6f47b158dc99933ed6e1ddaf23152f226afc31b9e058c0d5357",
		},
		{
			name:     "拉起支付（参数含 HTML 特殊字符 < > &）",
			appKey:   "test_app_key_1234567890",
			method:   "requestVirtualPayment",
			signData: `{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY","productId":"a<b>&c","goodsPrice":100,"outTradeNo":"ORDER20260101002","attach":"x&y"}`,
			want:     "540b7ce535b85c51f5419207c1a833f10765d1050254f21a1cfa23a2bcf4e271",
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
		signData   = `{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY","productId":"prod_001","goodsPrice":100,"outTradeNo":"ORDER20260101001","attach":"test"}`
		want       = "d98ead1b8e264829062e802822714fd9bcf4c774d402e04cc39a74860c7829e6"
	)
	if got := CalcSignature(sessionKey, signData); got != want {
		t.Fatalf("CalcSignature() = %s\nwant             = %s", got, want)
	}
}

// TestOfficialVectors 用的是**微信官方文档正文里自带 assert 的样例**——
// 这是唯一来自腾讯而非本项目的期望值，比任何自生成向量都可信。
//
// 特别留意 post_body 里的空格：Go 的 json.Marshal 永远产不出这个串
// （它会输出 `{"openid":"xxx",...}`，无空格）。所以这里只能原样硬编码。
// 这恰好说明为什么 pay_sig 必须对「真正发出去的那串字节」计算——
// 而不是「重新序列化一次再看是否等价」。
func TestOfficialVectors(t *testing.T) {
	const (
		officialAppKey     = "12345"
		officialSessionKey = "9hAb/NEYUlkaMBEsmFgzig=="
		officialPostBody   = `{"openid": "xxx", "user_ip": "127.0.0.1", "env": 0}`
		officialURI        = "/xpay/query_user_balance"
	)

	t.Run("pay_sig", func(t *testing.T) {
		const want = "c37809f27c6d7fd1837ad2500a04512b66b34fd793a39a385fade56dca89a4b5"
		if got := CalcPaySig(officialAppKey, officialURI, officialPostBody); got != want {
			t.Fatalf("CalcPaySig() = %s\n官方期望     = %s", got, want)
		}
	})

	t.Run("signature", func(t *testing.T) {
		const want = "089d9e8dc5d308977360c4b79ec600a93d736802802a807d634192328032f6c7"
		if got := CalcSignature(officialSessionKey, officialPostBody); got != want {
			t.Fatalf("CalcSignature() = %s\n官方期望         = %s", got, want)
		}
	})
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
