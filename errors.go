package wechat_virtualpay_go

import "fmt"

// 微信虚拟支付涉及**两套不同的错误码**，分别出自两份官方文档，不要混用：
//
//	服务端接口（/xpay/*）  → 本库发起的调用，返回 268490xxx（外加 -1），即下面这些
//	小程序端拉起支付        → wx.requestVirtualPayment 的 fail 回调，返回 -150xx
//
// 后者由前端拿到，本库不返回也不定义常量（需要时见官方《wx.requestVirtualPayment》
// 一页的「错误」表）；但排查支付问题时两边常要对照，所以在此记一笔。
//
// 另外，微信所有开放接口还有一层《通用错误码》（40001、42001 之类），本库不定义，
// 遇到时请查官方那一页。

// ErrorCode 是服务端接口（/xpay/*）返回的**业务错误码**。
//
// 常量见下方。每个码的中文说明用 ErrorCode.ErrorText() 取——接口失败时它也会被
// 拼进错误文案里。
type ErrorCode int

// 服务端接口（/xpay/*）返回的业务错误码。取自 33 个接口页各自的「错误码」表。
const (
	// ErrCodeSystemError 系统错误。
	ErrCodeSystemError ErrorCode = -1

	ErrCodeInvalidOpenID        ErrorCode = 268490001 // openid 错误
	ErrCodeInvalidParam         ErrorCode = 268490002 // 请求参数字段错误，具体看 errmsg
	ErrCodeSignature            ErrorCode = 268490003 // 签名错误
	ErrCodeDuplicateOperation   ErrorCode = 268490004 // 重复操作
	ErrCodeOrderAlreadyRefunded ErrorCode = 268490005 // 订单已经通过 cancel_currency_pay 接口退款，不支持再退款
	ErrCodeAmountInsufficient   ErrorCode = 268490006 // 代币的退款/支付操作金额不足
	ErrCodeSensitiveContent     ErrorCode = 268490007 // 图片或文字存在敏感内容，禁止使用
	ErrCodeCoinNotPublished     ErrorCode = 268490008 // 代币未发布，不允许进行代币操作
	ErrCodeSessionKeyExpired    ErrorCode = 268490009 // 用户 session_key 不存在或已过期，请重新登录
	ErrCodeDataGenerating       ErrorCode = 268490011 // 数据生成中，请稍后调用本接口获取
	ErrCodeBatchTaskRunning     ErrorCode = 268490012 // 批量任务运行中，请等待完成后才能再次运行
	ErrCodeRefundNotAllowed     ErrorCode = 268490013 // 禁止对核销状态的单进行退款
	ErrCodeRefundInProgress     ErrorCode = 268490014 // 退款操作进行中，稍后可以使用相同参数重试
	ErrCodeRateLimited          ErrorCode = 268490015 // 频率限制
	ErrCodeLeftFeeMismatch      ErrorCode = 268490016 // 退款的 left_fee 字段与实际不符，请通过 query_order 接口查询确认

	ErrCodeAdFundIndustryMismatch ErrorCode = 268490018 // 广告金充值帐户行业 id 不匹配
	ErrCodeAdFundAccountBound     ErrorCode = 268490019 // 广告金充值帐户 id 已绑定其他 appid
	ErrCodeAdFundNameMismatch     ErrorCode = 268490020 // 广告金充值帐户主体名称错误
	ErrCodeAccountNotOnboarded    ErrorCode = 268490021 // 账户未完成进件
	ErrCodeAdFundAccountInvalid   ErrorCode = 268490022 // 广告金充值账户无效
	ErrCodeAdFundInsufficient     ErrorCode = 268490023 // 广告金余额不足
	ErrCodeAdFundAmountInvalid    ErrorCode = 268490024 // 广告金充值金额必须大于 0
)

// ErrorText 返回这个错误码对应的中文说明。
//
// 大部分就是官方错误码表里的原文；少数几个在原文后面补了一句排查方向。
// 库中未收录的码返回一句兜底说明——微信将来可能新增码，得让它也能带上数字。
func (c ErrorCode) ErrorText() string {
	switch c {
	case ErrCodeSystemError:
		return "系统错误"
	case ErrCodeInvalidOpenID:
		return "openid 错误——确认它属于当前 AppID，且与下单时用的是同一个"
	case ErrCodeInvalidParam:
		return "请求参数字段错误，具体看 errmsg——常见是必填项缺失或字段格式不符"
	case ErrCodeSignature:
		return "签名错误——检查 AppKey 是否正确、签名算法是否为 HMAC-SHA256(appKey, uri+\"&\"+请求体)、以及参与签名的字符串是否与真正发出的请求体字节级一致（最常见的失配来源）"
	case ErrCodeDuplicateOperation:
		return "重复操作——微信表示之前的同一次操作已经成功，通常可直接当作成功处理（例如赠送、代币支付、广告金充值）"
	case ErrCodeOrderAlreadyRefunded:
		return "订单已经通过 cancel_currency_pay 接口退款，不支持再退款"
	case ErrCodeAmountInsufficient:
		return "代币的退款/支付操作金额不足"
	case ErrCodeSensitiveContent:
		return "图片或文字存在敏感内容，禁止使用"
	case ErrCodeCoinNotPublished:
		return "代币未发布，不允许进行代币操作——先在虚拟支付后台发布代币"
	case ErrCodeSessionKeyExpired:
		return "用户 session_key 不存在或已过期，请重新登录——让前端重新 wx.login，再用 code 换新的 session_key"
	case ErrCodeDataGenerating:
		return "数据生成中，请稍后调用本接口获取"
	case ErrCodeBatchTaskRunning:
		return "批量任务运行中，请等待完成后才能再次运行"
	case ErrCodeRefundNotAllowed:
		return "禁止对核销状态的单进行退款"
	case ErrCodeRefundInProgress:
		return "退款操作进行中，用相同参数稍后重试即可"
	case ErrCodeRateLimited:
		return "频率限制——降低调用频率后重试"
	case ErrCodeLeftFeeMismatch:
		return "退款的 left_fee 字段与实际不符——先用 QueryOrder 查 order.left_fee，再按它填 RefundFee"
	case ErrCodeAdFundIndustryMismatch:
		return "广告金充值帐户行业 id 不匹配"
	case ErrCodeAdFundAccountBound:
		return "广告金充值帐户 id 已绑定其他 appid"
	case ErrCodeAdFundNameMismatch:
		return "广告金充值帐户主体名称错误"
	case ErrCodeAccountNotOnboarded:
		return "账户未完成进件"
	case ErrCodeAdFundAccountInvalid:
		return "广告金充值账户无效"
	case ErrCodeAdFundInsufficient:
		return "广告金余额不足"
	case ErrCodeAdFundAmountInvalid:
		return "广告金充值金额必须大于 0"
	}
	return fmt.Sprintf("未知错误码 %d（本库未收录，见官方文档的错误码表）", int(c))
}
