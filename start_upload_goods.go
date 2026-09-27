package virtualpay

import "context"

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
