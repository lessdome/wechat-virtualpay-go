package wechat_virtualpay_go

import (
	"context"
	"fmt"
)

// 本文件是官方 /xpay/* 里「广告金」这一类 7 个接口：
//
//	POST /xpay/query_transfer_account     查询广告金充值账户    access_token
//	POST /xpay/query_adver_funds          查询广告金发放记录    access_token
//	POST /xpay/create_funds_bill          充值广告金            access_token
//	POST /xpay/bind_transfer_accout       绑定广告金充值账户    access_token
//	POST /xpay/query_funds_bill           查询广告金充值记录    access_token
//	POST /xpay/query_recover_bill         查询广告金回收记录    access_token
//	POST /xpay/download_adverfunds_order  下载广告金对应订单    access_token
//
// ⚠️ 七个都在**最粗的那一档**：只带 access_token，没有 pay_sig。但官方这几页的请求体里
// 有一句跨页复制的模板文字，说 env「仅作为签名校验（查询的结果都是正式环境的）」——它跟
// 这几页自己的参数表**互相矛盾**：参数表里根本没有 pay_sig 这一项。
//
// 本包按参数表实现（不签名）。理由是那句注释不足采信：「仅作为签名校验」是模板文字，
// 连**明确需要 pay_sig** 的 query_biz_balance 页上都有同一句。而「不签名」一侧有三处
// 各自独立的证据——query 参数表、HTTPS 示例 URL（只有 ?access_token=ACCESS_TOKEN）、
// 注意事项，三处都没有 pay_sig。三比一，且多的一边是模板。
//
// ⚠️ 也正因为不签名，本类的 env 既不参与签名、也不切换数据源：官方写明查询结果都是
// **正式环境**的，传 env=1 不会得到沙箱数据。它仍然留在请求体里（官方字段表确实有这一行），
// 取值照样只允许 0/1（见 checkEnv）。
//
// 真机联调时若这七个里有哪个回了 268490003（签名错误），把它换到 PostWithPaySig 即可：
// 每个函数只需多收一个 appKey 参数、多一行 checkAppKey。
//
// 「`pre-rewrite` 分支上那份被删掉的实现」这七个也全是 `callMerchant`（即本档）——两次
// 照同一张参数表读出的结论一致。不算独立证据，但至少说明这不是这一次新押的注。
//
// 本类的本地校验遵循同一条规则，只有两半：
//
//  1. **官方字段表标了必填的，空值/零值在本地拦**（本类不少必填字段在广告金这条链路上
//     是「用户手上现成的东西」，漏传只会在微信侧变成一个语焉不详的参数错误）。
//  2. **从语义推得不可能成立的**也拦：金额（单位分）不可能 <= 0，时间区间不可能反着写，
//     必填的时间戳不可能是 0（unix 秒的 0 是 1970 年，只可能是漏填）。这半条是**推出来的**，
//     不是文档写的，注释里逐条注明。
//
// 金额单位一律是**分**（与资金类的元不同；见 xpay_funds.go 文件头的单位说明）。
//
// 其余约定与订单类**逐字同义**，不在这里重抄（见 xpay_order.go 文件头）：凭据显式传参、
// env 是请求结构体上的一个裸 int（0=现网 / 1=沙箱）、字段顺序照官方字段表逐行抄、
// 响应**原值返回**、`err == nil` 不等于成功（成败看 resp.ErrCode）、失败时响应为 nil。
//
// 文件按接口分段，每段是「枚举 → 请求结构体 → 本地校验 → 响应结构体 → 调用函数」，
// 读一个接口只需要看一段。

// AdFundType 是广告金发放原因。查询时用它筛选，充值时要说明这笔钱来自哪一类广告金。
//
// ⚠️ 零值是 AdFundTypeGeneral(0)「通用赠送」，**不是**「未设置」——所以按类型筛选时
// 它是一个有意义的取值，不能用「零值＝不筛」来判断。查询侧的 AdFundFilter.FundType 因此
// 是**指针**：nil 才是「不按类型筛」（理由见那个字段）。
type AdFundType int

