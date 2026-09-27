package virtualpay

import "context"

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
