package wechat_virtualpay_go

import (
	"context"
	"fmt"
	"regexp"
)

// 本文件是官方 /xpay/* 里「资金」这一类 3 个接口：
//
//	POST /xpay/create_withdraw_order  创建提现单        access_token + pay_sig
//	POST /xpay/query_withdraw_order   查询提现单        access_token + pay_sig
//	POST /xpay/query_biz_balance      查商家可提现余额  access_token + pay_sig
//
// ⚠️ 本类是**全包唯一金额以「元」为单位**的地方，其余所有接口（GoodsPrice、OrderFee、
// LeftFee、代币的 Amount……）一律是**分**：
//
//	CreateWithdrawOrderRequest.WithdrawAmount  元，字符串形式（"0.01" 就是 1 分钱）
//	QueryWithdrawOrderResponse.WithdrawAmount  元
//	BizBalance.Amount                          元
//
// 差一个单位就是 **100 倍**的金额。所以本类的金额字段一律是 string 而不是 int——用字符串
// 是官方的形状，顺带让「元」这件事在类型上就与别处的 int64（分）分得开，两边互相赋值
// 先得过一次显式转换，而那次转换就是让人停一下的地方。
//
// 其余约定与订单类**逐字同义**，不在这里重抄（见 xpay_order.go 文件头）：凭据显式传参、
// env 是请求结构体上的一个裸 int（0=现网 / 1=沙箱）、字段顺序照官方字段表逐行抄、
// 响应**原值返回**、`err == nil` 不等于成功（成败看 resp.ErrCode）、失败时响应为 nil。
//
// 本类**故意不做**的本地校验：
//
//   - **不查 WithdrawAmount 的写法**：官方只说「元、字符串形式」，没给小数位数、前导零、
//     能不能写成 "1" 这类规范。写错了在微信侧报参数错误，本包不发明语法。
//     （但**单位**那条要自己盯住：按分填就是 100 倍，见上。）
//
// 文件按接口分段，每段是「枚举 → 请求结构体 → 本地校验 → 响应结构体 → 调用函数」，
// 读一个接口只需要看一段。

// withdrawNoRe 是官方对提现单号 withdraw_no 的要求：长度 [8,32]，仅字母、数字、'_'、'-'。
//
// 它碰巧与 refund_order_id 那条一模一样（xpay_order.go 的 refundOrderIDRe），但两条是
// **各自页面**的独立要求，所以各留一份、不合并：哪天有一页改了规范，另一页不该跟着变，
// 合并之后这种改动会静默扩散。
var withdrawNoRe = regexp.MustCompile(`^[0-9A-Za-z_-]{8,32}$`)

// ---------------------------------------------------------------------------
// 1/3  create_withdraw_order —— 创建提现单
//
//	POST /xpay/create_withdraw_order  access_token + pay_sig
// ---------------------------------------------------------------------------

