package virtualpay

import "context"

// UploadVPFileRequest 是上传媒体文件的请求。
//
// Base64Img 与 ImgURL 二选一，且 ImgURL 优先。Base64Img 最大 1M，ImgURL 最大 2M。
type UploadVPFileRequest struct {
	// Base64Img 经 base64 编码后的图片内容，最多 1M。
	Base64Img string `json:"base64_img,omitempty"`
	// ImgURL 图片 URL，需能直接下载（不能返回 302 等），最高 2M。**优先使用本字段**。
	ImgURL string `json:"img_url,omitempty"`
	// FileName 图片名称。
	FileName string `json:"file_name"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// UploadVPFileResponse 是上传媒体文件的响应。
type UploadVPFileResponse struct {
	// FileID 返回的文件 ID，用于 ResponseComplaint 的 ResponseImages。
	FileID string `json:"file_id"`
}

// UploadVPFile 上传媒体文件（图片、凭证等），用于回复投诉。
//
// 官方文档：POST /xpay/upload_vp_file
func (c *Client) UploadVPFile(ctx context.Context, req UploadVPFileRequest) (*UploadVPFileResponse, error) {
	req.Env = c.envInt()
	var resp UploadVPFileResponse
	if err := c.call(ctx, "/xpay/upload_vp_file", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
