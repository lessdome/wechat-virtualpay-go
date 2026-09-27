package virtualpay

import "context"

// CreateWithdrawOrderRequest 是创建提现单的请求。
type CreateWithdrawOrderRequest struct {
	// WithdrawNo 提现单单号，长度 [8,32]，只允许字母、数字、'_'、'-'。
	WithdrawNo string `json:"withdraw_no"`
	// WithdrawAmount 提现金额，**单位是元**，字符串形式——例如提现 1 分钱传 "0.01"。
	//
	// ⚠️ 这是本 SDK 里唯一以「元」为单位的金额字段。其余所有金额（GoodsPrice、
	// OrderFee、PaidFee、RefundFee、代币 amount 等）全部是**分**。写代码时务必
	// 分清，这里按分填会提现出 100 倍金额。
	//
	// 留空表示全额提现。
	WithdrawAmount string `json:"withdraw_amount,omitempty"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
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
	req.Env = c.envInt()
	var resp CreateWithdrawOrderResponse
	if err := c.call(ctx, "/xpay/create_withdraw_order", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
