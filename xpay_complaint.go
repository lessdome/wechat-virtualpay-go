package wechat_virtualpay_go

import (
	"context"
)

// ComplaintState 是投诉状态。
type ComplaintState string

const (
	ComplaintStatePending    ComplaintState = "PENDING"    // 待处理
	ComplaintStateProcessing ComplaintState = "PROCESSING" // 处理中
	ComplaintStateProcessed  ComplaintState = "PROCESSED"  // 已处理完成
)

// ProblemType 是用户投诉的问题类型。
type ProblemType string

const (
	// ProblemTypeRefund 申请退款。这类单据需要**最高优先处理**。
	ProblemTypeRefund ProblemType = "REFUND"
	// ProblemTypeServiceNotWork 服务权益未生效。
	ProblemTypeServiceNotWork ProblemType = "SERVICE_NOT_WORK"
	// ProblemTypeOthers 其他类型。
	ProblemTypeOthers ProblemType = "OTHERS"
)

// UserTag 是投诉人标签。
type UserTag string

const (
	// UserTagTrusted 该用户满足极速退款条件。
	UserTagTrusted UserTag = "TRUSTED"
	// UserTagHighRisk 高风险投诉，需按运营要求优先妥善处理。
	UserTagHighRisk UserTag = "HIGH_RISK"
)

// ComplaintMediaType 是投诉相关媒体的业务类型。
type ComplaintMediaType string

const (
	ComplaintMediaUserImage      ComplaintMediaType = "USER_COMPLAINT_IMAGE" // 用户提交投诉时上传的图片凭证
	ComplaintMediaOperationImage ComplaintMediaType = "OPERATION_IMAGE"      // 协商解决投诉时上传的图片凭证
)

// ServiceOrderState 是服务订单状态（投诉单里记录的是用户投诉发起那一刻的快照，不实时更新）。
type ServiceOrderState string

const (
	ServiceOrderDoing   ServiceOrderState = "DOING"   // 服务订单进行中
	ServiceOrderRevoked ServiceOrderState = "REVOKED" // 服务订单已取消
	ServiceOrderWaitPay ServiceOrderState = "WAITPAY" // 服务订单待支付
	ServiceOrderDone    ServiceOrderState = "DONE"    // 服务订单已完成
)

// ComplaintOrderInfo 是投诉单关联的订单信息。
type ComplaintOrderInfo struct {
	// TransactionID 微信支付交易单号。
	TransactionID string `json:"transaction_id"`
	// OutTradeNo 渠道单号，即 QueryOrder 返回的 channel_order_id。
	OutTradeNo string `json:"out_trade_no"`
	// Amount 订单金额，单位分。
	Amount int64 `json:"amount"`
	// WxaOutTradeNo 商户单号，商家在拉起支付时传的单号。
	WxaOutTradeNo string `json:"wxa_out_trade_no"`
	// WxOrderID 小程序侧单号。
	WxOrderID string `json:"wx_order_id"`
}

// ComplaintMedia 是一条投诉相关资料。
type ComplaintMedia struct {
	// MediaType 媒体文件对应的业务类型。
	MediaType ComplaintMediaType `json:"media_type"`
	// MediaURL 媒体文件请求 URL。
	MediaURL []string `json:"media_url"`
}

// ServiceOrderInfo 是投诉单关联的服务单信息。
type ServiceOrderInfo struct {
	// OrderID 微信支付服务订单号。
	OrderID string `json:"order_id"`
	// OutOrderNo 商户系统内部服务订单号（不是交易单号）。
	OutOrderNo string `json:"out_order_no"`
	// State 用户发起投诉那一刻的服务单状态快照，不会实时更新。
	State ServiceOrderState `json:"state"`
}

