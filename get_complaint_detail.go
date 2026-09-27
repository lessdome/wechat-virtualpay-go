package wechat_virtualpay_go

import "context"

// GetComplaintDetailRequest 是获取投诉详情的请求。
type GetComplaintDetailRequest struct {
	// ComplaintID 投诉 ID，由 GetComplaintList 返回。
	ComplaintID string `json:"complaint_id"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
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
	req.Env = c.envInt()
	var resp GetComplaintDetailResponse
	if err := c.call(ctx, "/xpay/get_complaint_detail", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
