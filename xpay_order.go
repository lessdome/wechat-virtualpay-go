package wechat_virtualpay_go

import (
	"context"
)

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
// 签名）。若实测返回 -15006，把 authAccessTokenOnly 改为 authPaySig 即可。
//
// 官方文档：POST /xpay/notify_provide_goods
func (c *Client) NotifyProvideGoods(ctx context.Context, req NotifyProvideGoodsRequest) error {
	req.Env = c.envInt()
	return c.call(ctx, "/xpay/notify_provide_goods", req, authAccessTokenOnly, "", nil)
}

// DownloadOrderType 是下载订单的类型。
type DownloadOrderType int

const (
	DownloadOrderCoin         DownloadOrderType = 1 // 代币交易订单
	DownloadOrderGoods        DownloadOrderType = 2 // 道具直购交易订单
	DownloadOrderSubscription DownloadOrderType = 3 // 会员订阅订单
	DownloadOrderRefund       DownloadOrderType = 4 // 退款订单
)

// PayChannel 是支付渠道。
type PayChannel int

const (
	PayChannelNormal PayChannel = 1 // 普通虚拟支付
	PayChannelIAP    PayChannel = 2 // 苹果 IAP
)

// RefundStatusFilter 是下载订单时的退款状态筛选（仅 order_type=4 有效）。
type RefundStatusFilter int

const (
	RefundStatusFilterAll      RefundStatusFilter = 0 // 全部
	RefundStatusFilterRefunded RefundStatusFilter = 2 // 已退款
	RefundStatusFilterOngoing  RefundStatusFilter = 4 // 退款中
	RefundStatusFilterFailed   RefundStatusFilter = 5 // 退款失败
)

// StartDownloadOrderRequest 是发起下载订单明细任务的请求。
type StartDownloadOrderRequest struct {
	// BeginDs 开始日期，格式 YYYYMMDD，如 20260420。
	BeginDs int64 `json:"begin_ds"`
	// EndDs 结束日期，格式 YYYYMMDD，与 BeginDs 间隔不超过 31 天。
	EndDs int64 `json:"end_ds"`
	// OrderType 要下载哪一类订单。
	//
	// ⚠️ 与 Order.OrderType 同名不同义：那个是**这一单自身的类型**（0/1/7/8），
	// 这个是**筛选条件**（1/2/3/4）。
	OrderType DownloadOrderType `json:"order_type"`
	// OrderInfo 搜索关键字，支持按交易单号/商户单号/用户 ID 模糊匹配，可选。
	OrderInfo string `json:"order_info,omitempty"`
	// IsProvided 发货状态筛选。OrderType 为道具(2)或会员订阅(3)时**必须**传入。
	// 使用指针以区分「未传」与「传 false」。
	IsProvided *bool `json:"is_provided,omitempty"`
	// RefundStatus 退款状态筛选，仅 OrderType=退款订单(4) 时有效。
	RefundStatus RefundStatusFilter `json:"refund_status,omitempty"`
	// PayChannel 支付渠道。
	PayChannel PayChannel `json:"pay_channel"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// StartDownloadOrderResponse 是发起下载任务的响应。
type StartDownloadOrderResponse struct {
	// TaskID 下载任务 ID，用于 QueryDownloadOrder 查询结果。
	TaskID string `json:"task_id"`
}

// StartDownloadOrder 发起下载小程序订单明细的任务。
//
// 任务为异步：本方法返回后需轮询 QueryDownloadOrder 直到拿到 download_url。
//
// 官方文档：POST /xpay/start_download_order
func (c *Client) StartDownloadOrder(ctx context.Context, req StartDownloadOrderRequest) (*StartDownloadOrderResponse, error) {
	req.Env = c.envInt()
	var resp StartDownloadOrderResponse
	if err := c.call(ctx, "/xpay/start_download_order", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DownloadTaskStatus 是下载任务状态。
type DownloadTaskStatus int

const (
	DownloadTaskInit    DownloadTaskStatus = 0 // 初始化
	DownloadTaskRunning DownloadTaskStatus = 1 // 运行中
	DownloadTaskSuccess DownloadTaskStatus = 2 // 成功
	DownloadTaskFailed  DownloadTaskStatus = 3 // 失败
)

// QueryDownloadOrderRequest 是查询下载任务的请求。
type QueryDownloadOrderRequest struct {
	// TaskID 由 StartDownloadOrder 返回的下载任务 ID。
	TaskID string `json:"task_id"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// QueryDownloadOrderResponse 是查询下载任务的响应。
type QueryDownloadOrderResponse struct {
	// TaskID 下载任务 ID，与请求参数对应。
	TaskID string `json:"task_id"`
	// Status 任务状态。
	Status DownloadTaskStatus `json:"status"`
	// DownloadURL 下载文件 URL，仅 Status=DownloadTaskSuccess 时有值。
	DownloadURL string `json:"download_url"`
	// ExpireAt URL 过期时间（Unix 秒级时间戳）。
	ExpireAt int64 `json:"expire_at"`
}

// QueryDownloadOrder 查询下载任务的结果。
//
// 官方文档：POST /xpay/query_download_order
func (c *Client) QueryDownloadOrder(ctx context.Context, req QueryDownloadOrderRequest) (*QueryDownloadOrderResponse, error) {
	req.Env = c.envInt()
	var resp QueryDownloadOrderResponse
	if err := c.call(ctx, "/xpay/query_download_order", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
