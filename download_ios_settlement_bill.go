package wechat_virtualpay_go

import "context"

// DownloadIOSBillRequest 是下载苹果 IAP 支付月账单的请求。
type DownloadIOSBillRequest struct {
	// StartMonth 开始月份，格式 YYYYMM，如 202601。
	StartMonth string `json:"start_month"`
	// EndMonth 结束月份，格式 YYYYMM。
	EndMonth string `json:"end_month"`
}

// IOSBill 是一张苹果 IAP 结算单。
type IOSBill struct {
	// Month 月份，格式 YYYYMM。
	Month string `json:"month"`
	// BillURL 账单下载链接，请及时使用，一定时间后失效。
	BillURL string `json:"bill_url"`
}

// DownloadIOSBillResponse 是下载苹果 IAP 月账单的响应。
type DownloadIOSBillResponse struct {
	// BillList 结算单列表。
	BillList []IOSBill `json:"bill_list"`
}

// DownloadIOSBill 下载指定月份的苹果 IAP 支付月账单及其下载链接。
//
// 本接口的请求体同样不需要 env。
//
// 官方文档：POST /xpay/download_ios_settlement_bill
func (c *Client) DownloadIOSBill(ctx context.Context, req DownloadIOSBillRequest) (*DownloadIOSBillResponse, error) {
	var resp DownloadIOSBillResponse
	if err := c.call(ctx, "/xpay/download_ios_settlement_bill", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
