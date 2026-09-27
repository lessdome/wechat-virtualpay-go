package wechat_virtualpay_go

import "context"

// BindTransferAccountRequest 是绑定广告金充值账户的请求。
type BindTransferAccountRequest struct {
	// TransferAccountUID 充值账户 uid。
	TransferAccountUID int64 `json:"transfer_account_uid,omitempty"`
	// TransferAccountOrgName 充值账户主体名称。
	TransferAccountOrgName string `json:"transfer_account_org_name,omitempty"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// BindTransferAccount 绑定广告金充值账户。
//
// 官方文档：POST /xpay/bind_transfer_accout
// （路径里的 accout 是微信官方的拼写，不是笔误，改动会导致 404。）
func (c *Client) BindTransferAccount(ctx context.Context, req BindTransferAccountRequest) error {
	req.Env = c.envInt()
	return c.call(ctx, "/xpay/bind_transfer_accout", req, authTokenOnly, "", nil)
}
