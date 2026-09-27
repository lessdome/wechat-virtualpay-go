package wechat_virtualpay_go

import "context"

// WithdrawStatus 是提现单状态。
type WithdrawStatus int

const (
	WithdrawStatusCreated WithdrawStatus = 1 // 创建成功，提现中
	WithdrawStatusSuccess WithdrawStatus = 2 // 提现成功
	WithdrawStatusFailed  WithdrawStatus = 3 // 提现失败
)

// QueryWithdrawOrderRequest 是查询提现单的请求。
type QueryWithdrawOrderRequest struct {
	// WithdrawNo 提现单单号。
	WithdrawNo string `json:"withdraw_no"`
	// Env 由 Client 按 Config.Env 自动填充，调用方无需设置（设置了也会被覆盖）。
	Env int `json:"env"`
}

// QueryWithdrawOrderResponse 是查询提现单的响应。
//
// 注意：本接口的金额与时间字段都是**字符串**（微信如此返回），不是数字。
type QueryWithdrawOrderResponse struct {
	// WithdrawNo 提现单号。
	WithdrawNo string `json:"withdraw_no"`
	// Status 提现状态。
	Status WithdrawStatus `json:"status"`
	// WithdrawAmount 提现金额，**单位是元**。
	WithdrawAmount string `json:"withdraw_amount"`
	// WxWithdrawNo 提现单的微信侧单号。
	WxWithdrawNo string `json:"wx_withdraw_no"`
	// WithdrawSuccessTimestamp 提现成功的秒级时间戳。
	WithdrawSuccessTimestamp string `json:"withdraw_success_timestamp"`
	// CreateTime 提现单创建时间。
	CreateTime string `json:"create_time"`
	// FailReason 提现失败的原因。
	FailReason string `json:"fail_reason"`
}

// QueryWithdrawOrder 查询提现单。
//
// 官方文档：POST /xpay/query_withdraw_order
func (c *Client) QueryWithdrawOrder(ctx context.Context, req QueryWithdrawOrderRequest) (*QueryWithdrawOrderResponse, error) {
	req.Env = c.envInt()
	var resp QueryWithdrawOrderResponse
	if err := c.call(ctx, "/xpay/query_withdraw_order", req, authPaySig, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