const (
	AdFundTypeGeneral AdFundType = 0 // 通用赠送
	AdFundTypeAd      AdFundType = 1 // 广告激励
	AdFundTypeTarget  AdFundType = 2 // 定向激励
)

// checkUnixRange 校验一对「开始/结束」unix 秒级时间戳。
//
// required 为 true 表示这两个时间戳都是必填的（官方字段表里没标可选）：那样 0 就是漏填
// ——unix 秒的 0 是 1970 年，不可能是本意。为 false 时（AdFundFilter 那三个）只查「既然
// 填了就得成区间」，两个都不填是合法的「不按时间筛」。
//
// 区间一律取**闭区间**语义（end >= begin 就放行）：官方没说这两端是开是闭，而相等这种
// 退化情形在「就查这一秒」时是能解释的，不值得为它编一条规则。
func checkUnixRange(begin, end int64, what string, required bool) error {
	if required {
		if begin <= 0 {
			return fmt.Errorf("wechat_virtualpay_go: %s 的开始时间戳必须为正的 unix 秒（0 是 1970 年，只会是漏填）", what)
		}
		if end <= 0 {
			return fmt.Errorf("wechat_virtualpay_go: %s 的结束时间戳必须为正的 unix 秒（0 是 1970 年，只会是漏填）", what)
		}
	}
	if begin != 0 && end != 0 && end < begin {
		return fmt.Errorf("wechat_virtualpay_go: %s 的结束时间(%d) 早于开始时间(%d)", what, end, begin)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 1/7  query_transfer_account —— 查询广告金充值账户
//
//	POST /xpay/query_transfer_account  access_token
// ---------------------------------------------------------------------------

// TransferAccountState 是广告金充值账户的审核状态。
type TransferAccountState int

const (
	TransferAccountPending  TransferAccountState = 0 // 待审核
	TransferAccountApproved TransferAccountState = 1 // 审核通过
	TransferAccountRejected TransferAccountState = 2 // 审核驳回
)

// TransferAccountBindResult 是广告金充值账户的绑定结果。
//
// ⚠️ 这套取值**从 1 开始**（没有 0）：零值不在取值集合里，读到 0 说明响应里根本没回
// 这个字段（多半是账户还没走到绑定那一步），不要当成某个具体结果。
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
	// State 审核状态，取值见 TransferAccountState。
	State TransferAccountState `json:"state"`
	// BindResult 绑定结果，取值见 TransferAccountBindResult。
	//
	// ⚠️ 取值从 1 开始，零值不在集合里——见那个类型的说明。
	BindResult TransferAccountBindResult `json:"bind_result"`
	// ErrorMsg 错误信息。
	//
	// ⚠️ 字段名是 error_msg（**不是**公共头那个 errmsg）：它是**这一个账户**的问题说明，
	// 与响应整体的成败是两件事——查得到账户列表（errcode=0）不代表每个账户都没问题。
	ErrorMsg string `json:"error_msg"`
}

// QueryTransferAccountRequest 是查询广告金充值账户的请求体。
type QueryTransferAccountRequest struct {
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	//
	// ⚠️ 本类**不签名**，而且官方写明查询结果都是正式环境的：这里填 1 既不会签出沙箱签名，
	// 也不会查到沙箱数据（见文件头）。
	Env int `json:"env"`
}

// validate 没有可查的字段：本请求体只有 env，而 env 由 checkEnv 查（0/1 之外都在本地
// 拦下）。留着这个方法是为了让本类七个接口的调用形状一样。
func (r QueryTransferAccountRequest) validate() error { return nil }

// QueryTransferAccountResponse 是查询广告金充值账户的响应体。
type QueryTransferAccountResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// AcctList 广告金充值账户列表。没绑定过就是空列表，不是错误。
	AcctList []TransferAccount `json:"acct_list"`
}

