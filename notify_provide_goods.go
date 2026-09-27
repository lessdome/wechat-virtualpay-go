package wechat_virtualpay_go

import "context"

// NotifyProvideGoodsRequest 是通知已发货完成的请求。
//
// OrderID 与 WxOrderID 二选一。
type NotifyProvideGoodsRequest struct {
	// OrderID 下单时传的单号。
	OrderID string `json:"order_id,omitempty"`
	// WxOrderID 微信内部单号，与 OrderID 二选一。
	WxOrderID string `json:"wx_order_id,omitempty"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// NotifyProvideGoods 通知微信「已发货完成」，仅用于现金单。
//
// 正常情况下，正确应答 xpay_goods_deliver_notify 推送即可，无需调用本接口。
// 本接口用于推送异常、需要手动把订单改成已发货状态的场景。
//
// ⚠️ 文档存疑：本接口的 query 参数表只有 access_token，没有 pay_sig。但其请求体
// 里的 env 字段注释写着「仅作为签名校验」，两处矛盾。本包暂按参数表实现（不加
// 签名）。若实测返回 -15006，把 authTokenOnly 改为 authPaySig 即可。
//
// 官方文档：POST /xpay/notify_provide_goods
func (c *Client) NotifyProvideGoods(ctx context.Context, req NotifyProvideGoodsRequest) error {
	req.Env = c.envInt()
	return c.call(ctx, "/xpay/notify_provide_goods", req, authTokenOnly, "", nil)
}
