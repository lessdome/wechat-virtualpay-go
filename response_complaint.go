package wechat_virtualpay_go

import "context"

// ResponseComplaintRequest 是回复用户的请求。
type ResponseComplaintRequest struct {
	// ComplaintID 投诉 ID。
	ComplaintID string `json:"complaint_id"`
	// ResponseContent 回复内容。
	ResponseContent string `json:"response_content"`
	// ResponseImages 回复的图片，每一项是 UploadVPFile 返回的 file_id。
	ResponseImages []string `json:"response_images"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// ResponseComplaint 回复用户投诉。
//
// 回复中要附带的图片，需先用 UploadVPFile 上传拿到 file_id。
//
// 官方文档：POST /xpay/response_complaint
func (c *Client) ResponseComplaint(ctx context.Context, req ResponseComplaintRequest) error {
	req.Env = c.envInt()
	return c.call(ctx, "/xpay/response_complaint", req, authPaySig, "", nil)
}
