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
// ⚠️ 消息推送的验签**不是** HMAC、也不使用 AppKey，见 notify_verify.go。

// hmacSHA256Hex 计算 HMAC-SHA256 并返回小写十六进制字符串。
func hmacSHA256Hex(key, message string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// CalcPaySig 计算支付签名 pay_sig。
//
// 入参：
//
//	appKey    商家密钥（商户后台里那把）。**必须与被签内容的环境配套**：env=0 的订单配
//	          现网 AppKey、env=1 的配沙箱 AppKey，两把不能混。
//	uri       签名原文里 uri 那一段，**不带** "?" 及其后的部分。拉起支付时固定
//	          "requestVirtualPayment"，调服务端接口时是接口路径（如 "/xpay/query_order"）。
//	          带上 query string 会让签名与微信侧不一致（服务端报 268490003）。
//	signData  被签名的原文：下单时是完整的 signData 字符串，调接口时是请求体。它必须与
//	          实际下发/发送的字符串**字节级一致**——这里算的是字节，微信校验的也是字节，
//	          中间任何一次重新序列化（换了字段顺序、多了转义）都会让两边对不上。
//
// 多数调用方不用直接调它：BuildPayment 与 PostWithPaySig 内部已经算了。
func CalcPaySig(appKey, uri, signData string) string {
	return hmacSHA256Hex(appKey, uri+"&"+signData)
}

// CalcSignature 计算用户态签名 signature。
//
// 与 pay_sig 的唯一区别是**不拼 uri 前缀**（公式见文件头）。
//
// 入参：
//
//	sessionKey  用户会话密钥，由 wx.login 的 code 经 Code2Session 换取。它是**会话级**的、
//	            会过期——过期后服务端报 268490009、客户端报 -15007，届时让前端重新 wx.login。
//	signData    被签名的原文，与 CalcPaySig 同样是「字节级一致」的要求。
//
// 多数调用方不用直接调它：BuildPayment 与 PostWithUserSig 内部已经算了。
func CalcSignature(sessionKey, signData string) string {
	return hmacSHA256Hex(sessionKey, signData)
}