// Complaint 是一条用户投诉。
type Complaint struct {
	// ComplaintID 投诉 ID。
	ComplaintID string `json:"complaint_id"`
	// ComplaintTime 投诉时间。
	ComplaintTime string `json:"complaint_time"` // 格式 yyyy-mm-dd'T'HH:MM:ssXXX
	// ComplaintDetail 投诉内容。
	ComplaintDetail string `json:"complaint_detail"`
	// ComplaintState 投诉状态。
	ComplaintState ComplaintState `json:"complaint_state"`
	// PayerPhone 投诉人联系方式。
	PayerPhone string `json:"payer_phone"`
	// PayerOpenID 投诉人在商户 AppID 下的唯一标识。
	PayerOpenID string `json:"payer_openid"`
	// ComplaintOrderInfo 投诉单关联的订单信息。
	ComplaintOrderInfo []ComplaintOrderInfo `json:"complaint_order_info"`
	// ComplaintFullRefunded 投诉单下所有订单是否已全部全额退款。
	ComplaintFullRefunded bool `json:"complaint_full_refunded"`
	// IncomingUserResponse 是否有待回复的用户留言。
	IncomingUserResponse bool `json:"incoming_user_response"`
	// UserComplaintTimes 用户投诉次数：首次记为 1，每继续投诉一次加 1。
	UserComplaintTimes int `json:"user_complaint_times"`
	// ComplaintMediaList 用户上传的投诉相关资料（图片凭证等）。
	ComplaintMediaList []ComplaintMedia `json:"complaint_media_list"`
	// ProblemDescription 用户发起投诉前选择的 FAQ 标题。
	ProblemDescription string `json:"problem_description"`
	// ProblemType 问题类型。ProblemTypeRefund 需最高优先处理。
	ProblemType ProblemType `json:"problem_type"`
	// ApplyRefundAmount 当问题类型为申请退款时有值，单位分。
	ApplyRefundAmount int64 `json:"apply_refund_amount"`
	// UserTagList 用户标签列表。
	UserTagList []UserTag `json:"user_tag_list"`
	// ServiceOrderInfo 投诉单关联的服务单信息。
	ServiceOrderInfo []ServiceOrderInfo `json:"service_order_info"`
}

// GetComplaintListRequest 是获取投诉列表的请求。
type GetComplaintListRequest struct {
	// BeginDate 筛选开始时间，格式 yyyy-mm-dd，如 "2023-01-01"。
	BeginDate string `json:"begin_date"`
	// EndDate 筛选结束时间，格式 yyyy-mm-dd。
	EndDate string `json:"end_date"`
	// Offset 筛选偏移，从 0 开始。
	Offset int `json:"offset"`
	// Limit 最多返回条数。
	Limit int `json:"limit"`
	envField
}

// GetComplaintListResponse 是获取投诉列表的响应。
type GetComplaintListResponse struct {
	// Total 总条数。
	Total int `json:"total"`
	// Complaints 投诉列表。
	Complaints []Complaint `json:"complaints"`
}

