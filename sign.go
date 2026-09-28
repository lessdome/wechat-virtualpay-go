package wechat_virtualpay_go

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// 虚拟支付涉及两个 HMAC-SHA256 签名，输出 64 位小写十六进制（官方 2.5「签名详解」）：
//
//	pay_sig   = hex( HMAC-SHA256( appKey,     uri + "&" + signData ) )
//	signature = hex( HMAC-SHA256( sessionKey, signData ) )
//
// 唯一的区别是 pay_sig 要拼上 uri、signature 不拼——这是最容易写错的地方，所以
// 两个函数分开实现，不共用一个带开关的内部函数。
//
// uri 的取值：拉起支付时固定 "requestVirtualPayment"；调服务端接口时是接口路径
// （如 "/xpay/query_order"），**不带** "?" 及其后的 query string。
//
// ⚠️ 消息推送的验签**不是** HMAC、也不使用 AppKey，见 notify_crypto.go。

// hmacSHA256Hex 计算 HMAC-SHA256 并返回小写十六进制字符串。
func hmacSHA256Hex(key, message string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// CalcPaySig 计算支付签名 pay_sig。
//
// uri **不带** "?" 及其后的部分：pay_sig 的签名原文是 uri + "&" + signData，
// uri 带上 query string 会让签名与微信侧不一致（服务端报 268490003）。
//
// signData 必须与实际下发/发送的字符串**字节级一致**，见 json.go 的说明。
func CalcPaySig(appKey, uri, signData string) string {
	return hmacSHA256Hex(appKey, uri+"&"+signData)
}

// CalcSignature 计算用户态签名 signature。
//
// 与 pay_sig 不同，它**不带** uri 前缀。sessionKey 由 wx.login 的 code 通过
// code2Session 换取，会过期（服务端报 268490009）。
func CalcSignature(sessionKey, signData string) string {
	return hmacSHA256Hex(sessionKey, signData)
}
