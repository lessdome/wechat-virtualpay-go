package wechat_virtualpay_go

import "testing"

// 官方 2.6 参考脚本里写死的 post_body（注意冒号后有空格，是它自己的序列化结果）
const docBody = `{"openid": "xxx", "user_ip": "127.0.0.1", "env": 0}`

// 第 2 项：直接对官方参考脚本里的两个 assert。
func TestSignMatchesOfficialSample(t *testing.T) {
	if got := CalcPaySig("12345", "/xpay/query_user_balance", docBody); got != "c37809f27c6d7fd1837ad2500a04512b66b34fd793a39a385fade56dca89a4b5" {
		t.Errorf("pay_sig = %s，与官方样例不一致", got)
	}
	if got := CalcSignature("9hAb/NEYUlkaMBEsmFgzig==", docBody); got != "089d9e8dc5d308977360c4b79ec600a93d736802802a807d634192328032f6c7" {
		t.Errorf("signature = %s，与官方样例不一致", got)
	}
}

// uri 带不带 "?" 后面那段，签名必须不同——这正是 268490003 最常见的来源。
func TestPaySigUriMustNotCarryQuery(t *testing.T) {
	clean := CalcPaySig("k", "/xpay/query_order", "{}")
	dirty := CalcPaySig("k", "/xpay/query_order?access_token=x", "{}")
	if clean == dirty {
		t.Fatal("uri 带上 query string 后签名应当变化")
	}
}

// pay_sig 的签名原文就是 uri + "&" + signData，signature 则直接用 signData。
// 两者不能共用同一个实现；关系必须恰好是这样。
func TestPaySigIsSignatureOverUriAndBody(t *testing.T) {
	const uri, body = "/xpay/query_order", "{}"
	if CalcPaySig("k", uri, body) != CalcSignature("k", uri+"&"+body) {
		t.Fatal(`pay_sig 应当等于对 uri + "&" + signData 做签名`)
	}
}
