package virtualpay

import "context"

// GoodsTaskStatus 是道具「上传/发布」批量任务的状态（query_upload_goods、
// query_publish_goods 响应中的 status）。
type GoodsTaskStatus int

const (
	GoodsTaskNone        GoodsTaskStatus = 0 // 无任务在运行
	GoodsTaskRunning     GoodsTaskStatus = 1 // 任务运行中
	GoodsTaskPartialFail GoodsTaskStatus = 2 // 上传/发布失败或部分失败（任务已完成）
	GoodsTaskSuccess     GoodsTaskStatus = 3 // 上传/发布成功
)

// GoodsItemStatus 是单个道具的上传/发布状态。
//
// 上传与发布共用同一套取值（upload_status / publish_status）。
type GoodsItemStatus int

const (
	GoodsItemPending GoodsItemStatus = 0 // 上传中/发布中
	GoodsItemExists  GoodsItemStatus = 1 // id 已经存在
	GoodsItemOK      GoodsItemStatus = 2 // 上传/发布成功
	GoodsItemFailed  GoodsItemStatus = 3 // 上传/发布失败
)

// UploadGoodsItem 是要上传的道具。
type UploadGoodsItem struct {
	// ID 道具 ID，长度 (0,20]，只允许字母、数字、'_'、'-'（中文算一个字符）。
	ID string `json:"id"`
	// Name 道具名称，长度 (0,20]。
	Name string `json:"name"`
	// Price 道具单价，单位分，需大于 0。
	Price int64 `json:"price"`
	// Remark 道具备注，长度 (0,1024]。
	Remark string `json:"remark"`
	// ItemURL 道具图片 URL，当前仅支持 jpg、png 等格式。
	ItemURL string `json:"item_url"`
}

// StartUploadGoodsRequest 是启动批量上传道具任务的请求。
type StartUploadGoodsRequest struct {
	// UploadItem 上传的道具列表。一次仅支持上传一个道具，多个道具需分多次请求。
	UploadItem []UploadGoodsItem `json:"upload_item"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// StartUploadGoods 启动批量上传道具任务。
//
// 任务为异步：本方法只表示「任务已受理」，需轮询 QueryUploadGoods 查看结果。
// 一次仅支持上传一个道具，多个道具需分多次请求。
//
// 官方文档：POST /xpay/start_upload_goods
func (c *Client) StartUploadGoods(ctx context.Context, req StartUploadGoodsRequest) error {
	req.Env = c.envInt()
	return c.call(ctx, "/xpay/start_upload_goods", req, authPaySig, "", nil)
}

// UploadedGoodsItem 是查询上传任务结果里的单个道具。
type UploadedGoodsItem struct {
	// ID 道具 ID。
	ID string `json:"id"`
	// Name 道具名称。
	Name string `json:"name"`
	// Price 道具单价，单位分。
	Price int64 `json:"price"`
	// Remark 道具备注。
	Remark string `json:"remark"`
	// ItemURL 道具图片 URL（微信转存后的地址）。
	ItemURL string `json:"item_url"`
	// UploadStatus 该道具的上传状态。
	UploadStatus GoodsItemStatus `json:"upload_status"`
	// ErrMsg 上传失败的原因。
	ErrMsg string `json:"errmsg"`
}

// QueryUploadGoodsRequest 是查询批量上传道具任务的请求。
type QueryUploadGoodsRequest struct {
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// QueryUploadGoodsResponse 是查询批量上传道具任务的响应。
type QueryUploadGoodsResponse struct {
	// UploadItem 上传的道具列表。
	UploadItem []UploadedGoodsItem `json:"upload_item"`
	// Status 整体任务状态。
	Status GoodsTaskStatus `json:"status"`
}

// QueryUploadGoods 查询批量上传道具任务的结果。
//
// 官方文档：POST /xpay/query_upload_goods
func (c *Client) QueryUploadGoods(ctx context.Context, req QueryUploadGoodsRequest) (*QueryUploadGoodsResponse, error) {
	req.Env = c.envInt()
	var resp QueryUploadGoodsResponse
	if err := c.call(ctx, "/xpay/query_upload_goods", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PublishGoodsItem 是要发布的道具。
type PublishGoodsItem struct {
	// ID 道具 ID，即添加到开发环境时传的道具 ID。
	ID string `json:"id"`
}

// StartPublishGoodsRequest 是启动批量发布道具任务的请求。
type StartPublishGoodsRequest struct {
	// PublishItem 发布的道具列表。一次仅支持发布一个道具，多个道具需分多次请求。
	PublishItem []PublishGoodsItem `json:"publish_item"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// StartPublishGoods 启动批量发布道具任务（把开发环境的道具发布到现网）。
//
// 任务为异步：需轮询 QueryPublishGoods 查看结果。一次仅支持发布一个道具。
//
// 注意：道具发布后约 10 分钟才生效，生效前调用会返回 -15014。
//
// 官方文档：POST /xpay/start_publish_goods
func (c *Client) StartPublishGoods(ctx context.Context, req StartPublishGoodsRequest) error {
	req.Env = c.envInt()
	return c.call(ctx, "/xpay/start_publish_goods", req, authPaySig, "", nil)
}

// PublishedGoodsItem 是查询发布任务结果里的单个道具。
type PublishedGoodsItem struct {
	// ID 道具 ID。
	ID string `json:"id"`
	// PublishStatus 该道具的发布状态。
	PublishStatus GoodsItemStatus `json:"publish_status"`
	// ErrMsg 发布失败的原因。
	ErrMsg string `json:"errmsg"`
}

// QueryPublishGoodsRequest 是查询批量发布道具任务的请求。
type QueryPublishGoodsRequest struct {
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// QueryPublishGoodsResponse 是查询批量发布道具任务的响应。
type QueryPublishGoodsResponse struct {
	// PublishItem 发布的道具列表。
	PublishItem []PublishedGoodsItem `json:"publish_item"`
	// Status 整体任务状态。
	Status GoodsTaskStatus `json:"status"`
}

// QueryPublishGoods 查询批量发布道具任务的结果。
//
// 官方文档：POST /xpay/query_publish_goods
func (c *Client) QueryPublishGoods(ctx context.Context, req QueryPublishGoodsRequest) (*QueryPublishGoodsResponse, error) {
	req.Env = c.envInt()
	var resp QueryPublishGoodsResponse
	if err := c.call(ctx, "/xpay/query_publish_goods", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
