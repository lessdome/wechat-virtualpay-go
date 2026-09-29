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
//
// ⚠️ **它证明了什么、没证明什么**：签名原文只有 Token、timestamp、nonce 三个 URL 参数，
// **报文体不参与**。所以验签通过只说明「请求来自知道 Token 的一方」（即微信），
// **不等于**报文可信：
//
//   - **可重放**：这组三元组就摆在请求 URL 上，一旦落进访问日志、代理日志或浏览器历史，
//     拿到它的人就能反复重发，并任意替换 body——比如发一条自造的
//     xpay_goods_deliver_notify 让你重复发货。
//   - **本包不做时间窗校验**：微信的重试跨 2、4、8…最多 15 次，合计可达数小时；若重试
//     复用同一组 timestamp/nonce，时间窗会把合法重试一并挡掉，弊大于利，所以不加。
//
// 结论：**幂等与订单归属校验是调用方的责任，且不可省**——发货前务必用
// WeChatPayInfo.MchOrderNo 去重，并确认该订单在你自己库里真实存在、金额对得上。
// 别把「验签通过」当成「报文可信」。

// verifySignature 校验推送请求 URL 上的 signature。
//
// 两处容易写错：排序是**逐字节字典序**（数字排在字母之前），拼接时**不加任何分隔符**。
//
// 注意它签的只有这三个 URL 参数，**报文体不在其中**——见本文件开头的说明。
func verifySignature(token, timestamp, nonce, signature string) bool {
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	// 定长比较：不因为前半段相同就提前返回。
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(signature)) == 1
}
