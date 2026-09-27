package wechat_virtualpay_go

import "context"

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
	//
	// ⚠️ 文档把本字段的**类型**写成 boolean，**说明**里却写「0:不满足。1:满足」。
	// 此处按文档的类型列用 bool；若实测微信返回 0/1，整个响应会解析失败，届时
	// 需要改成接受两种表示的自定义反序列化。
	FirstSaveFlag bool `json:"first_save_flag"`
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
