package wechat_virtualpay_go

import (
	"errors"
	"fmt"
)

// APIError 表示微信接口返回的业务错误。
//
// 微信开放接口的成功响应里没有 errcode 字段；一旦出现非 0 的 errcode，
// 即表示失败，本包会将其转换为 *APIError 返回。
type APIError struct {
	Code    int    // 微信 errcode
	Message string // 微信 errmsg
	Raw     []byte // 原始响应体，便于排查
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wechat_virtualpay_go: 微信接口错误 errcode=%d errmsg=%s", e.Code, e.Message)
}

// Hint 针对高频错误码给出排查提示，便于直接打日志或返回给调用方。
func (e *APIError) Hint() string {
	switch e.Code {
	case ErrCodeSignatureUser:
		return "用户签名（signature）错误：检查 session_key 是否最新；session_key 由 code2Session 获取，会过期"
	case ErrCodeSignaturePay:
		return "支付签名（pay_sig）错误：检查 AppKey 与环境是否匹配、签名算法是否为 HMAC-SHA256(appKey, method+\"&\"+signData)、以及 signData 是否与发送的字符串字节级一致"
	case ErrCodeSessionKeyExpired:
		return "session_key 已过期：重新调用 wx.login 并 code2Session 刷新登录态"
	case ErrCodeEnvMismatch:
		return "环境不匹配：现网版本请求体的 env 必须为 0"
	case ErrCodeOutTradeNoDuplicate:
		return "订单号重复：outTradeNo 每个订单只能使用一次"
	case ErrCodeGoodsPriceMismatch:
		return "道具价格错误：goodsPrice 必须与后台配置的道具价格一致"
	case ErrCodeProductNotPublished:
		return "道具未发布：productId 对应的道具尚未在虚拟支付后台发布"
	case ErrCodeMerchantRestricted:
		return "商户涉嫌违规，收款功能已被限制"
	default:
		return ""
	}
}

// 常见错误码（命名以便业务侧 switch / errors.Is）。
//
// 完整列表以官方文档为准，这里收录了接入阶段最容易遇到的若干。
const (
	ErrCodeParamError          = -15001 // 参数错误
	ErrCodeOutTradeNoDuplicate = -15002 // outTradeNo 重复使用
	ErrCodeSystemError         = -15003 // 系统错误
	ErrCodeCurrencyType        = -15004 // currencyType 错误（目前只支持 CNY）
	ErrCodeSignatureUser       = -15005 // 用户态签名 signature 错误
	ErrCodeSignaturePay        = -15006 // 支付签名 pay_sig 错误
	ErrCodeSessionKeyExpired   = -15007 // session_key 过期
	ErrCodeSubMerchantInvalid  = -15008 // 二级商户进件未完成
	ErrCodeCoinNotPublished    = -15009 // 代币未发布
	ErrCodeProductNotPublished = -15010 // 道具 productId 未发布
	ErrCodeEnvMismatch         = -15011 // 现网版本 env 只能为 0
	ErrCodeMidasFailed         = -15012 // 调用米大师失败导致关单
	ErrCodeGoodsPriceMismatch  = -15013 // goodsPrice 道具价格错误
	ErrCodePublishNotEffective = -15014 // 道具/代币发布未生效（约 10 分钟后生效）
	ErrCodeSignDataFormat      = -15016 // signData 格式有问题
	ErrCodeMerchantRestricted  = -15017 // 商家涉嫌违规，收款功能被限制
)

// ErrInvalidSignature 表示回调验签失败。
var ErrInvalidSignature = errors.New("wechat_virtualpay_go: 回调验签失败")

// IsCode 判断 err 是否为指定的微信错误码。
func IsCode(err error, code int) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Code == code
	}
	return false
}
