package virtualpay

import (
	"context"
	"encoding/json"
	"fmt"
)

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
		return "", fmt.Errorf("virtualpay: 序列化 payitem 失败: %w", err)
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
