package wechat_virtualpay_go

import (
	"context"
	"encoding/json"
	"fmt"
)

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

// PayItem 是 CurrencyPay 请求里 payitem 字段的一项，会被记入账户流水。
type PayItem struct {
	// ProductID 物品 ID。
	ProductID string `json:"productid"`
	// UnitPrice 单价。
	UnitPrice int64 `json:"unit_price"`
	// Quantity 数量。
	Quantity int64 `json:"quantity"`
}

// MarshalPayItems 把道具明细序列化为 CurrencyPayRequest.PayItem 所需的字符串。
//
// currency_pay 的 payitem 字段要求的是一个「JSON 数组的字符串」形式：
//
//	[{"productid":"物品id", "unit_price": 单价, "quantity": 数量}]
//
// 手写容易出错（漏引号、字段名写错），交给这个函数。
func MarshalPayItems(items ...PayItem) (string, error) {
	raw, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("wechat_virtualpay_go: 序列化 payitem 失败: %w", err)
	}
	return string(raw), nil
}

// CurrencyPayRequest 是扣减代币的请求。
//
// ⚠️ 官方文档的参数表把本接口所有请求体字段的「必填」列都标成了「否」，这显然是
// 文档生成的问题——OpenID / Amount / OrderID / UserIP 缺任何一个都不可能调用成功。
// 本包按实际语义处理：这四个字段不加 omitempty（始终发送），留空由微信报参数错误，
// 好过静默省略后拿到一个更费解的错误。PayItem 与 Remark 是真正可选的。
type CurrencyPayRequest struct {
	// OpenID 用户的 openid。
	OpenID string `json:"openid"`
	// UserIP 用户 IP，形如 1.1.1.1。
	UserIP string `json:"user_ip"`
	// Amount 支付的代币数量。
	Amount int64 `json:"amount"`
	// OrderID 订单号。
	OrderID string `json:"order_id"`
	// PayItem 物品信息（JSON 数组的字符串），会记录到账户流水中。
	// 可用 MarshalPayItems 生成。
	PayItem string `json:"payitem,omitempty"`
	// Remark 备注。
	Remark string `json:"remark,omitempty"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// CurrencyPayResponse 是扣减代币的响应。
type CurrencyPayResponse struct {
	// OrderID 订单号。
	OrderID string `json:"order_id"`
	// Balance 总余额，包括有价和赠送部分。
	Balance int64 `json:"balance"`
	// UsedPresentAmount 本次使用赠送部分的代币数量。
	UsedPresentAmount int64 `json:"used_present_amount"`
}

// CurrencyPay 扣减用户代币，一般用于代币支付。需要 SessionKey。
//
// 官方文档：POST /xpay/currency_pay
func (c *Client) CurrencyPay(ctx context.Context, sessionKey string, req CurrencyPayRequest) (*CurrencyPayResponse, error) {
	req.Env = c.envInt()
	var resp CurrencyPayResponse
	if err := c.call(ctx, "/xpay/currency_pay", req, authUserAndPaySig, sessionKey, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

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
// 返回 -15006，把下面的 authAccessTokenOnly 改为 authPaySig 即可。
//
// 官方文档：POST /xpay/present_currency
func (c *Client) PresentCurrency(ctx context.Context, req PresentCurrencyRequest) (*PresentCurrencyResponse, error) {
	req.Env = c.envInt()
	var resp PresentCurrencyResponse
	if err := c.call(ctx, "/xpay/present_currency", req, authAccessTokenOnly, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
