package wechat_virtualpay_go

import "context"

// CancelCurrencyPayRequest 是代币支付退款的请求。
type CancelCurrencyPayRequest struct {
	// OpenID 用户的 openid。
	OpenID string `json:"openid"`
	// UserIP 用户 IP，形如 1.1.1.1。
	UserIP string `json:"user_ip"`
	// PayOrderID 原代币支付单号，即调用 CurrencyPay 时传的 OrderID。
	PayOrderID string `json:"pay_order_id"`
	// OrderID 本次退款单的单号。
	//
	// ⚠️ 注意与 RefundOrderRequest 的区别：那边的 OrderID 指**原支付单**，本接口的
	// order_id 指**本次退款单**——同名反义。两个接口功能相近，勿直接复制粘贴。
	OrderID string `json:"order_id"`
	// Amount 退款金额（代币数量）。
	Amount int64 `json:"amount"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// CancelCurrencyPayResponse 是代币支付退款的响应。
type CancelCurrencyPayResponse struct {
	// OrderID 退款订单号。
	OrderID string `json:"order_id"`
}

// CancelCurrencyPay 代币支付退款，是 CurrencyPay 的逆操作。需要 SessionKey。
//
// 官方文档：POST /xpay/cancel_currency_pay
func (c *Client) CancelCurrencyPay(ctx context.Context, sessionKey string, req CancelCurrencyPayRequest) (*CancelCurrencyPayResponse, error) {
	req.Env = c.envInt()
	var resp CancelCurrencyPayResponse
	if err := c.call(ctx, "/xpay/cancel_currency_pay", req, authUserAndPaySig, sessionKey, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
