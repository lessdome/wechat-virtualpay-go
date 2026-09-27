package wechat_virtualpay_go

import "context"

// RefundReason 是退款原因。
type RefundReason string

const (
	RefundReasonNone      RefundReason = "0" // 暂无描述
	RefundReasonProduct   RefundReason = "1" // 产品问题，影响使用或效果不佳
	RefundReasonAfterSale RefundReason = "2" // 售后问题，无法满足需求
	RefundReasonUserWill  RefundReason = "3" // 意愿问题，用户主动退款
	RefundReasonPrice     RefundReason = "4" // 价格问题
	RefundReasonOther     RefundReason = "5" // 其他原因
)

// RefundFrom 是退款来源。
type RefundFrom string

const (
	RefundFromCustomerService RefundFrom = "1" // 人工客服退款
	RefundFromUser            RefundFrom = "2" // 用户自己发起退款流程
	RefundFromOther           RefundFrom = "3" // 其它
)

// RefundOrderRequest 是发起退款的请求。
//
// OrderID 与 WxOrderID 二选一。
type RefundOrderRequest struct {
	// OpenID 下单时的用户 openid。
	OpenID string `json:"openid"`
	// OrderID 下单时的单号（业务侧 outTradeNo），与 WxOrderID 二选一。
	OrderID string `json:"order_id,omitempty"`
	// WxOrderID 支付单对应的微信侧单号，与 OrderID 二选一。
	WxOrderID string `json:"wx_order_id,omitempty"`
	// RefundOrderID 本次退款单号，长度 8–32，仅允许字母、数字、'_'、'-'。
	RefundOrderID string `json:"refund_order_id"`
	// LeftFee 当前单剩余可退金额，单位分。可先调 QueryOrder 获取 order.left_fee。
	LeftFee int64 `json:"left_fee"`
	// RefundFee 本次退款金额，单位分，需满足 (0, LeftFee]。
	RefundFee int64 `json:"refund_fee"`
	// BizMeta 商家自定义数据，长度 [0,1024]，QueryOrder 时原样返回。
	BizMeta string `json:"biz_meta"`
	// RefundReason 退款原因。
	RefundReason RefundReason `json:"refund_reason"`
	// RefundFrom 退款来源。
	RefundFrom RefundFrom `json:"req_from"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// RefundOrderResponse 是发起退款的响应。
type RefundOrderResponse struct {
	RefundOrderID   string `json:"refund_order_id"`    // 退款单号
	RefundWxOrderID string `json:"refund_wx_order_id"` // 退款单的微信侧单号
	PayOrderID      string `json:"pay_order_id"`       // 该退款单对应的支付单单号
	PayWxOrderID    string `json:"pay_wx_order_id"`    // 该退款单对应的支付单微信侧单号
}

// RefundOrder 发起订单退款。
//
// 注意：本接口只是**启动退款任务**，返回成功不代表退款已完成。启动后需调用
// QueryOrder 查询退款单状态，等状态变为 OrderStatusRefundCompleted 才是最终成功。
//
// iOS 订单无法通过本接口退款——Apple IAP 由用户向 App Store 申请，开发者只能
// 被动接收 xpay_subscribe_ios_refund_query_notify 问询。
//
// ⚠️ 文档存疑：本接口的「注意事项」写着「使用用户态签名与支付签名」，但其 query
// 参数表只列了 access_token 与 pay_sig（没有 signature），且该段落与其它接口的
// 模板文字雷同。本包暂按参数表实现（只加 pay_sig）。若实测返回 -15005
// （用户签名错误），把本方法改为 authUserAndPaySig 并传入 SessionKey 即可。
//
// 官方文档：POST /xpay/refund_order
func (c *Client) RefundOrder(ctx context.Context, req RefundOrderRequest) (*RefundOrderResponse, error) {
	req.Env = c.envInt()
	var resp RefundOrderResponse
	if err := c.call(ctx, "/xpay/refund_order", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
