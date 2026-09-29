package wechat_virtualpay

import "testing"

// 官方《消息推送》页给了两条带具体数值的样例，直接拿来当验收。
// 样例里的 Token 是 "AAAAA"。
func TestVerifySignatureMatchesOfficialSample(t *testing.T) {
	cases := []struct {
		name, ts, nonce, want string
	}{
		{"样例一", "1714036504", "1514711492", "f464b24fc39322e44b38aa78f5edd27bd1441696"},
		{"样例二", "1714037059", "486452656", "899cf89e464efb63f54ddac96b0a0a235f53aa78"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !verifySignature("AAAAA", c.ts, c.nonce, c.want) {
				t.Fatalf("官方样例没验过（ts=%s nonce=%s）", c.ts, c.nonce)
			}
		})
	}
}

// 任一项不同都必须不通过；大小写也要敏感。
func TestVerifySignatureRejects(t *testing.T) {
	const ts, nonce, sig = "1714036504", "1514711492", "f464b24fc39322e44b38aa78f5edd27bd1441696"
	cases := []struct{ name, token, ts, nonce, sig string }{
		{"签名不对", "AAAAA", ts, nonce, "deadbeef"},
		{"签名截断", "AAAAA", ts, nonce, sig[:39]},
		{"签名大写", "AAAAA", ts, nonce, "F464B24FC39322E44B38AA78F5EDD27BD1441696"},
		{"token 不同", "AAAAAB", ts, nonce, sig},
		{"token 为空", "", ts, nonce, sig},
		{"timestamp 不同", "AAAAA", "1" + ts, nonce, sig},
		{"nonce 不同", "AAAAA", ts, "9" + nonce, sig},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if verifySignature(c.token, c.ts, c.nonce, c.sig) {
				t.Fatalf("不该验过：%+v", c)
			}
		})
	}
}
