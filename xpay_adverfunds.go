package wechat_virtualpay_go

import (
	"context"
)

// 广告金这一批（7 个接口）的官方文档质量明显低于支付主链路：请求体里的 env 字段
// 注释统一写着「仅作为签名校验（查询的结果都是正式环境的）」，但它们的 query 参数
// 表里**并没有 pay_sig**——两处互相矛盾。
//
// 取舍偏向「不签名」，理由是那句注释不足采信：「仅作为签名校验」是**跨页复制的
// 模板文字**，明确需要 pay_sig 的 query_biz_balance 页上同样有这句。而「不签名」
// 一侧有三处独立证据——query 参数表、HTTPS 示例 URL（`?access_token=ACCESS_TOKEN`）、
// 注意事项，都不含 pay_sig。
//
// 故本包按参数表实现（callMerchant，不签名）。若实测返回 268490003，
// 把对应调用的 callMerchant 改为 callPaySig 即可。

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
	// TransferAccountName 充值账户名称。
	TransferAccountName string `json:"transfer_account_name"`
	// TransferAccountUID 充值账户 uid。
	TransferAccountUID int64 `json:"transfer_account_uid"`
	// TransferAccountAgencyID 充值账户服务商账号 id。
	TransferAccountAgencyID int64 `json:"transfer_account_agency_id"`
	// TransferAccountAgencyName 充值账户服务商账号名称。
	TransferAccountAgencyName string `json:"transfer_account_agency_name"`
	// State 审核状态。
	State TransferAccountState `json:"state"`
	// BindResult 绑定结果。
	BindResult TransferAccountBindResult `json:"bind_result"`
	// ErrorMsg 错误信息。
	ErrorMsg string `json:"error_msg"`
}

// QueryTransferAccountRequest 是查询广告金充值账户的请求。
type QueryTransferAccountRequest struct {
	// Env 环境标识。本包只支持现网，固定为 0。
	Env int `json:"env"`
}

// QueryTransferAccountResponse 是查询广告金充值账户的响应。
type QueryTransferAccountResponse struct {
	// AcctList 广告金充值账户列表。
	AcctList []TransferAccount `json:"acct_list"`
}

