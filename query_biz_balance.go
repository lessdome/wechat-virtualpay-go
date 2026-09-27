package wechat_virtualpay_go

import "context"

// BizBalance 是商家账户的可提现余额。
type BizBalance struct {
	// Amount 可提现余额，**单位是元**（字符串形式）。见 Yuan。
	Amount Yuan `json:"amount"`
	// CurrencyCode 币种，一般为 CNY。
	CurrencyCode string `json:"currency_code"`
}

// QueryBizBalanceRequest 是查询商家账户可提现余额的请求。
type QueryBizBalanceRequest struct {
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	//
	// 注意：本接口的 env 只用于签名校验，查询结果始终是**正式环境**的数据，
	// 沙箱下查不到沙箱余额。
	Env int `json:"env"`
}

// QueryBizBalanceResponse 是查询商家账户可提现余额的响应。
type QueryBizBalanceResponse struct {
	// BalanceAvailable 可提现余额。
	BalanceAvailable BizBalance `json:"balance_available"`
}

// QueryBizBalance 查询商家账户里的可提现余额。
//
// 官方文档：POST /xpay/query_biz_balance
func (c *Client) QueryBizBalance(ctx context.Context, req QueryBizBalanceRequest) (*QueryBizBalanceResponse, error) {
	req.Env = c.envInt()
	var resp QueryBizBalanceResponse
	if err := c.call(ctx, "/xpay/query_biz_balance", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
