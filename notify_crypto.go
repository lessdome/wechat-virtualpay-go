package wechat_virtualpay_go

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"sort"
	"strings"
)

// 本文件实现微信「消息推送配置」的验签。
//
// 这套机制**与支付签名毫无关系**，不要混淆：
//
//	signature 用 SHA-1，密钥是**开发者在 MP 后台自填的 Token**；
//	pay_sig / signature（支付那两个）用 HMAC-SHA256，密钥是 AppKey / session_key。
//
// 本包只支持**明文模式**，因此只有下面这一种验签：
//
//	明文模式   signature = sha1( sort([Token, timestamp, nonce]).join("") )
//
// 安全模式用的是另一套（msg_signature + AES 解密），本包不支持——MP 后台的
// 「消息加解密方式」请选**明文模式**。

// sha1SortedHex 把若干字符串按字典序排序后拼接，取 SHA-1 的十六进制。
//
// 这是微信消息推送验签的核心算法。注意是**字符串拼接**（无分隔符），
// 且排序是逐字节的字典序（Go 的 sort.Strings 即如此）。
func sha1SortedHex(parts ...string) string {
	sort.Strings(parts)
	h := sha1.New()
	// 微信规定如此，非安全问题。
	h.Write([]byte(strings.Join(parts, ""))) //nolint:gosec
	return hex.EncodeToString(h.Sum(nil))
}

// verifySignature 校验推送请求上的 signature。
func verifySignature(notifyToken, timestamp, nonce, signature string) bool {
	expected := sha1SortedHex(notifyToken, timestamp, nonce)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}
