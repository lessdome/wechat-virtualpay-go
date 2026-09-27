package virtualpay

import "context"

// 广告金这一批（7 个接口）的官方文档质量明显低于支付主链路：请求体里的 env 字段
// 注释统一写着「仅作为签名校验（查询的结果都是正式环境的）」，但它们的 query 参数
// 表里**并没有 pay_sig**——两处互相矛盾。本包按参数表实现（authTokenOnly，不签名）。
// 若实测返回 -15006，把对应调用的 authTokenOnly 改为 authPaySig 即可。

// TransferAccountState 是广告金充值账户的审核状态。
type TransferAccountState int

const (
	TransferAccountPending  TransferAccountState = 0 // 待审核
	TransferAccountApproved TransferAccountState = 1 // 审核通过
	TransferAccountRejected TransferAccountState = 2 // 审核驳回
)

// TransferAccountBindResult 是广告金充值账户的绑定结果。
type TransferAccountBindResult int

const (
	TransferAccountBindOK   TransferAccountBindResult = 1 // 绑定成功
	TransferAccountBindFail TransferAccountBindResult = 2 // 绑定失败
)

// TransferAccount 是广告金充值账户。
type TransferAccount struct {
	TransferAccountName       string                    `json:"transfer_account_name"`
	TransferAccountUID        int64                     `json:"transfer_account_uid"`
	TransferAccountAgencyID   int64                     `json:"transfer_account_agency_id"`
	TransferAccountAgencyName string                    `json:"transfer_account_agency_name"`
	State                     TransferAccountState      `json:"state"`
	BindResult                TransferAccountBindResult `json:"bind_result"`
	ErrorMsg                  string                    `json:"error_msg"`
}

// QueryTransferAccountRequest 是查询广告金充值账户的请求。
type QueryTransferAccountRequest struct {
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// QueryTransferAccountResponse 是查询广告金充值账户的响应。
type QueryTransferAccountResponse struct {
	AcctList []TransferAccount `json:"acct_list"`
}

// QueryTransferAccount 查询广告金充值账户。
//
// 官方文档：POST /xpay/query_transfer_account
func (c *Client) QueryTransferAccount(ctx context.Context, req QueryTransferAccountRequest) (*QueryTransferAccountResponse, error) {
	req.Env = c.envInt()
	var resp QueryTransferAccountResponse
	if err := c.call(ctx, "/xpay/query_transfer_account", req, authTokenOnly, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
