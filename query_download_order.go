package virtualpay

import "context"

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
