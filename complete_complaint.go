package virtualpay

import "context"

// CompleteComplaintRequest 是完成投诉处理的请求。
type CompleteComplaintRequest struct {
	// ComplaintID 投诉 ID。
	ComplaintID string `json:"complaint_id"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// CompleteComplaint 完成投诉处理（即「申请结单」）。
//
// 官方文档：POST /xpay/complete_complaint
func (c *Client) CompleteComplaint(ctx context.Context, req CompleteComplaintRequest) error {
	req.Env = c.envInt()
	return c.call(ctx, "/xpay/complete_complaint", req, authPaySig, "", nil)
}
