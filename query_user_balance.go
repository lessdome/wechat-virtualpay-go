package virtualpay

import (
	"context"
	"fmt"
)

// BoolFlag 是兼容两种 JSON 表示的布尔值。
//
// 为什么需要它：query_user_balance 响应里的 first_save_flag，文档把**类型**写成
// boolean、却在**说明**里写「0:不满足。1:满足」。两种都出现过，直接声明成 bool
// 一旦微信回 0/1 就会整体解析失败——在一个查余额的接口上因为这个报错不值得。
type BoolFlag bool

// UnmarshalJSON 同时接受 true/false、0/1 与 null。
func (b *BoolFlag) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true", "1":
		*b = true
	case "false", "0", "null":
		*b = false
	default:
		return fmt.Errorf("virtualpay: 无法把 %s 解析为布尔值（期望 true/false 或 0/1）", data)
	}
	return nil
}

// QueryUserBalanceRequest 是查询代币余额的请求。
type QueryUserBalanceRequest struct {
	// OpenID 用户的 openid。
	OpenID string `json:"openid"`
	// UserIP 用户 IP，形如 1.1.1.1。
	UserIP string `json:"user_ip"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// QueryUserBalanceResponse 是查询代币余额的响应。
type QueryUserBalanceResponse struct {
	// Balance 代币总余额，包括有价和赠送部分。
	Balance int64 `json:"balance"`
	// PresentBalance 赠送账户的代币余额。
	PresentBalance int64 `json:"present_balance"`
	// SumSave 累计有价货币充值数量。
	SumSave int64 `json:"sum_save"`
	// SumPresent 累计赠送无价货币数量。
	SumPresent int64 `json:"sum_present"`
	// SumBalance 历史总增加的代币金额。
	SumBalance int64 `json:"sum_balance"`
	// SumCost 历史总消耗代币金额。
	SumCost int64 `json:"sum_cost"`
	// FirstSaveFlag 是否满足首充活动。
	FirstSaveFlag BoolFlag `json:"first_save_flag"`
}

// QueryUserBalance 查询用户的代币余额。
//
// 这是三个用户态接口之一，需要 SessionKey（由 wx.login 的 code 通过
// code2Session 换取），本包会据此计算用户签名 signature。
//
// 官方文档：POST /xpay/query_user_balance
func (c *Client) QueryUserBalance(ctx context.Context, sessionKey string, req QueryUserBalanceRequest) (*QueryUserBalanceResponse, error) {
	req.Env = c.envInt()
	var resp QueryUserBalanceResponse
	if err := c.call(ctx, "/xpay/query_user_balance", req, authUserAndPaySig, sessionKey, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
