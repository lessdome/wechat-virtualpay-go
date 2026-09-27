package wechat_virtualpay_go

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
