package virtualpay

import "context"

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
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
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
	req.Env = c.envInt()
	var resp GetNegotiationHistoryResponse
	if err := c.call(ctx, "/xpay/get_negotiation_history", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
