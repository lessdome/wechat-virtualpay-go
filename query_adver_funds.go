package wechat_virtualpay_go

import "context"

// AdFundType 是广告金发放原因。
type AdFundType int

const (
	AdFundTypeGeneral AdFundType = 0 // 通用赠送
	AdFundTypeAd      AdFundType = 1 // 广告激励
	AdFundTypeTarget  AdFundType = 2 // 定向激励
)

// AdFundFilter 是查询广告金发放记录的过滤条件。
type AdFundFilter struct {
	// SettleBegin 结算周期开始时间，unix 秒级时间戳。
	SettleBegin int64 `json:"settle_begin,omitempty"`
	// SettleEnd 结算周期结束时间，unix 秒级时间戳。
	SettleEnd int64 `json:"settle_end,omitempty"`
	// FundType 广告金发放原因。使用指针以区分「不筛选」与「筛选 0（通用赠送）」。
	FundType *AdFundType `json:"fund_type,omitempty"`
}

// QueryAdverFundsRequest 是查询广告金发放记录的请求。
type QueryAdverFundsRequest struct {
	// Page 查询页码，不小于 1。
	Page int `json:"page,omitempty"`
	// PageSize 每页记录数量。
	PageSize int `json:"page_size,omitempty"`
	// Filter 查询过滤条件。
	Filter *AdFundFilter `json:"filter,omitempty"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// AdverFund 是一条广告金发放记录。
type AdverFund struct {
	SettleBegin  int64      `json:"settle_begin"`  // 结算周期开始时间，unix 秒级时间戳
	SettleEnd    int64      `json:"settle_end"`    // 结算周期结束时间，unix 秒级时间戳
	TotalAmount  int64      `json:"total_amount"`  // 发放广告金金额，单位分
	RemainAmount int64      `json:"remain_amount"` // 剩余可用广告金金额，单位分
	ExpireTime   int64      `json:"expire_time"`   // 广告金过期时间，unix 秒级时间戳
	FundType     AdFundType `json:"fund_type"`     // 广告金发放原因
	FundID       string     `json:"fund_id"`       // 广告金发放 ID
}

// QueryAdverFundsResponse 是查询广告金发放记录的响应。
type QueryAdverFundsResponse struct {
	AdverFundsList []AdverFund `json:"adver_funds_list"`
	TotalPage      int         `json:"total_page"`
}

// QueryAdverFunds 查询广告金发放记录。
//
// 官方文档：POST /xpay/query_adver_funds
func (c *Client) QueryAdverFunds(ctx context.Context, req QueryAdverFundsRequest) (*QueryAdverFundsResponse, error) {
	req.Env = c.envInt()
	var resp QueryAdverFundsResponse
	if err := c.call(ctx, "/xpay/query_adver_funds", req, authAccessTokenOnly, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