// QueryTransferAccount 查询广告金充值账户。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证。自研小程序传 GetStableAccessToken 换来的 access_token，第三方
//	             平台代商家调用传 authorizer_access_token——两者在这里是同一种东西。
//	req          只有 Env（本类不签名，所以没有 appKey 参数，见文件头）。
//
// 官方文档：POST /xpay/query_transfer_account
func QueryTransferAccount(ctx context.Context, accessToken string, req QueryTransferAccountRequest) (*QueryTransferAccountResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp QueryTransferAccountResponse
	if err := PostTokenOnly(ctx, accessToken, "/xpay/query_transfer_account", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 2/7  query_adver_funds —— 查询广告金发放记录
//
//	POST /xpay/query_adver_funds  access_token
// ---------------------------------------------------------------------------

// AdFundFilter 是查询广告金发放记录的过滤条件。
//
// 三个字段可以一个都不填：那表示「不筛」。**整体是 nil 与「填一个零值结构体」在这里
// 是同一件事**（都是三个字段全空），所以 QueryAdverFundsRequest.Filter 用指针只是为了让
// 「不传 filter」这个意图在类型上看得见，不影响发出去的内容。
type AdFundFilter struct {
	// SettleBegin 结算周期开始时间，unix 秒级时间戳。不填＝不按结算周期筛。
	SettleBegin int64 `json:"settle_begin,omitempty"`
	// SettleEnd 结算周期结束时间，unix 秒级时间戳。不填＝不按结算周期筛。
	SettleEnd int64 `json:"settle_end,omitempty"`
	// FundType 广告金发放原因。**指针**：用 nil 区分「不按类型筛」与「筛通用赠送」。
	//
	// ⚠️ 必须是指针。AdFundType 的零值是 AdFundTypeGeneral(0) 这个**有意义的取值**，
	// 换成值类型就再也表达不出「不筛类型」——两个意图会挤在同一个零值上，而 omitempty
	// 又会把 0 静默省掉，于是「筛通用赠送」变成「不筛」。
	FundType *AdFundType `json:"fund_type,omitempty"`
}

func (f AdFundFilter) validate() error {
	return checkUnixRange(f.SettleBegin, f.SettleEnd, "AdFundFilter 的结算周期", false)
}

// QueryAdverFundsRequest 是查询广告金发放记录的请求体。
type QueryAdverFundsRequest struct {
	// Page 查询页码，不小于 1。不填（0）表示不传这一项，由微信用默认值。
	Page int `json:"page,omitempty"`
	// PageSize 每页记录数量。不填（0）表示不传这一项，由微信用默认值。
	//
	// ⚠️ 官方没给最大值：本包**不设上限**——多大的页算太大是微信自己的事，本地编一个
	// 上限只会把某天官方放宽后的合法请求挡在门外。
	PageSize int `json:"page_size,omitempty"`
	// Filter 查询过滤条件。nil＝不筛。
	Filter *AdFundFilter `json:"filter,omitempty"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。本类不签名、也不切换数据源，见文件头。
	Env int `json:"env"`
}

func (r QueryAdverFundsRequest) validate() error {
	// 0 是「这一项不传」（omitempty 会把它省掉），所以只需要拦负数；正数就是官方说的
	// 「不小于 1」。
	if r.Page < 0 {
		return fmt.Errorf("wechat_virtualpay_go: Page %d 非法，官方要求不小于 1（不传请留 0）", r.Page)
	}
	if r.PageSize < 0 {
		return fmt.Errorf("wechat_virtualpay_go: PageSize %d 非法，每页条数不能为负（不传请留 0）", r.PageSize)
	}
	if r.Filter != nil {
		return r.Filter.validate()
	}
	return nil
}

// AdverFund 是一条广告金发放记录。
type AdverFund struct {
	// SettleBegin 结算周期开始时间，unix 秒级时间戳。
	SettleBegin int64 `json:"settle_begin"`
	// SettleEnd 结算周期结束时间，unix 秒级时间戳。
	SettleEnd int64 `json:"settle_end"`
	// TotalAmount 发放广告金金额，**单位分**。
	TotalAmount int64 `json:"total_amount"`
	// RemainAmount 剩余可用广告金金额，**单位分**。
	RemainAmount int64 `json:"remain_amount"`
	// ExpireTime 广告金过期时间，unix 秒级时间戳。
	ExpireTime int64 `json:"expire_time"`
	// FundType 广告金发放原因，取值见 AdFundType。
	FundType AdFundType `json:"fund_type"`
	// FundID 广告金发放 ID。它也是 create_funds_bill 与 download_adverfunds_order
	// 要传的那个 fund_id。
	FundID string `json:"fund_id"`
}

// QueryAdverFundsResponse 是查询广告金发放记录的响应体。
type QueryAdverFundsResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// AdverFundsList 广告金发放记录列表。
	AdverFundsList []AdverFund `json:"adver_funds_list"`
	// TotalPage 查询命中总的页数（用来翻页：Page 从 1 到 TotalPage）。
	TotalPage int `json:"total_page"`
}

// QueryAdverFunds 查询广告金发放记录。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryTransferAccount。
//	req          分页与过滤条件（可以不筛，全空即按默认分页拉）。
//
// 翻页靠响应里的 TotalPage；每条记录的 FundID 就是充值时要用的那个 id。
//
// 官方文档：POST /xpay/query_adver_funds
func QueryAdverFunds(ctx context.Context, accessToken string, req QueryAdverFundsRequest) (*QueryAdverFundsResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp QueryAdverFundsResponse
	if err := PostTokenOnly(ctx, accessToken, "/xpay/query_adver_funds", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 3/7  create_funds_bill —— 充值广告金
//
//	POST /xpay/create_funds_bill  access_token
// ---------------------------------------------------------------------------

// CreateFundsBillRequest 是充值广告金的请求体。
//
// ⚠️ 金额单位是**分**（transfer_amount），与资金类的「元」不同——见 xpay_funds.go 文件头。
type CreateFundsBillRequest struct {
	// TransferAmount 充值金额，**单位分**。
	TransferAmount int64 `json:"transfer_amount"`
	// TransferAccountUID 充值账户 uid，来自 QueryTransferAccount。
	TransferAccountUID int64 `json:"transfer_account_uid"`
	// TransferAccountName 充值账户名称，来自 QueryTransferAccount。
	TransferAccountName string `json:"transfer_account_name"`
	// TransferAccountAgencyID 充值账户服务商账号 id，来自 QueryTransferAccount。
	TransferAccountAgencyID int64 `json:"transfer_account_agency_id"`
	// RequestID 每一次请求的唯一 id（不超过 1024 字符）。
	//
	// **相同 id 的不同请求会被视为重复请求**——这就是本接口的幂等键：网络超时后拿同一个
	// RequestID 重试是安全的，换一个 id 重试就可能充两次。资金类那条 CreateWithdrawOrder
	// 靠 WithdrawNo 去重，这里靠它。
	RequestID string `json:"request_id"`
	// SettleBegin 充值所使用的广告金对应的结算周期开始时间，unix 秒级时间戳。
	SettleBegin int64 `json:"settle_begin"`
	// SettleEnd 充值所使用的广告金对应的结算周期结束时间，unix 秒级时间戳。
	SettleEnd int64 `json:"settle_end"`
	// AuthorizeAdvertise 是否授权广告数据：0 否，1 是。
	//
	// ⚠️ 这一栏本包**不校验取值**（与订单类的枚举同一条规矩：枚举的语义由微信定，
	// 本地编一个取值表就等于替官方冻结了协议）。
	AuthorizeAdvertise int `json:"authorize_advertise"`
	// FundType 广告金发放原因，取值见 AdFundType。同样不做本地取值校验。
	FundType AdFundType `json:"fund_type"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。本类不签名、也不切换数据源，见文件头。
	Env int `json:"env"`
}

func (r CreateFundsBillRequest) validate() error {
	if r.TransferAmount <= 0 {
		return fmt.Errorf("wechat_virtualpay_go: TransferAmount 必须大于 0（单位分）——充值一笔 0 或负数的广告金没有含义")
	}
	if r.TransferAccountUID == 0 {
		return fmt.Errorf("wechat_virtualpay_go: TransferAccountUID 不能为 0（必填，来自 QueryTransferAccount）")
	}
	if r.TransferAccountName == "" {
		return fmt.Errorf("wechat_virtualpay_go: TransferAccountName 不能为空（必填，来自 QueryTransferAccount）")
	}
	if r.TransferAccountAgencyID == 0 {
		return fmt.Errorf("wechat_virtualpay_go: TransferAccountAgencyID 不能为 0（必填，来自 QueryTransferAccount）")
	}
	if r.RequestID == "" {
		return fmt.Errorf("wechat_virtualpay_go: RequestID 不能为空——它是本接口的幂等键，空着等于放弃重试保护")
	}
	// 1024 这条是官方写的（「不超过 1024 字符」），不是推的。
	if len(r.RequestID) > 1024 {
		return fmt.Errorf("wechat_virtualpay_go: RequestID 超过 1024 字符（当前 %d），官方上限是 1024", len(r.RequestID))
	}
	if err := checkUnixRange(r.SettleBegin, r.SettleEnd, "充值对应的结算周期", true); err != nil {
		return err
	}
	// AuthorizeAdvertise / FundType 不做取值校验，理由见字段注释。
	return nil
}

// CreateFundsBillResponse 是充值广告金的响应体。
type CreateFundsBillResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// BillID 充值单 id。拿它调 QueryFundsBill 查这笔充值成没成。
	BillID string `json:"bill_id"`
}

// CreateFundsBill 充值广告金。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryTransferAccount。
//	req          充值账户三件套 + 金额（**分**）+ 幂等键 RequestID + 结算周期。
//
// ⚠️ 受理成功**不等于**充值到账：拿返回的 BillID 调 QueryFundsBill 查到
// FundsBillSuccess 才算成。
//
// 官方文档：POST /xpay/create_funds_bill
func CreateFundsBill(ctx context.Context, accessToken string, req CreateFundsBillRequest) (*CreateFundsBillResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp CreateFundsBillResponse
	if err := PostTokenOnly(ctx, accessToken, "/xpay/create_funds_bill", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 4/7  bind_transfer_accout —— 绑定广告金充值账户
//
//	POST /xpay/bind_transfer_accout  access_token
//
// ⚠️ 路径里的 **accout** 是微信官方的拼写（少一个 n），不是本包的笔误——照着
// "account" 去改会打到 404 上。
// ---------------------------------------------------------------------------

// BindTransferAccountRequest 是绑定广告金充值账户的请求体。
type BindTransferAccountRequest struct {
	// TransferAccountUID 充值账户 uid。
	TransferAccountUID int64 `json:"transfer_account_uid"`
	// TransferAccountOrgName 充值账户主体名称。
	TransferAccountOrgName string `json:"transfer_account_org_name"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。本类不签名、也不切换数据源，见文件头。
	Env int `json:"env"`
}

func (r BindTransferAccountRequest) validate() error {
	if r.TransferAccountUID == 0 {
		return fmt.Errorf("wechat_virtualpay_go: TransferAccountUID 不能为 0（绑定的就是它）")
	}
	if r.TransferAccountOrgName == "" {
		return fmt.Errorf("wechat_virtualpay_go: TransferAccountOrgName 不能为空（绑定的就是它）")
	}
	return nil
}

// BindTransferAccountResponse 是绑定广告金充值账户的响应体。
//
// 它没有自己的字段：微信只回公共头。仍然留着这个类型，因为**没有自己的字段不等于不会
// 失败**——绑定成没成、失败是哪种，都要有地方读 errcode（同 StartUploadGoodsResponse）。
type BindTransferAccountResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
}

// BindTransferAccount 绑定广告金充值账户。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryTransferAccount。
//	req          要绑的账户 uid 与主体名称。
//
// 绑成没成看 resp.ErrCode；绑完可以调 QueryTransferAccount 查审核状态与绑定结果。
//
// ⚠️ 两栏都按**必填**处理。依据是绑定这个动作的输入就是这两样，缺一个构不成一次绑定；
// 本包也没有独立核过官方这两栏的可选标记（广告金这条链路的文档在开发机上取不到）。若实测
// 官方确实允许只传其中一样，去掉那一条校验即可。
//
// ⚠️ 这是本类里**本包与旧实现不一致**的唯一一处：「`pre-rewrite` 分支上那份被删掉的实现」
// 给这两栏都挂了 `omitempty`，值空就不发（于是 uid=0 时会把一个空请求体发出去）。本包
// 改成必填，是**有意的收紧**——空 uid 的绑定请求没有任何意义，与其发出去换一个语焉不详的
// 参数错误，不如在本地拦下。
//
// 官方文档：POST /xpay/bind_transfer_accout（拼写见上面的 ⚠️）
func BindTransferAccount(ctx context.Context, accessToken string, req BindTransferAccountRequest) (*BindTransferAccountResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp BindTransferAccountResponse
	if err := PostTokenOnly(ctx, accessToken, "/xpay/bind_transfer_accout", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 5/7  query_funds_bill —— 查询广告金充值记录
//
//	POST /xpay/query_funds_bill  access_token
// ---------------------------------------------------------------------------

// FundsBillStatus 是广告金充值单状态。
type FundsBillStatus int

const (
	FundsBillProcessing FundsBillStatus = 0 // 充值中
	FundsBillSuccess    FundsBillStatus = 1 // 充值成功
	FundsBillFailed     FundsBillStatus = 2 // 充值失败
)

// FundsBillFilter 是查询广告金充值记录的过滤条件。
type FundsBillFilter struct {
	// OperTimeBegin 查询充值开始时间，unix 秒级时间戳（按充值时间筛，不是按结算周期）。
	OperTimeBegin int64 `json:"oper_time_begin"`
	// OperTimeEnd 查询充值结束时间，unix 秒级时间戳。
	OperTimeEnd int64 `json:"oper_time_end"`
	// BillID 广告金充值单 ID，可选。填了就是查这一单（create_funds_bill 返回的那个）。
	BillID string `json:"bill_id,omitempty"`
	// RequestID 调 CreateFundsBill 时传入的 request_id，可选。
	//
	// ⚠️ 它与 BillID 是**两条独立的路**：拿不到 BillID 时可以用自己的 RequestID 反查
	// （比如超时重试后想确认头一次到底成没成）。两个都填也能查到，但官方没说这时按哪个
	// 生效——要精确就一次只填一个。
	RequestID string `json:"request_id,omitempty"`
}

func (f FundsBillFilter) validate() error {
	return checkUnixRange(f.OperTimeBegin, f.OperTimeEnd, "FundsBillFilter 的充值时间", true)
}

// QueryFundsBillRequest 是查询广告金充值记录的请求体。
type QueryFundsBillRequest struct {
	// Page 查询页码，不小于 1。
	Page int `json:"page"`
	// PageSize 每页记录数量。
	PageSize int `json:"page_size"`
	// Filter 查询过滤条件。**值类型**（不是指针）：本接口的过滤条件是必填的，没有
	// 「不筛」这个选项——至少得给出要查哪段时间。
	Filter FundsBillFilter `json:"filter"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。本类不签名、也不切换数据源，见文件头。
	Env int `json:"env"`
}

func (r QueryFundsBillRequest) validate() error {
	if r.Page < 1 {
		return fmt.Errorf("wechat_virtualpay_go: Page %d 非法，官方要求不小于 1（必填）", r.Page)
	}
	if r.PageSize < 1 {
		return fmt.Errorf("wechat_virtualpay_go: PageSize %d 非法，每页条数要大于 0（必填）", r.PageSize)
	}
	return r.Filter.validate()
}

// FundsBill 是一条广告金充值记录。
type FundsBill struct {
	// BillID 充值单 ID。
	BillID string `json:"bill_id"`
	// OperTime 充值时间，unix 秒级时间戳。
	OperTime int64 `json:"oper_time"`
	// SettleBegin 结算周期开始时间，unix 秒级时间戳。
	SettleBegin int64 `json:"settle_begin"`
	// SettleEnd 结算周期结束时间，unix 秒级时间戳。
	SettleEnd int64 `json:"settle_end"`
	// FundID 对应广告金 ID。
	FundID string `json:"fund_id"`
	// TransferAccountName 充值账户。
	TransferAccountName string `json:"transfer_account_name"`
	// TransferAccountUID 充值账户 UID。
	TransferAccountUID int64 `json:"transfer_account_uid"`
	// TransferAmount 充值金额，**单位分**。
	TransferAmount int64 `json:"transfer_amount"`
	// Status 充值状态，取值见 FundsBillStatus。
	//
	// ⚠️ 零值是 FundsBillProcessing(0)「充值中」——查到一个 0 说明还没到终态，**不是**
	// 成功也不是失败，要接着轮询。
	Status FundsBillStatus `json:"status"`
	// RequestID 充值时的 request_id（就是 CreateFundsBill 传的那个）。
	RequestID string `json:"request_id"`
}

// QueryFundsBillResponse 是查询广告金充值记录的响应体。
type QueryFundsBillResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// BillList 广告金充值记录列表。
	BillList []FundsBill `json:"bill_list"`
	// TotalPage 查询命中总的页数。
	TotalPage int `json:"total_page"`
}

// QueryFundsBill 查询广告金充值记录。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryTransferAccount。
//	req          分页 + 充值记录过滤条件（充值时间区间必填，BillID/RequestID 可选）。
//
// 查一笔充值到底成没成，就是拿到 BillID 后在这里查 Status 到终态。
//
// 官方文档：POST /xpay/query_funds_bill
func QueryFundsBill(ctx context.Context, accessToken string, req QueryFundsBillRequest) (*QueryFundsBillResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp QueryFundsBillResponse
	if err := PostTokenOnly(ctx, accessToken, "/xpay/query_funds_bill", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 6/7  query_recover_bill —— 查询广告金回收记录
//
//	POST /xpay/query_recover_bill  access_token
// ---------------------------------------------------------------------------

// RecoverBillFilter 是查询广告金回收记录的过滤条件。
type RecoverBillFilter struct {
	// RecoverTimeBegin 查询回收开始时间，unix 秒级时间戳。
	RecoverTimeBegin int64 `json:"recover_time_begin"`
	// RecoverTimeEnd 查询回收结束时间，unix 秒级时间戳。
	RecoverTimeEnd int64 `json:"recover_time_end"`
	// BillID 广告金回收单 ID。
	//
	// ⚠️ 官方这一栏**自相矛盾**：字段表的「必填」列标着必填，说明文字里却写着「(可选)」。
	// 本包按**必填**处理——理由是这一条与上面那两栏不同：时间区间已经能筛出一批记录，
	// 而「没有时间区间就查不了」的接口在别处也有先例；更实际的一层是，本栏没有 omitempty
	// （永远会被发出去），若它真可选，漏填时微信收到的就是一个空串，不如在本地就说清。
	// 若实测官方允许空着查全部，去掉这条校验并给它加 omitempty。
	BillID string `json:"bill_id"`
}

func (f RecoverBillFilter) validate() error {
	if f.BillID == "" {
		return fmt.Errorf("wechat_virtualpay_go: RecoverBillFilter.BillID 不能为空（官方字段表标必填，说明文字却写「可选」——本包按必填处理，见字段注释）")
	}
	return checkUnixRange(f.RecoverTimeBegin, f.RecoverTimeEnd, "RecoverBillFilter 的回收时间", true)
}

// QueryRecoverBillRequest 是查询广告金回收记录的请求体。
type QueryRecoverBillRequest struct {
	// Page 查询页码，不小于 1。
	Page int `json:"page"`
	// PageSize 每页记录数量。
	PageSize int `json:"page_size"`
	// Filter 查询过滤条件。**值类型**：同 QueryFundsBillRequest，过滤条件必填。
	Filter RecoverBillFilter `json:"filter"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。本类不签名、也不切换数据源，见文件头。
	Env int `json:"env"`
}

func (r QueryRecoverBillRequest) validate() error {
	if r.Page < 1 {
		return fmt.Errorf("wechat_virtualpay_go: Page %d 非法，官方要求不小于 1（必填）", r.Page)
	}
	if r.PageSize < 1 {
		return fmt.Errorf("wechat_virtualpay_go: PageSize %d 非法，每页条数要大于 0（必填）", r.PageSize)
	}
	return r.Filter.validate()
}

// RecoverBill 是一条广告金回收记录。
//
// 回收（recover）与充值（funds_bill）方向相反：退款发生时，之前发放的广告金会被按比例
// 收回，这就是回收记录的来源。
type RecoverBill struct {
	// BillID 回收单 ID。
	BillID string `json:"bill_id"`
	// RecoverTime 回收时间，unix 秒级时间戳。
	RecoverTime int64 `json:"recover_time"`
	// SettleBegin 结算周期开始时间，unix 秒级时间戳。
	SettleBegin int64 `json:"settle_begin"`
	// SettleEnd 结算周期结束时间，unix 秒级时间戳。
	SettleEnd int64 `json:"settle_end"`
	// FundID 对应的发放广告金 ID。
	FundID string `json:"fund_id"`
	// RecoverAccountName 回收广告金账户。
	RecoverAccountName string `json:"recover_account_name"`
	// RecoverAmount 回收金额，**单位分**。
	RecoverAmount int64 `json:"recover_amount"`
	// RefundOrderList 对应的退款订单 id 列表。
	//
	// ⚠️ 它是**数组**：一次回收通常对应一笔退款，但官方没有承诺一对一，别按「取第一个」
	// 去写——按笔对账要把列表整个走一遍。
	RefundOrderList []string `json:"refund_order_list"`
}

// QueryRecoverBillResponse 是查询广告金回收记录的响应体。
type QueryRecoverBillResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// BillList 广告金回收记录列表。
	BillList []RecoverBill `json:"bill_list"`
	// TotalPage 查询命中总的页数。
	TotalPage int `json:"total_page"`
}

// QueryRecoverBill 查询广告金回收记录。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryTransferAccount。
//	req          分页 + 回收记录过滤条件（时间区间与 BillID 都必填，见 RecoverBillFilter）。
//
// 官方文档：POST /xpay/query_recover_bill
func QueryRecoverBill(ctx context.Context, accessToken string, req QueryRecoverBillRequest) (*QueryRecoverBillResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp QueryRecoverBillResponse
	if err := PostTokenOnly(ctx, accessToken, "/xpay/query_recover_bill", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 7/7  download_adverfunds_order —— 下载广告金对应的商户订单信息
//
//	POST /xpay/download_adverfunds_order  access_token
// ---------------------------------------------------------------------------

// DownloadAdverFundsOrderRequest 是下载广告金对应商户订单信息的请求体。
type DownloadAdverFundsOrderRequest struct {
	// FundID 广告金发放 ID，来自 QueryAdverFunds 里某条记录的 FundID。
	FundID string `json:"fund_id"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。本类不签名、也不切换数据源，见文件头。
	Env int `json:"env"`
}

func (r DownloadAdverFundsOrderRequest) validate() error {
	if r.FundID == "" {
		return fmt.Errorf("wechat_virtualpay_go: FundID 不能为空（来自 QueryAdverFunds 的 FundID）")
	}
	return nil
}

// DownloadAdverFundsOrderResponse 是下载广告金对应商户订单信息的响应体。
type DownloadAdverFundsOrderResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// URL 订单下载链接。
	//
	// ⚠️ 第一次调用**很可能拿到空串**：那一趟只是触发生成，链接还没做出来。空串不是错误
	// （errcode 仍是 0），要隔一会儿再调一次（见下面的调用说明）。
	URL string `json:"url"`
}

// DownloadAdverFundsOrder 下载广告金对应的商户订单信息。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryTransferAccount。
//	req          广告金发放 ID（来自 QueryAdverFunds）。
//
// ⚠️ 官方「注意事项」有两条，本方法不会替你处理，得调用方自己记着：
//
//  1. **只支持通用赠送广告金**（FundType = AdFundTypeGeneral，即 fund_type=0）对应订单的
//     下载；广告激励/定向激励那两种拿不到。
//  2. **第一次调用只触发生成下载 url**，返回的 URL 可能尚未生成——要间隔轮询再调，
//     直到拿到非空的 URL 为止。判据是**URL 非空**，不是 errcode（空 URL 时 errcode 为 0，
//     所以「err == nil」在这里只说明这一趟走通了）。
//
// 官方文档：POST /xpay/download_adverfunds_order
func DownloadAdverFundsOrder(ctx context.Context, accessToken string, req DownloadAdverFundsOrderRequest) (*DownloadAdverFundsOrderResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp DownloadAdverFundsOrderResponse
	if err := PostTokenOnly(ctx, accessToken, "/xpay/download_adverfunds_order", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
