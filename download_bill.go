package virtualpay

import "context"

// DownloadBillRequest 是下载普通虚拟支付日账单的请求。
type DownloadBillRequest struct {
	// BeginDs 起始时间，格式 YYYYMMDD，如 20230801。
	BeginDs int64 `json:"begin_ds"`
	// EndDs 截止时间，格式 YYYYMMDD，如 20230810。
	EndDs int64 `json:"end_ds"`
}

// DownloadBillResponse 是下载日账单的响应。
type DownloadBillResponse struct {
	// URL 账单下载地址，有效期为半小时，失效后需重新获取。
	URL string `json:"url"`
}

// DownloadBill 下载普通虚拟支付按日汇总的结算账单。
//
// 本接口是**轮询式**的：首次调用触发生成下载 URL，若返回的 URL 为空说明还在生成中，
// 间隔一段时间重试即可。拿到 URL 后请尽快下载——有效期只有半小时。
//
// 与 DownloadIOSBill 不同，本接口的请求体不需要 env。
//
// 官方文档：POST /xpay/download_bill
func (c *Client) DownloadBill(ctx context.Context, req DownloadBillRequest) (*DownloadBillResponse, error) {
	var resp DownloadBillResponse
	if err := c.call(ctx, "/xpay/download_bill", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
