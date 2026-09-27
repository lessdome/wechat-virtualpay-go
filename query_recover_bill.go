package wechat_virtualpay_go

import "context"

// RecoverBillFilter 是查询广告金回收记录的过滤条件。
type RecoverBillFilter struct {
	// RecoverTimeBegin 查询回收开始时间，unix 秒级时间戳。
	RecoverTimeBegin int64 `json:"recover_time_begin"`
	// RecoverTimeEnd 查询回收结束时间，unix 秒级时间戳。
	RecoverTimeEnd int64 `json:"recover_time_end"`
	// BillID 广告金回收单 ID。文档标为必填，但说明里又写"(可选)"，此处按必填处理。
	BillID string `json:"bill_id"`
}

// QueryRecoverBillRequest 是查询广告金回收记录的请求。
type QueryRecoverBillRequest struct {
	// Page 查询页码，不小于 1。
	Page int `json:"page"`
	// PageSize 每页记录数量。
	PageSize int `json:"page_size"`
	// Filter 查询过滤条件。
	Filter RecoverBillFilter `json:"filter"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// RecoverBill 是一条广告金回收记录。
type RecoverBill struct {
	BillID             string   `json:"bill_id"`
	RecoverTime        int64    `json:"recover_time"`         // 回收时间，unix 秒级时间戳
	SettleBegin        int64    `json:"settle_begin"`         // 结算周期开始时间
	SettleEnd          int64    `json:"settle_end"`           // 结算周期结束时间
	FundID             string   `json:"fund_id"`              // 对应的发放广告金 ID
	RecoverAccountName string   `json:"recover_account_name"` // 回收广告金账户
	RecoverAmount      int64    `json:"recover_amount"`       // 回收金额，单位分
	RefundOrderList    []string `json:"refund_order_list"`    // 对应的退款订单 id
}

// QueryRecoverBillResponse 是查询广告金回收记录的响应。
type QueryRecoverBillResponse struct {
	BillList  []RecoverBill `json:"bill_list"`
	TotalPage int           `json:"total_page"`
}

// QueryRecoverBill 查询广告金回收记录。
//
// 官方文档：POST /xpay/query_recover_bill
func (c *Client) QueryRecoverBill(ctx context.Context, req QueryRecoverBillRequest) (*QueryRecoverBillResponse, error) {
	req.Env = c.envInt()
	var resp QueryRecoverBillResponse
	if err := c.call(ctx, "/xpay/query_recover_bill", req, authAccessTokenOnly, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
