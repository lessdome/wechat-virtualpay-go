package wechat_virtualpay_go

import (
	"context"
)

// CreateWithdrawOrderRequest 是创建提现单的请求。
type CreateWithdrawOrderRequest struct {
	// WithdrawNo 提现单单号，长度 [8,32]，只允许字母、数字、'_'、'-'。
	WithdrawNo string `json:"withdraw_no"`
	// WithdrawAmount 提现金额，**单位是元**，字符串形式——例如提现 1 分钱传 "0.01"。
	//
	// ⚠️ 提现金额与可提现余额（BizBalance.Amount）都是**元**，其余所有金额
	// （GoodsPrice、OrderFee、PaidFee、RefundFee、代币 Amount 等）全部是**分**。
	// 写代码时务必分清，这里按分填会提现出 100 倍金额。
	//
	// 留空表示全额提现。
	WithdrawAmount string `json:"withdraw_amount,omitempty"`
	// Env 环境标识。本包只支持现网，固定为 0。
	Env int `json:"env"`
}

// CreateWithdrawOrderResponse 是创建提现单的响应。
type CreateWithdrawOrderResponse struct {
	// WithdrawNo 提现单号。
	WithdrawNo string `json:"withdraw_no"`
	// WxWithdrawNo 提现单的微信侧单号。
	WxWithdrawNo string `json:"wx_withdraw_no"`
}

// CreateWithdrawOrder 创建提现单。
//
// 创建成功不等于提现到账，需用 QueryWithdrawOrder 查询状态。
//
// 官方文档：POST /xpay/create_withdraw_order
func (c *Client) CreateWithdrawOrder(ctx context.Context, req CreateWithdrawOrderRequest) (*CreateWithdrawOrderResponse, error) {
	var resp CreateWithdrawOrderResponse
	if err := c.call(ctx, "/xpay/create_withdraw_order", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// WithdrawStatus 是提现单状态。
type WithdrawStatus int

const (
	WithdrawStatusCreated WithdrawStatus = 1 // 创建成功，提现中
	WithdrawStatusSuccess WithdrawStatus = 2 // 提现成功
	WithdrawStatusFailed  WithdrawStatus = 3 // 提现失败
)

// QueryWithdrawOrderRequest 是查询提现单的请求。
type QueryWithdrawOrderRequest struct {
	// WithdrawNo 提现单单号。
	WithdrawNo string `json:"withdraw_no"`
	// Env 环境标识。本包只支持现网，固定为 0。
	Env int `json:"env"`
}

// QueryWithdrawOrderResponse 是查询提现单的响应。
//
// 注意：本接口的金额与时间字段都是**字符串**（微信如此返回），不是数字。
type QueryWithdrawOrderResponse struct {
	// WithdrawNo 提现单号。
	WithdrawNo string `json:"withdraw_no"`
	// Status 提现状态。
	Status WithdrawStatus `json:"status"`
	// WithdrawAmount 提现金额，**单位是元**。
	WithdrawAmount string `json:"withdraw_amount"`
	// WxWithdrawNo 提现单的微信侧单号。
	WxWithdrawNo string `json:"wx_withdraw_no"`
	// WithdrawSuccessTimestamp 提现成功的秒级时间戳。
	WithdrawSuccessTimestamp string `json:"withdraw_success_timestamp"`
	// CreateTime 提现单创建时间。
	CreateTime string `json:"create_time"`
	// FailReason 提现失败的原因。
	FailReason string `json:"fail_reason"`
}

// QueryWithdrawOrder 查询提现单。
//
// 官方文档：POST /xpay/query_withdraw_order
func (c *Client) QueryWithdrawOrder(ctx context.Context, req QueryWithdrawOrderRequest) (*QueryWithdrawOrderResponse, error) {
	var resp QueryWithdrawOrderResponse
	if err := c.call(ctx, "/xpay/query_withdraw_order", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BizBalance 是商家账户的可提现余额。
type BizBalance struct {
	// Amount 可提现余额，**单位是元**（字符串形式）。
	Amount string `json:"amount"`
	// CurrencyCode 币种，一般为 CNY。
	CurrencyCode string `json:"currency_code"`
}

// QueryBizBalanceRequest 是查询商家账户可提现余额的请求。
type QueryBizBalanceRequest struct {
	// Env 环境标识。本包只支持现网，固定为 0。
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
	var resp QueryBizBalanceResponse
	if err := c.call(ctx, "/xpay/query_biz_balance", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
