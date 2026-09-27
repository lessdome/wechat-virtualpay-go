package wechat_virtualpay_go

import "context"

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
	ComplaintID     string         `json:"complaint_id"`
	ComplaintTime   string         `json:"complaint_time"` // 格式 yyyy-mm-dd'T'HH:MM:ssXXX
	ComplaintDetail string         `json:"complaint_detail"`
	ComplaintState  ComplaintState `json:"complaint_state"`
	PayerPhone      string         `json:"payer_phone"`
	PayerOpenID     string         `json:"payer_openid"`
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
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
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
	req.Env = c.envInt()
	var resp GetComplaintListResponse
	if err := c.call(ctx, "/xpay/get_complaint_list", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
