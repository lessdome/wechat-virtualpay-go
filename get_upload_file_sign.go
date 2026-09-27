package virtualpay

import "context"

// GetUploadFileSignRequest 是获取微信支付投诉图片签名头部的请求。
type GetUploadFileSignRequest struct {
	// WxpayURL 微信支付的图片地址，格式为
	// https://api.mch.weixin.qq.com/v3/merchant-service/images/{xxxxxx}
	WxpayURL string `json:"wxpay_url"`
	// ConvertCOS 是否转存到 COS，转存后可获得 30 分钟有效的临时下载地址。
	ConvertCOS bool `json:"convert_cos"`
	// ComplaintID 对应的投诉 ID。
	ComplaintID string `json:"complaint_id"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// GetUploadFileSignResponse 是获取签名头部的响应。
type GetUploadFileSignResponse struct {
	// Sign 微信支付图片请求的 Authorization 头部值。
	Sign string `json:"sign"`
	// CosURL 当 ConvertCOS 为 true 时才有意义，转存后的 URL，30 分钟有效。
	CosURL string `json:"cos_url"`
}

// GetUploadFileSign 获取微信支付投诉图片的签名头部。
//
// 投诉详情里的图片托管在微信支付侧，直接下载会被拒。需要先用本接口拿到
// Sign，再做 HTTP 请求并带上三个头部：
//
//	Authorization: <Sign>
//	Accept: application/json
//	User-Agent: <非空>
//
// 官方文档：POST /xpay/get_upload_file_sign
func (c *Client) GetUploadFileSign(ctx context.Context, req GetUploadFileSignRequest) (*GetUploadFileSignResponse, error) {
	req.Env = c.envInt()
	var resp GetUploadFileSignResponse
	if err := c.call(ctx, "/xpay/get_upload_file_sign", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
