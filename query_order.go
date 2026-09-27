package wechat_virtualpay_go

import "context"

// OrderStatus 是订单状态（query_order 响应中的 order.status）。
type OrderStatus int

const (
	OrderStatusInit             OrderStatus = 0  // 订单初始化（未创建成功，不可用于支付）
	OrderStatusCreated          OrderStatus = 1  // 订单创建成功
	OrderStatusPaid             OrderStatus = 2  // 订单已支付，待发货
	OrderStatusProviding        OrderStatus = 3  // 订单发货中
	OrderStatusProvided         OrderStatus = 4  // 订单已发货
	OrderStatusRefunded         OrderStatus = 5  // 订单已退款
	OrderStatusClosed           OrderStatus = 6  // 订单已关闭（不可再使用）
	OrderStatusRefundFailed     OrderStatus = 7  // 订单退款失败
	OrderStatusRefundCompleted  OrderStatus = 8  // 用户退款完成
	OrderStatusAdFundsRecovered OrderStatus = 9  // 回收广告金完成
	OrderStatusSettleRollback   OrderStatus = 10 // 分账回退完成
)

// OrderType 是订单类型（query_order 响应中的 order.order_type）。
type OrderType int

const (
	OrderTypeNormal    OrderType = 0 // 普通虚拟支付
	OrderTypeRefund    OrderType = 1 // 普通退款
	OrderTypeIOS       OrderType = 7 // 苹果 iOS 支付
	OrderTypeIOSRefund OrderType = 8 // 苹果 iOS 退款
)

// SettleState 是结算状态。
type SettleState int

const (
	SettleStatePending SettleState = 0 // 未开始结算（与 3 相同）
	SettleStateRunning SettleState = 1 // 结算中
	SettleStateSuccess SettleState = 2 // 结算成功
	SettleStateWaiting SettleState = 3 // 待结算（与 0 相同）
)

// OrderEnvType 是订单响应里的环境类型（order.env_type）。
//
// ⚠️ 取值与请求体的 env **不同**：这里是 1=现网 / 2=沙箱，而请求体的 env 是
// 0=现网 / 1=沙箱。直接用 order.EnvType 与 Config.Env 比较会永远不相等。
// 之所以单独给一个具名类型，就是为了让这两套编码在类型层面就分得开。
type OrderEnvType int

const (
	OrderEnvTypeProduction OrderEnvType = 1 // 现网
	OrderEnvTypeSandbox    OrderEnvType = 2 // 沙箱
)

// Order 是订单信息。
type Order struct {
	OrderID        string       `json:"order_id"`         // 订单号
	CreateTime     int64        `json:"create_time"`      // 创建时间
	UpdateTime     int64        `json:"update_time"`      // 更新时间
	Status         OrderStatus  `json:"status"`           // 当前状态
	BizType        int          `json:"biz_type"`         // 业务类型，0-短剧
	OrderFee       int64        `json:"order_fee"`        // 订单金额，单位分
	CouponFee      int64        `json:"coupon_fee"`       // 订单优惠金额，单位分（暂无此字段）
	PaidFee        int64        `json:"paid_fee"`         // 用户支付金额，单位分
	OrderType      OrderType    `json:"order_type"`       // 订单类型
	RefundFee      int64        `json:"refund_fee"`       // 退款单时表示退款金额，单位分
	PaidTime       int64        `json:"paid_time"`        // 支付/退款时间，unix 秒级时间戳
	ProvideTime    int64        `json:"provide_time"`     // 发货时间
	BizMeta        string       `json:"biz_meta"`         // 订单创建时传的信息
	EnvType        OrderEnvType `json:"env_type"`         // 环境类型，注意取值是 1/2，见 OrderEnvType
	Token          string       `json:"token"`            // 下单时米大师返回的 token
	LeftFee        int64        `json:"left_fee"`         // 支付单经过退款后剩余的金额，单位分
	WxOrderID      string       `json:"wx_order_id"`      // 微信内部单号
	ChannelOrderID string       `json:"channel_order_id"` // 渠道单号（用户支付详情页的商户单号）
	WxpayOrderID   string       `json:"wxpay_order_id"`   // 微信支付交易单号
	SettTime       int64        `json:"sett_time"`        // 结算时间戳，大于 0 表示结算成功
	SettState      SettleState  `json:"sett_state"`       // 结算状态
	PlatformFeeFen int64        `json:"platform_fee_fen"` // 虚拟支付技术服务费，单位分；sett_state=2 时返回
	CpsFeeFen      int64        `json:"cps_fee_fen"`      // 公众号/视频号平台 cps 服务费，单位分；sett_state=2 时返回
}

// QueryOrderRequest 是查询订单的请求。
//
// OrderID 与 WxOrderID 二选一。
type QueryOrderRequest struct {
	// OpenID 用户的 openid。
	OpenID string `json:"openid"`
	// OrderID 创建的订单号（业务侧 outTradeNo）。
	OrderID string `json:"order_id,omitempty"`
	// WxOrderID 微信内部单号，与 OrderID 二选一。
	WxOrderID string `json:"wx_order_id,omitempty"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

type queryOrderResponse struct {
	responseHeader
	Order *Order `json:"order"`
}

// QueryOrder 查询创建的订单（现金单，非代币单）。
//
// 这是「查单补发」兜底方案的基础：发货推送可能丢失（用户异常退出等），
// 建议定时调用本接口核对已支付但未发货的订单。
//
// 官方文档：POST /xpay/query_order
func (c *Client) QueryOrder(ctx context.Context, req QueryOrderRequest) (*Order, error) {
	req.Env = c.envInt()
	var resp queryOrderResponse
	if err := c.call(ctx, "/xpay/query_order", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return resp.Order, nil
}