// CreateWithdrawOrderRequest 是创建提现单的请求体。
type CreateWithdrawOrderRequest struct {
	// WithdrawNo 提现单单号，长度 [8,32]，只允许字母、数字、'_'、'-'。
	WithdrawNo string `json:"withdraw_no"`
	// WithdrawAmount 提现金额，**单位是元**，字符串形式——例如提现 1 分钱传 "0.01"。
	//
	// ⚠️ 本字段与可提现余额（BizBalance.Amount）都是**元**，本包其余所有金额
	// （GoodsPrice、OrderFee、PaidFee、RefundFee、代币 Amount 等）全部是**分**。
	// 写代码时务必分清，这里按分填会提现出 100 倍金额。
	//
	// 留空表示全额提现（所以带 omitempty：空串不发给微信，由它按全额处理）。
	WithdrawAmount string `json:"withdraw_amount,omitempty"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

func (r CreateWithdrawOrderRequest) validate() error {
	if !withdrawNoRe.MatchString(r.WithdrawNo) {
		return fmt.Errorf("wechat_virtualpay_go: WithdrawNo %q 非法，须为 8–32 位字母/数字/_/-", r.WithdrawNo)
	}
	// WithdrawAmount 不查写法（留空是「全额提现」，合法），理由见文件头。
	return nil
}

// CreateWithdrawOrderResponse 是创建提现单的响应体。
type CreateWithdrawOrderResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// WithdrawNo 提现单号（就是请求里传的那个）。
	WithdrawNo string `json:"withdraw_no"`
	// WxWithdrawNo 提现单的微信侧单号。
	WxWithdrawNo string `json:"wx_withdraw_no"`
}

// CreateWithdrawOrder 创建提现单。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证。自研小程序传 GetStableAccessToken 换来的 access_token，第三方
//	             平台代商家调用传 authorizer_access_token——两者在这里是同一种东西。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          提现单号与金额（金额**单位是元**、字符串形式；留空=全额提现）。
//
// ⚠️ 创建成功**不等于**提现到账：要拿 WithdrawNo 调 QueryWithdrawOrder 查状态，等到
// WithdrawStatusSuccess 才算成。
//
// 官方文档：POST /xpay/create_withdraw_order
func CreateWithdrawOrder(ctx context.Context, accessToken, appKey string, req CreateWithdrawOrderRequest) (*CreateWithdrawOrderResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp CreateWithdrawOrderResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/create_withdraw_order", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 2/3  query_withdraw_order —— 查询提现单
//
//	POST /xpay/query_withdraw_order  access_token + pay_sig
// ---------------------------------------------------------------------------

// WithdrawStatus 是提现单状态。
type WithdrawStatus int

const (
	WithdrawStatusCreated WithdrawStatus = 1 // 创建成功，提现中
	WithdrawStatusSuccess WithdrawStatus = 2 // 提现成功
	WithdrawStatusFailed  WithdrawStatus = 3 // 提现失败
)

// QueryWithdrawOrderRequest 是查询提现单的请求体。
type QueryWithdrawOrderRequest struct {
	// WithdrawNo 提现单单号，由 CreateWithdrawOrder 返回。
	WithdrawNo string `json:"withdraw_no"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

func (r QueryWithdrawOrderRequest) validate() error {
	if r.WithdrawNo == "" {
		return fmt.Errorf("wechat_virtualpay_go: WithdrawNo 不能为空（由 CreateWithdrawOrder 返回）")
	}
	// 这里**只查非空**，不查创建那一页的 [8,32] 格式：查询是**读**路径，而单号是创建那一步
	// 的产物——历史上创建出来的单号未必满足今天写在文档上的规范，读它不该被格式挡住。
	// 格式判断留给创建那一步。
	return nil
}

// QueryWithdrawOrderResponse 是查询提现单的响应体。
//
// ⚠️ 本接口的金额与时间字段都是**字符串**（微信如此返回），不是数字——别照订单类那些
// int64 时间戳去写。金额的单位还是**元**（见文件头）。
type QueryWithdrawOrderResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// WithdrawNo 提现单号。
	WithdrawNo string `json:"withdraw_no"`
	// Status 提现状态，取值见 WithdrawStatus。
	Status WithdrawStatus `json:"status"`
	// WithdrawAmount 提现金额，**单位是元**，字符串形式。
	WithdrawAmount string `json:"withdraw_amount"`
	// WxWithdrawNo 提现单的微信侧单号。
	WxWithdrawNo string `json:"wx_withdraw_no"`
	// WithdrawSuccessTimestamp 提现成功的秒级时间戳——**字符串形式**。
	WithdrawSuccessTimestamp string `json:"withdraw_success_timestamp"`
	// CreateTime 提现单创建时间，字符串形式。
	CreateTime string `json:"create_time"`
	// FailReason 提现失败的原因。仅在 Status=WithdrawStatusFailed 时有值。
	FailReason string `json:"fail_reason"`
}

// QueryWithdrawOrder 查询提现单的状态。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 CreateWithdrawOrder。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          只有 WithdrawNo（由 CreateWithdrawOrder 返回）与 Env。
//
// 状态到了终态（WithdrawStatusSuccess / WithdrawStatusFailed）就不必再查了；失败的原因
// 在 FailReason 里。
//
// 官方文档：POST /xpay/query_withdraw_order
func QueryWithdrawOrder(ctx context.Context, accessToken, appKey string, req QueryWithdrawOrderRequest) (*QueryWithdrawOrderResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp QueryWithdrawOrderResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/query_withdraw_order", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 3/3  query_biz_balance —— 查商家账户可提现余额
//
//	POST /xpay/query_biz_balance  access_token + pay_sig
// ---------------------------------------------------------------------------

// BizBalance 是商家账户的可提现余额。
type BizBalance struct {
	// Amount 可提现余额，**单位是元**（字符串形式）。见文件头的单位说明。
	Amount string `json:"amount"`
	// CurrencyCode 币种，一般为 CNY。
	CurrencyCode string `json:"currency_code"`
}

// QueryBizBalanceRequest 是查询商家账户可提现余额的请求体。
type QueryBizBalanceRequest struct {
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

// validate 没有可查的字段：本请求体只有 env，而 env 由 checkEnv 查（0/1 之外都在本地
// 拦下）。留着这个方法是为了让本类三个接口的调用形状一样——写死的那五步里，校验那一步
// 不该因为「刚好没得查」就少一步。
func (r QueryBizBalanceRequest) validate() error { return nil }

// QueryBizBalanceResponse 是查询商家账户可提现余额的响应体。
type QueryBizBalanceResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// BalanceAvailable 可提现余额。金额单位是**元**，见 BizBalance。
	BalanceAvailable BizBalance `json:"balance_available"`
}

// QueryBizBalance 查询商家账户里的可提现余额。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 CreateWithdrawOrder。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          只有 Env——本接口没有别的请求参数，查询对象就是调用凭据对应的那个商户。
//
// 官方文档：POST /xpay/query_biz_balance
func QueryBizBalance(ctx context.Context, accessToken, appKey string, req QueryBizBalanceRequest) (*QueryBizBalanceResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp QueryBizBalanceResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/query_biz_balance", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
