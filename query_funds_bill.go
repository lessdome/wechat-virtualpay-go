package wechat_virtualpay_go

import "context"

// FundsBillStatus 是广告金充值单状态。
type FundsBillStatus int

const (
	FundsBillProcessing FundsBillStatus = 0 // 充值中
	FundsBillSuccess    FundsBillStatus = 1 // 充值成功
	FundsBillFailed     FundsBillStatus = 2 // 充值失败
)

// FundsBillFilter 是查询广告金充值记录的过滤条件。
type FundsBillFilter struct {
	// OperTimeBegin 查询充值开始时间，unix 秒级时间戳。
	OperTimeBegin int64 `json:"oper_time_begin"`
	// OperTimeEnd 查询充值结束时间，unix 秒级时间戳。
	OperTimeEnd int64 `json:"oper_time_end"`
	// BillID 广告金充值单 ID，可选。
	BillID string `json:"bill_id,omitempty"`
	// RequestID 调用 CreateFundsBill 时传入的 request_id，可选。
	RequestID string `json:"request_id,omitempty"`
}

// QueryFundsBillRequest 是查询广告金充值记录的请求。
type QueryFundsBillRequest struct {
	// Page 查询页码，不小于 1。
	Page int `json:"page"`
	// PageSize 每页记录数量。
	PageSize int `json:"page_size"`
	// Filter 查询过滤条件。
	Filter FundsBillFilter `json:"filter"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// FundsBill 是一条广告金充值记录。
type FundsBill struct {
	BillID              string          `json:"bill_id"`
	OperTime            int64           `json:"oper_time"`             // 充值时间，unix 秒级时间戳
	SettleBegin         int64           `json:"settle_begin"`          // 结算周期开始时间
	SettleEnd           int64           `json:"settle_end"`            // 结算周期结束时间
	FundID              string          `json:"fund_id"`               // 对应广告金 ID
	TransferAccountName string          `json:"transfer_account_name"` // 充值账户
	TransferAccountUID  int64           `json:"transfer_account_uid"`  // 充值账户 UID
	TransferAmount      int64           `json:"transfer_amount"`       // 充值金额，单位分
	Status              FundsBillStatus `json:"status"`                // 充值状态
	RequestID           string          `json:"request_id"`            // 充值时的 request_id
}

// QueryFundsBillResponse 是查询广告金充值记录的响应。
type QueryFundsBillResponse struct {
	BillList  []FundsBill `json:"bill_list"`
	TotalPage int         `json:"total_page"`
}

// QueryFundsBill 查询广告金充值记录。
//
// 官方文档：POST /xpay/query_funds_bill
func (c *Client) QueryFundsBill(ctx context.Context, req QueryFundsBillRequest) (*QueryFundsBillResponse, error) {
	req.Env = c.envInt()
	var resp QueryFundsBillResponse
	if err := c.call(ctx, "/xpay/query_funds_bill", req, authTokenOnly, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
