package wechat_virtualpay_go

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"sort"
	"strings"
)

// 本文件实现微信「消息推送」的验签。
//
// ⚠️ 它与支付签名**毫无关系**，别混：验签用 SHA-1，密钥是 MP 后台「消息推送配置」里
// 自填的 Token；而 pay_sig / signature 用 HMAC-SHA256，密钥是 AppKey / session_key。
//
// 算法（官方《消息推送》页）：
//
//	把 Token、timestamp、nonce 三个参数按字典序排序，拼接成一个字符串，做 sha1。
//
// 本包只支持**明文模式**，所以只有这一种验签。安全模式是另一套（msg_signature +
// Encrypt + AES 解密），本包不支持——MP 后台的「消息解密方式」请选明文模式。

// verifySignature 校验推送请求 URL 上的 signature。
//
// 两处容易写错：排序是**逐字节字典序**（数字排在字母之前），拼接时**不加任何分隔符**。
func verifySignature(token, timestamp, nonce, signature string) bool {
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	// 定长比较：不因为前半段相同就提前返回。
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(signature)) == 1
}
