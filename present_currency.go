package wechat_virtualpay_go

import "context"

// PresentCurrencyRequest 是代币赠送的请求。
type PresentCurrencyRequest struct {
	// OpenID 用户的 openid。
	OpenID string `json:"openid"`
	// OrderID 赠送单号。
	OrderID string `json:"order_id"`
	// Amount 赠送金额（代币数量）。
	Amount int64 `json:"amount"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// PresentCurrencyResponse 是代币赠送的响应。
type PresentCurrencyResponse struct {
	// Balance 赠送后用户的代币余额。
	Balance int64 `json:"balance"`
	// OrderID 赠送单号。
	OrderID string `json:"order_id"`
	// PresentBalance 用户收到的总赠送金额。
	PresentBalance int64 `json:"present_balance"`
}

// PresentCurrency 向用户赠送代币。
//
// 注意：本接口不支持按单号查询赠送单，赠送时可重试至返回成功（errcode=0）或
// 重复操作（268490004）为止——也就是说，重试是安全的，但也拿不到「查一下到底
// 赠出去没有」的能力。
//
// ⚠️ 文档存疑：本接口的 query 参数表只有 access_token，没有 pay_sig，但其请求体
// 里的 env 注释写着「仅作为签名校验」。本包暂按参数表实现（不加签名）。若实测
// 返回 -15006，把下面的 authTokenOnly 改为 authPaySig 即可。
//
// 官方文档：POST /xpay/present_currency
func (c *Client) PresentCurrency(ctx context.Context, req PresentCurrencyRequest) (*PresentCurrencyResponse, error) {
	req.Env = c.envInt()
	var resp PresentCurrencyResponse
	if err := c.call(ctx, "/xpay/present_currency", req, authTokenOnly, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
