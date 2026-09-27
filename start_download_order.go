package wechat_virtualpay_go

import "context"

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
	// OrderTypeFilter 要下载哪一类订单（对应协议里的 order_type 字段）。
	//
	// ⚠️ 与 Order.OrderType 同名不同义：那个是**这一单自身的类型**（0/1/7/8），
	// 这个是**筛选条件**（1/2/3/4）。刻意改名以免误读。
	OrderTypeFilter DownloadOrderType `json:"order_type"`
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
