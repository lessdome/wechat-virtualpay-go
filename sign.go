package virtualpay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// 虚拟支付的三个签名，算法都是 HMAC-SHA256，输出小写十六进制（64 字符）。
//
//	pay_sig       = hex( HMAC-SHA256( AppKey,     method + "&" + signData ) )
//	signature     = hex( HMAC-SHA256( sessionKey, signData ) )
//	pay_event_sig = hex( HMAC-SHA256( AppKey,     event  + "&" + payload  ) )
//
// 注意 pay_sig 与 signature 的差异：pay_sig 会拼上 method，signature 不会。
// 这是最容易写错的地方——两者必须分别实现，不能共用一个函数。

// hmacSHA256Hex 计算 HMAC-SHA256 并返回小写十六进制字符串。
func hmacSHA256Hex(key, message string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// CalcPaySig 计算支付签名 pay_sig。
//
// 参数：
//   - appKey:   虚拟支付支付密钥（Env=Production 用现网 Key，Env=Sandbox 用沙箱 Key）
//   - method:   接口方法名。拉起支付时固定为 "requestVirtualPayment"；
//     其他接口用其路径（如 "/xpay/start_upload_goods"）。
//     注意：method 不能带查询参数（"?" 及其后内容要舍去）。
//   - signData: 待签名的原始 JSON 字符串（必须与实际下发/发送的字符串字节级一致）。
//
// 返回 64 位小写十六进制字符串。
func CalcPaySig(appKey, method, signData string) string {
	return hmacSHA256Hex(appKey, method+"&"+signData)
}

// CalcSignature 计算用户签名 signature。
//
// 与 pay_sig 不同，signature 不带 method 前缀，直接用 sessionKey 对 signData 签名。
// sessionKey 由 wx.login 的 code 通过 code2Session 换取，会过期（对应错误码 -15007）。
func CalcSignature(sessionKey, signData string) string {
	return hmacSHA256Hex(sessionKey, signData)
}

// CalcPayEventSig 计算回调事件签名 pay_event_sig。
//
//   - event:   事件类型，如 "xpay_goods_deliver_notify"
//   - payload: 推送数据原文
func CalcPayEventSig(appKey, event, payload string) string {
	return hmacSHA256Hex(appKey, event+"&"+payload)
}

// VerifyPayEventSig 校验回调事件签名。
//
// 使用 hmac.Equal 做恒定时间比较，避免时序侧信道。
func VerifyPayEventSig(appKey, event, payload, signature string) bool {
	expected := CalcPayEventSig(appKey, event, payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}
