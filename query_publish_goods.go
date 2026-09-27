package wechat_virtualpay_go

import "context"

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
	Status GoodsBatchStatus `json:"status"`
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
