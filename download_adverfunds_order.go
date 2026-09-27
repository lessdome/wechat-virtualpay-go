package wechat_virtualpay_go

import "context"

// DownloadAdverFundsOrderRequest 是下载广告金对应商户订单信息的请求。
type DownloadAdverFundsOrderRequest struct {
	// FundID 广告金发放 ID。
	FundID string `json:"fund_id"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// DownloadAdverFundsOrderResponse 是下载广告金对应商户订单信息的响应。
type DownloadAdverFundsOrderResponse struct {
	// URL 订单下载链接。
	URL string `json:"url"`
}

// DownloadAdverFundsOrder 下载广告金对应的商户订单信息。
//
// 官方文档：POST /xpay/download_adverfunds_order
func (c *Client) DownloadAdverFundsOrder(ctx context.Context, req DownloadAdverFundsOrderRequest) (*DownloadAdverFundsOrderResponse, error) {
	req.Env = c.envInt()
	var resp DownloadAdverFundsOrderResponse
	if err := c.call(ctx, "/xpay/download_adverfunds_order", req, authTokenOnly, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
