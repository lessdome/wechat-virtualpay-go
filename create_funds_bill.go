package wechat_virtualpay_go

import "context"

// CreateFundsBillRequest 是充值广告金的请求。
//
// 充值金额单位是**分**（与 CreateWithdrawOrder 的元不同）。
type CreateFundsBillRequest struct {
	// TransferAmount 充值金额，单位分。
	TransferAmount int64 `json:"transfer_amount"`
	// TransferAccountUID 充值账户 uid。
	TransferAccountUID int64 `json:"transfer_account_uid"`
	// TransferAccountName 充值账户名称。
	TransferAccountName string `json:"transfer_account_name"`
	// TransferAccountAgencyID 充值账户服务商账号 id。
	TransferAccountAgencyID int64 `json:"transfer_account_agency_id"`
	// RequestID 每一次请求的唯一 id（不超过 1024 字符）。
	// **相同 id 的不同请求会被视为重复请求**——这是本接口的幂等键。
	RequestID string `json:"request_id"`
	// SettleBegin 充值所使用的广告金对应的结算周期开始时间，unix 秒级时间戳。
	SettleBegin int64 `json:"settle_begin"`
	// SettleEnd 充值所使用的广告金对应的结算周期结束时间，unix 秒级时间戳。
	SettleEnd int64 `json:"settle_end"`
	// AuthorizeAdvertise 是否授权广告数据：0 否，1 是。
	AuthorizeAdvertise int `json:"authorize_advertise"`
	// FundType 广告金发放原因。
	FundType AdFundType `json:"fund_type"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// CreateFundsBillResponse 是充值广告金的响应。
type CreateFundsBillResponse struct {
	// BillID 充值单 id。
	BillID string `json:"bill_id"`
}

// CreateFundsBill 充值广告金。
//
// 幂等靠 RequestID：相同 RequestID 的重复请求会被微信识别为重复。
//
// 官方文档：POST /xpay/create_funds_bill
func (c *Client) CreateFundsBill(ctx context.Context, req CreateFundsBillRequest) (*CreateFundsBillResponse, error) {
	req.Env = c.envInt()
	var resp CreateFundsBillResponse
	if err := c.call(ctx, "/xpay/create_funds_bill", req, authTokenOnly, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
