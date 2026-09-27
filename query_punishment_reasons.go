package wechat_virtualpay_go

import "context"

// RecoverySpecification 是一条管控原因及其解脱路径。
type RecoverySpecification struct {
	// LimitationCaseID 该条管控原因对应的单据号，可与管控流水通知中的 BusinessCode 关联。
	LimitationCaseID string `json:"limitation_case_id"`
	// LimitationReasonType 该条管控原因所属的类型。
	LimitationReasonType string `json:"limitation_reason_type"`
	// LimitationReason 该条管控原因的简要描述。
	LimitationReason string `json:"limitation_reason"`
	// LimitationReasonDescribe 该条管控原因的进一步说明。
	LimitationReasonDescribe string `json:"limitation_reason_describe"`
	// RelateLimitations 在该条管控原因下具体受影响的能力列表。
	RelateLimitations string `json:"relate_limitations"`
	// OtherRelateLimitations 未被标准枚举覆盖的受影响能力的补充说明。
	OtherRelateLimitations string `json:"other_relate_limitations"`
	// RecoverWay 微信支付建议的处理路径。
	RecoverWay string `json:"recover_way"`
	// RecoverWayParam 解脱路径对应的补充参数（尽调单号、申诉单号等）。
	RecoverWayParam string `json:"recover_way_param"`
	// RecoverHelpURL 微信支付提供的进一步说明页面。
	RecoverHelpURL string `json:"recover_help_url"`
	// LimitationActionType 该条管控原因对应的管控生效方式。
	LimitationActionType string `json:"limitation_action_type"`
	// LimitationStartDate 处置方式为延迟管控时返回的预计开始时间。
	LimitationStartDate string `json:"limitation_start_date"`
	// LimitationDate 该条管控原因实际生效的时间。
	LimitationDate string `json:"limitation_date"`
}

// QueryPunishmentReasonsResponse 是查询商户被管控原因的响应。
type QueryPunishmentReasonsResponse struct {
	// AppID 小程序 AppID。
	AppID string `json:"appid"`
	// NickName 小程序昵称。
	NickName string `json:"nickname"`
	// MerchantCode 微信支付商户号。
	MerchantCode string `json:"merchant_code"`
	// LimitedFunctions 商户被管控能力列表。
	LimitedFunctions []string `json:"limited_functions"`
	// OtherLimitedFunctions 其他被管控能力描述。
	OtherLimitedFunctions string `json:"other_limited_functions"`
	// RecoverySpecifications 被管控原因及解脱路径列表。
	RecoverySpecifications []RecoverySpecification `json:"recovery_specifications"`
}

// QueryPunishmentReasons 查询商户被微信支付管控的原因。
//
// ⚠️ 本接口的官方文档写明「请求体：无」，但 query 里又要求 pay_sig。签名是对请求体
// 计算的，所以这里传一个空结构体、序列化为 `{}` 参与签名（与真正发出的字节一致）。
// 若实测签名不通过，多半是微信期望的是一段完全为空的 body——改这里即可。
//
// 官方文档：POST /xpay/query_punishment_reasons
func (c *Client) QueryPunishmentReasons(ctx context.Context) (*QueryPunishmentReasonsResponse, error) {
	var resp QueryPunishmentReasonsResponse
	if err := c.call(ctx, "/xpay/query_punishment_reasons", struct{}{}, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