// QueryTransferAccount 查询广告金充值账户。
//
// 官方文档：POST /xpay/query_transfer_account
func (c *Client) QueryTransferAccount(ctx context.Context, req QueryTransferAccountRequest) (*QueryTransferAccountResponse, error) {
	var resp QueryTransferAccountResponse
	if err := c.callMerchant(ctx, "/xpay/query_transfer_account", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AdFundType 是广告金发放原因。
type AdFundType int

const (
	AdFundTypeGeneral AdFundType = 0 // 通用赠送
	AdFundTypeAd      AdFundType = 1 // 广告激励
	AdFundTypeTarget  AdFundType = 2 // 定向激励
)

// AdFundFilter 是查询广告金发放记录的过滤条件。
type AdFundFilter struct {
	// SettleBegin 结算周期开始时间，unix 秒级时间戳。
	SettleBegin int64 `json:"settle_begin,omitempty"`
	// SettleEnd 结算周期结束时间，unix 秒级时间戳。
	SettleEnd int64 `json:"settle_end,omitempty"`
	// FundType 广告金发放原因。使用指针以区分「不筛选」与「筛选 0（通用赠送）」。
	FundType *AdFundType `json:"fund_type,omitempty"`
}

// QueryAdverFundsRequest 是查询广告金发放记录的请求。
type QueryAdverFundsRequest struct {
	// Page 查询页码，不小于 1。
	Page int `json:"page,omitempty"`
	// PageSize 每页记录数量。
	PageSize int `json:"page_size,omitempty"`
	// Filter 查询过滤条件。
	Filter *AdFundFilter `json:"filter,omitempty"`
	// Env 环境标识。本包只支持现网，固定为 0。
	Env int `json:"env"`
}

// AdverFund 是一条广告金发放记录。
type AdverFund struct {
	SettleBegin  int64      `json:"settle_begin"`  // 结算周期开始时间，unix 秒级时间戳
	SettleEnd    int64      `json:"settle_end"`    // 结算周期结束时间，unix 秒级时间戳
	TotalAmount  int64      `json:"total_amount"`  // 发放广告金金额，单位分
	RemainAmount int64      `json:"remain_amount"` // 剩余可用广告金金额，单位分
	ExpireTime   int64      `json:"expire_time"`   // 广告金过期时间，unix 秒级时间戳
	FundType     AdFundType `json:"fund_type"`     // 广告金发放原因
	FundID       string     `json:"fund_id"`       // 广告金发放 ID
}

// QueryAdverFundsResponse 是查询广告金发放记录的响应。
type QueryAdverFundsResponse struct {
	// AdverFundsList 广告金发放记录列表。
	AdverFundsList []AdverFund `json:"adver_funds_list"`
	// TotalPage 查询命中总的页数。
	TotalPage int `json:"total_page"`
}

// QueryAdverFunds 查询广告金发放记录。
//
// 官方文档：POST /xpay/query_adver_funds
func (c *Client) QueryAdverFunds(ctx context.Context, req QueryAdverFundsRequest) (*QueryAdverFundsResponse, error) {
	var resp QueryAdverFundsResponse
	if err := c.callMerchant(ctx, "/xpay/query_adver_funds", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateFundsBillRequest 是充值广告金的请求。
//
// 充值金额单位是**分**（与 CreateWithdrawOrder 的元不同）。
type CreateFundsBillRequest struct {
	// TransferAmount 充值金额，单位分。
	TransferAmount int64 `json:"transfer_amount"`
	// TransferAccountUID 充值账户 uid。
	TransferAccountUID int64 `json:"transfer_account_uid"`
	// TransferAccountName 充值账户名称。
	TransferAccountName string `json:"transfer_account_name"`
	// TransferAccountAgencyID 充值账户服务商账号 id。
	TransferAccountAgencyID int64 `json:"transfer_account_agency_id"`
	// RequestID 每一次请求的唯一 id（不超过 1024 字符）。
	// **相同 id 的不同请求会被视为重复请求**——这是本接口的幂等键。
	RequestID string `json:"request_id"`
	// SettleBegin 充值所使用的广告金对应的结算周期开始时间，unix 秒级时间戳。
	SettleBegin int64 `json:"settle_begin"`
	// SettleEnd 充值所使用的广告金对应的结算周期结束时间，unix 秒级时间戳。
	SettleEnd int64 `json:"settle_end"`
	// AuthorizeAdvertise 是否授权广告数据：0 否，1 是。
	AuthorizeAdvertise int `json:"authorize_advertise"`
	// FundType 广告金发放原因。
	FundType AdFundType `json:"fund_type"`
	// Env 环境标识。本包只支持现网，固定为 0。
	Env int `json:"env"`
}

// CreateFundsBillResponse 是充值广告金的响应。
type CreateFundsBillResponse struct {
	// BillID 充值单 id。
	BillID string `json:"bill_id"`
}

// CreateFundsBill 充值广告金。
//
// 幂等靠 RequestID：相同 RequestID 的重复请求会被微信识别为重复。
//
// 官方文档：POST /xpay/create_funds_bill
func (c *Client) CreateFundsBill(ctx context.Context, req CreateFundsBillRequest) (*CreateFundsBillResponse, error) {
	var resp CreateFundsBillResponse
	if err := c.callMerchant(ctx, "/xpay/create_funds_bill", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BindTransferAccountRequest 是绑定广告金充值账户的请求。
type BindTransferAccountRequest struct {
	// TransferAccountUID 充值账户 uid。
	TransferAccountUID int64 `json:"transfer_account_uid,omitempty"`
	// TransferAccountOrgName 充值账户主体名称。
	TransferAccountOrgName string `json:"transfer_account_org_name,omitempty"`
	// Env 环境标识。本包只支持现网，固定为 0。
	Env int `json:"env"`
}

// BindTransferAccount 绑定广告金充值账户。
//
// 官方文档：POST /xpay/bind_transfer_accout
// （路径里的 accout 是微信官方的拼写，不是笔误，改动会导致 404。）
func (c *Client) BindTransferAccount(ctx context.Context, req BindTransferAccountRequest) error {
	return c.callMerchant(ctx, "/xpay/bind_transfer_accout", req, nil)
}

// FundsBillStatus 是广告金充值单状态。
type FundsBillStatus int

const (
	FundsBillProcessing FundsBillStatus = 0 // 充值中
	FundsBillSuccess    FundsBillStatus = 1 // 充值成功
	FundsBillFailed     FundsBillStatus = 2 // 充值失败
)

// FundsBillFilter 是查询广告金充值记录的过滤条件。
type FundsBillFilter struct {
	// OperTimeBegin 查询充值开始时间，unix 秒级时间戳。
	OperTimeBegin int64 `json:"oper_time_begin"`
	// OperTimeEnd 查询充值结束时间，unix 秒级时间戳。
	OperTimeEnd int64 `json:"oper_time_end"`
	// BillID 广告金充值单 ID，可选。
	BillID string `json:"bill_id,omitempty"`
	// RequestID 调用 CreateFundsBill 时传入的 request_id，可选。
	RequestID string `json:"request_id,omitempty"`
}

// QueryFundsBillRequest 是查询广告金充值记录的请求。
type QueryFundsBillRequest struct {
	// Page 查询页码，不小于 1。
	Page int `json:"page"`
	// PageSize 每页记录数量。
	PageSize int `json:"page_size"`
	// Filter 查询过滤条件。
	Filter FundsBillFilter `json:"filter"`
	// Env 环境标识。本包只支持现网，固定为 0。
	Env int `json:"env"`
}

// FundsBill 是一条广告金充值记录。
type FundsBill struct {
	// BillID 充值单 ID。
	BillID              string          `json:"bill_id"`
	OperTime            int64           `json:"oper_time"`             // 充值时间，unix 秒级时间戳
	SettleBegin         int64           `json:"settle_begin"`          // 结算周期开始时间
	SettleEnd           int64           `json:"settle_end"`            // 结算周期结束时间
	FundID              string          `json:"fund_id"`               // 对应广告金 ID
	TransferAccountName string          `json:"transfer_account_name"` // 充值账户
	TransferAccountUID  int64           `json:"transfer_account_uid"`  // 充值账户 UID
	TransferAmount      int64           `json:"transfer_amount"`       // 充值金额，单位分
	Status              FundsBillStatus `json:"status"`                // 充值状态
	RequestID           string          `json:"request_id"`            // 充值时的 request_id
}

// QueryFundsBillResponse 是查询广告金充值记录的响应。
type QueryFundsBillResponse struct {
	// BillList 广告金充值记录列表。
	BillList []FundsBill `json:"bill_list"`
	// TotalPage 查询命中总的页数。
	TotalPage int `json:"total_page"`
}

// QueryFundsBill 查询广告金充值记录。
//
// 官方文档：POST /xpay/query_funds_bill
func (c *Client) QueryFundsBill(ctx context.Context, req QueryFundsBillRequest) (*QueryFundsBillResponse, error) {
	var resp QueryFundsBillResponse
	if err := c.callMerchant(ctx, "/xpay/query_funds_bill", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RecoverBillFilter 是查询广告金回收记录的过滤条件。
type RecoverBillFilter struct {
	// RecoverTimeBegin 查询回收开始时间，unix 秒级时间戳。
	RecoverTimeBegin int64 `json:"recover_time_begin"`
	// RecoverTimeEnd 查询回收结束时间，unix 秒级时间戳。
	RecoverTimeEnd int64 `json:"recover_time_end"`
	// BillID 广告金回收单 ID。文档标为必填，但说明里又写"(可选)"，此处按必填处理。
	BillID string `json:"bill_id"`
}

// QueryRecoverBillRequest 是查询广告金回收记录的请求。
type QueryRecoverBillRequest struct {
	// Page 查询页码，不小于 1。
	Page int `json:"page"`
	// PageSize 每页记录数量。
	PageSize int `json:"page_size"`
	// Filter 查询过滤条件。
	Filter RecoverBillFilter `json:"filter"`
	// Env 环境标识。本包只支持现网，固定为 0。
	Env int `json:"env"`
}

// RecoverBill 是一条广告金回收记录。
type RecoverBill struct {
	// BillID 回收单 ID。
	BillID             string   `json:"bill_id"`
	RecoverTime        int64    `json:"recover_time"`         // 回收时间，unix 秒级时间戳
	SettleBegin        int64    `json:"settle_begin"`         // 结算周期开始时间
	SettleEnd          int64    `json:"settle_end"`           // 结算周期结束时间
	FundID             string   `json:"fund_id"`              // 对应的发放广告金 ID
	RecoverAccountName string   `json:"recover_account_name"` // 回收广告金账户
	RecoverAmount      int64    `json:"recover_amount"`       // 回收金额，单位分
	RefundOrderList    []string `json:"refund_order_list"`    // 对应的退款订单 id
}

// QueryRecoverBillResponse 是查询广告金回收记录的响应。
type QueryRecoverBillResponse struct {
	// BillList 广告金回收记录列表。
	BillList []RecoverBill `json:"bill_list"`
	// TotalPage 查询命中总的页数。
	TotalPage int `json:"total_page"`
}

// QueryRecoverBill 查询广告金回收记录。
//
// 官方文档：POST /xpay/query_recover_bill
func (c *Client) QueryRecoverBill(ctx context.Context, req QueryRecoverBillRequest) (*QueryRecoverBillResponse, error) {
	var resp QueryRecoverBillResponse
	if err := c.callMerchant(ctx, "/xpay/query_recover_bill", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DownloadAdverFundsOrderRequest 是下载广告金对应商户订单信息的请求。
type DownloadAdverFundsOrderRequest struct {
	// FundID 广告金发放 ID。
	FundID string `json:"fund_id"`
	// Env 环境标识。本包只支持现网，固定为 0。
	Env int `json:"env"`
}

// DownloadAdverFundsOrderResponse 是下载广告金对应商户订单信息的响应。
type DownloadAdverFundsOrderResponse struct {
	// URL 订单下载链接。
	URL string `json:"url"`
}

// DownloadAdverFundsOrder 下载广告金对应的商户订单信息。
//
// ⚠️ 文档的「注意事项」有两条，本方法不会替你处理：
//   - **仅支持通用赠送广告金**（fund_type=0）对应订单的下载；
//   - **第一次调用只触发生成下载 url**，返回的 url 可能尚未生成，需间隔轮询再次
//     调用才能拿到最终链接。
//
// 官方文档：POST /xpay/download_adverfunds_order
func (c *Client) DownloadAdverFundsOrder(ctx context.Context, req DownloadAdverFundsOrderRequest) (*DownloadAdverFundsOrderResponse, error) {
	var resp DownloadAdverFundsOrderResponse
	if err := c.callMerchant(ctx, "/xpay/download_adverfunds_order", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
