package wechat_virtualpay_go

import (
	"errors"
	"fmt"
)

// APIError 表示一次调用失败。
//
// 两种失败来源，用字段区分开：
//   - 微信返回了业务错误码 → Code 非 0，HTTPStatus 为 0
//   - HTTP 层就失败了（如 500、502）→ HTTPStatus 非 0，Code 为 0
//
// 分开是因为两者是**不同的命名空间**：微信的业务码是 268490xxx，而 HTTP 状态码
// 是 500 之类，混在一个字段里会让 IsCode 误命中。
type APIError struct {
	// Code 微信业务错误码。为 0 表示这不是业务错误（见 HTTPStatus）。
	Code int
	// Message 微信返回的 errmsg；HTTP 层失败时是状态描述。
	Message string
	// HTTPStatus HTTP 状态码。为 0 表示这不是 HTTP 层失败。
	HTTPStatus int
	// Raw 原始响应体，便于排查。
	Raw []byte
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("wechat_virtualpay_go: HTTP %d %s", e.HTTPStatus, e.Message)
	}
	return fmt.Sprintf("wechat_virtualpay_go: 微信接口错误 errcode=%d errmsg=%s", e.Code, e.Message)
}

// Hint 针对高频错误码给出排查方向，便于直接打日志。
//
// 只覆盖接入阶段最容易遇到的几个；其余返回空串。
func (e *APIError) Hint() string {
	switch e.Code {
	case ErrCodeSignature:
		return "签名错误：检查 AppKey 是否正确、签名算法是否为 HMAC-SHA256(appKey, uri+\"&\"+请求体)、以及参与签名的字符串是否与真正发出的请求体字节级一致（最常见的失配来源）"
	case ErrCodeSessionKeyExpired:
		return "session_key 已过期：让前端重新 wx.login，再用 code 换新的 session_key"
	case ErrCodeInvalidParam:
		return "请求参数字段错误：具体看 errmsg，常见是必填项缺失或字段格式不符"
	case ErrCodeInvalidOpenID:
		return "openid 错误：确认 openid 属于当前 AppID，且与下单时用的是同一个"
	case ErrCodeDuplicateOperation:
		return "重复操作：微信表示之前的同一次操作已经成功，通常可直接当作成功处理（例如赠送、代币支付、广告金充值）"
	case ErrCodeCoinNotPublished:
		return "代币未发布：先在虚拟支付后台发布代币"
	case ErrCodeLeftFeeMismatch:
		return "退款金额与剩余可退不符：先用 QueryOrder 查 order.left_fee，再按它填 RefundFee"
	case ErrCodeRefundInProgress:
		return "退款进行中：稍后用相同参数重试即可"
	case ErrCodeRateLimited:
		return "触发频率限制：降低调用频率后重试"
	}
	return ""
}

// 微信虚拟支付涉及**两套不同的错误码**，分别出自两份官方文档，不要混用：
//
//	服务端接口（/xpay/*）  → 本库发起的调用，返回 268490xxx（外加 -1），即下面这些
//	小程序端拉起支付        → wx.requestVirtualPayment 的 fail 回调，返回 -150xx
//
// 后者由前端拿到，本库不返回也不定义常量（需要时见官方《wx.requestVirtualPayment》
// 一页的「错误」表）；但排查支付问题时两边常要对照，所以在此记一笔。

// 服务端接口（/xpay/*）返回的业务错误码。
// 取自 33 个接口页各自的「错误码」表。
const (
	// ErrCodeSystemError 系统错误。
	ErrCodeSystemError = -1

	ErrCodeInvalidOpenID        = 268490001 // openid 错误
	ErrCodeInvalidParam         = 268490002 // 请求参数字段错误，具体看 errmsg
	ErrCodeSignature            = 268490003 // 签名错误
	ErrCodeDuplicateOperation   = 268490004 // 重复操作（赠送、代币支付、充值广告金相关接口会返回，表示之前的操作已经成功）
	ErrCodeOrderAlreadyRefunded = 268490005 // 订单已经通过 cancel_currency_pay 接口退款，不支持再退款
	ErrCodeAmountInsufficient   = 268490006 // 代币的退款/支付操作金额不足
	ErrCodeSensitiveContent     = 268490007 // 图片或文字存在敏感内容，禁止使用
	ErrCodeCoinNotPublished     = 268490008 // 代币未发布，不允许进行代币操作
	ErrCodeSessionKeyExpired    = 268490009 // 用户 session_key 不存在或已过期，请重新登录
	ErrCodeDataGenerating       = 268490011 // 数据生成中，请稍后调用本接口获取
	ErrCodeBatchTaskRunning     = 268490012 // 批量任务运行中，请等待完成后才能再次运行
	ErrCodeRefundNotAllowed     = 268490013 // 禁止对核销状态的单进行退款
	ErrCodeRefundInProgress     = 268490014 // 退款操作进行中，稍后可以使用相同参数重试
	ErrCodeRateLimited          = 268490015 // 频率限制
	ErrCodeLeftFeeMismatch      = 268490016 // 退款的 left_fee 字段与实际不符，请通过 query_order 接口查询确认

	ErrCodeAdFundIndustryMismatch = 268490018 // 广告金充值帐户行业 id 不匹配
	ErrCodeAdFundAccountBound     = 268490019 // 广告金充值帐户 id 已绑定其他 appid
	ErrCodeAdFundNameMismatch     = 268490020 // 广告金充值帐户主体名称错误
	ErrCodeAccountNotOnboarded    = 268490021 // 账户未完成进件
	ErrCodeAdFundAccountInvalid   = 268490022 // 广告金充值账户无效
	ErrCodeAdFundInsufficient     = 268490023 // 广告金余额不足
	ErrCodeAdFundAmountInvalid    = 268490024 // 广告金充值金额必须大于 0
)

// ErrInvalidSignature 表示回调验签失败。
var ErrInvalidSignature = errors.New("wechat_virtualpay_go: 回调验签失败")

// IsCode 判断 err 是否为指定的微信业务错误码。
func IsCode(err error, code int) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Code == code
	}
	return false
}