// GetComplaintList 获取投诉列表。
//
// 官方文档：POST /xpay/get_complaint_list
func (c *Client) GetComplaintList(ctx context.Context, req GetComplaintListRequest) (*GetComplaintListResponse, error) {
	var resp GetComplaintListResponse
	if err := c.call(ctx, "/xpay/get_complaint_list", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetComplaintDetailRequest 是获取投诉详情的请求。
type GetComplaintDetailRequest struct {
	// ComplaintID 投诉 ID，由 GetComplaintList 返回。
	ComplaintID string `json:"complaint_id"`
	envField
}

// GetComplaintDetailResponse 是获取投诉详情的响应。
type GetComplaintDetailResponse struct {
	// Complaint 投诉详情，字段与 GetComplaintList 的单项一致。
	Complaint *Complaint `json:"complaint"`
}

// GetComplaintDetail 获取投诉详情。
//
// 官方文档：POST /xpay/get_complaint_detail
func (c *Client) GetComplaintDetail(ctx context.Context, req GetComplaintDetailRequest) (*GetComplaintDetailResponse, error) {
	var resp GetComplaintDetailResponse
	if err := c.call(ctx, "/xpay/get_complaint_detail", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// OperateType 是投诉协商记录的操作类型。
type OperateType string

const (
	OpUserCreateComplaint       OperateType = "USER_CREATE_COMPLAINT"                     // 用户提交投诉
	OpUserContinueComplaint     OperateType = "USER_CONTINUE_COMPLAINT"                   // 用户继续投诉
	OpUserResponse              OperateType = "USER_RESPONSE"                             // 用户留言
	OpPlatformResponse          OperateType = "PLATFORM_RESPONSE"                         // 平台留言
	OpMerchantResponse          OperateType = "MERCHANT_RESPONSE"                         // 商户留言
	OpMerchantConfirmComplete   OperateType = "MERCHANT_CONFIRM_COMPLETE"                 // 商户申请结单
	OpUserCreateComplaintSystem OperateType = "USER_CREATE_COMPLAINT_SYSTEM_MESSAGE"      // 用户提交投诉系统通知
	OpFullRefundedSystem        OperateType = "COMPLAINT_FULL_REFUNDED_SYSTEM_MESSAGE"    // 投诉单发起全额退款系统通知
	OpPartialRefundedSystem     OperateType = "COMPLAINT_PARTIAL_REFUNDED_SYSTEM_MESSAGE" // 投诉单发起部分退款系统通知
	OpRefundReceivedSystem      OperateType = "COMPLAINT_REFUND_RECEIVED_SYSTEM_MESSAGE"  // 投诉单退款到账系统通知
	OpUserContinueSystem        OperateType = "USER_CONTINUE_COMPLAINT_SYSTEM_MESSAGE"    // 用户继续投诉系统通知
	OpUserRevokeComplaint       OperateType = "USER_REVOKE_COMPLAINT"                     // 用户主动撤诉（仅历史投诉单）
	OpUserConfirmComplaint      OperateType = "USER_COMFIRM_COMPLAINT"                    // 用户确认投诉解决（仅历史投诉单，微信原文如此拼写）
	OpPlatformHelpApplication   OperateType = "PLATFORM_HELP_APPLICATION"                 // 平台催办
	OpUserApplyPlatformHelp     OperateType = "USER_APPLY_PLATFORM_HELP"                  // 用户申请平台协助
	OpMerchantApproveRefund     OperateType = "MERCHANT_APPROVE_REFUND"                   // 商户同意退款申请
	OpMerchantRefuseRefund      OperateType = "MERCHANT_REFUSE_RERUND"                    // 商户拒绝退款申请（微信原文如此拼写）
	OpUserSubmitSatisfaction    OperateType = "USER_SUBMIT_SATISFACTION"                  // 用户提交满意度调查结果
	OpServiceOrderCancel        OperateType = "SERVICE_ORDER_CANCEL"                      // 服务订单已取消
	OpServiceOrderComplete      OperateType = "SERVICE_ORDER_COMPLETE"                    // 服务订单已完成
)

// NegotiationEntry 是一条投诉协商记录。
type NegotiationEntry struct {
	// LogID 操作流水号。
	LogID string `json:"log_id"`
	// Operator 操作人。
	Operator string `json:"operator"`
	// OperateTime 操作时间，格式 yyyy-mm-dd'T'HH:MM:ssXXX。
	OperateTime string `json:"operate_time"`
	// OperateType 操作类型。
	OperateType OperateType `json:"operate_type"`
	// OperateDetails 该条记录的具体内容。
	OperateDetails string `json:"operate_details"`
	// ComplaintMediaList 执行操作时上传的资料凭证。
	ComplaintMediaList []ComplaintMedia `json:"complaint_media_list"`
}

// GetNegotiationHistoryRequest 是获取协商历史的请求。
type GetNegotiationHistoryRequest struct {
	// ComplaintID 投诉 ID，由 GetComplaintList 返回。
	ComplaintID string `json:"complaint_id"`
	// Offset 筛选偏移，从 0 开始。
	Offset int `json:"offset"`
	// Limit 最多返回条数。
	Limit int `json:"limit"`
	envField
}

// GetNegotiationHistoryResponse 是获取协商历史的响应。
type GetNegotiationHistoryResponse struct {
	// Total 总条数。
	Total int `json:"total"`
	// History 协商历史。
	History []NegotiationEntry `json:"history"`
}

// GetNegotiationHistory 获取投诉的协商历史。
//
// 官方文档：POST /xpay/get_negotiation_history
func (c *Client) GetNegotiationHistory(ctx context.Context, req GetNegotiationHistoryRequest) (*GetNegotiationHistoryResponse, error) {
	var resp GetNegotiationHistoryResponse
	if err := c.call(ctx, "/xpay/get_negotiation_history", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ResponseComplaintRequest 是回复用户的请求。
type ResponseComplaintRequest struct {
	// ComplaintID 投诉 ID。
	ComplaintID string `json:"complaint_id"`
	// ResponseContent 回复内容。
	ResponseContent string `json:"response_content"`
	// ResponseImages 回复的图片，每一项是 UploadVPFile 返回的 file_id。
	ResponseImages []string `json:"response_images"`
	envField
}

// ResponseComplaint 回复用户投诉。
//
// 回复中要附带的图片，需先用 UploadVPFile 上传拿到 file_id。
//
// 官方文档：POST /xpay/response_complaint
func (c *Client) ResponseComplaint(ctx context.Context, req ResponseComplaintRequest) error {
	return c.call(ctx, "/xpay/response_complaint", req, authPaySig, "", nil)
}

// CompleteComplaintRequest 是完成投诉处理的请求。
type CompleteComplaintRequest struct {
	// ComplaintID 投诉 ID。
	ComplaintID string `json:"complaint_id"`
	envField
}

// CompleteComplaint 完成投诉处理（即「申请结单」）。
//
// 官方文档：POST /xpay/complete_complaint
func (c *Client) CompleteComplaint(ctx context.Context, req CompleteComplaintRequest) error {
	return c.call(ctx, "/xpay/complete_complaint", req, authPaySig, "", nil)
}

// UploadVPFileRequest 是上传媒体文件的请求。
//
// Base64Img 与 ImgURL 二选一，且 ImgURL 优先。Base64Img 最大 1M，ImgURL 最大 2M。
type UploadVPFileRequest struct {
	// Base64Img 经 base64 编码后的图片内容，最多 1M。
	Base64Img string `json:"base64_img,omitempty"`
	// ImgURL 图片 URL，需能直接下载（不能返回 302 等），最高 2M。**优先使用本字段**。
	ImgURL string `json:"img_url,omitempty"`
	// FileName 图片名称。
	FileName string `json:"file_name"`
	envField
}

// UploadVPFileResponse 是上传媒体文件的响应。
type UploadVPFileResponse struct {
	// FileID 返回的文件 ID，用于 ResponseComplaint 的 ResponseImages。
	FileID string `json:"file_id"`
}

// UploadVPFile 上传媒体文件（图片、凭证等），用于回复投诉。
//
// 官方文档：POST /xpay/upload_vp_file
func (c *Client) UploadVPFile(ctx context.Context, req UploadVPFileRequest) (*UploadVPFileResponse, error) {
	var resp UploadVPFileResponse
	if err := c.call(ctx, "/xpay/upload_vp_file", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetUploadFileSignRequest 是获取微信支付投诉图片签名头部的请求。
type GetUploadFileSignRequest struct {
	// WxpayURL 微信支付的图片地址，格式为
	// https://api.mch.weixin.qq.com/v3/merchant-service/images/{xxxxxx}
	WxpayURL string `json:"wxpay_url"`
	// ConvertCOS 是否转存到 COS，转存后可获得 30 分钟有效的临时下载地址。
	ConvertCOS bool `json:"convert_cos"`
	// ComplaintID 对应的投诉 ID。
	ComplaintID string `json:"complaint_id"`
	envField
}

// GetUploadFileSignResponse 是获取签名头部的响应。
type GetUploadFileSignResponse struct {
	// Sign 微信支付图片请求的 Authorization 头部值。
	Sign string `json:"sign"`
	// CosURL 当 ConvertCOS 为 true 时才有意义，转存后的 URL，30 分钟有效。
	CosURL string `json:"cos_url"`
}

// GetUploadFileSign 获取微信支付投诉图片的签名头部。
//
// 投诉详情里的图片托管在微信支付侧，直接下载会被拒。需要先用本接口拿到
// Sign，再做 HTTP 请求并带上三个头部：
//
//	Authorization: <Sign>
//	Accept: application/json
//	User-Agent: <非空>
//
// 官方文档：POST /xpay/get_upload_file_sign
func (c *Client) GetUploadFileSign(ctx context.Context, req GetUploadFileSignRequest) (*GetUploadFileSignResponse, error) {
	var resp GetUploadFileSignResponse
	if err := c.call(ctx, "/xpay/get_upload_file_sign", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

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
