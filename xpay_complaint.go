package wechat_virtualpay

import (
	"context"
	"fmt"
	"time"
)

// 本文件是官方 /xpay/* 里「投诉」这一类 8 个接口：
//
//	POST /xpay/get_complaint_list        获取投诉列表        access_token + pay_sig
//	POST /xpay/get_complaint_detail      获取投诉详情        access_token + pay_sig
//	POST /xpay/get_negotiation_history   获取协商历史        access_token + pay_sig
//	POST /xpay/response_complaint        回复用户投诉        access_token + pay_sig
//	POST /xpay/complete_complaint        完成投诉处理        access_token + pay_sig
//	POST /xpay/upload_vp_file            上传媒体文件        access_token + pay_sig
//	POST /xpay/get_upload_file_sign      取投诉图片签名头部  access_token + pay_sig
//	POST /xpay/query_punishment_reasons  查商户被管控原因    access_token + pay_sig
//
// 八个都是 pay_sig 档。它们是一条链上的两种活：
//
//   - **处理投诉**：get_complaint_list 拉一批 → get_complaint_detail 看单条 →
//     get_negotiation_history 看往来记录 → response_complaint 回复 → complete_complaint 结单。
//   - **搬图片**（投诉的凭证都是图片，两头都要过微信）：reply 时要附的图先用 upload_vp_file
//     换成 file_id；投诉里**用户传的图**托管在微信支付侧、直接下会被拒，要先拿
//     get_upload_file_sign 换出 Authorization 头再自己发 HTTP 去下。
//
// ⚠️ **官方文档在这几页上有一处老毛病**：get_complaint_list 的「注意事项」写「使用用户态
// 签名与支付签名」，但它的参数表只列了 pay_sig。这与 refund_order 那页是同一处文档问题
// （见 xpay_order.go 文件头的同类说明）——本包按**参数表**实现（pay_sig）。真机若报
// 268490003 再考虑升档，但按参数表走的理由更硬：签名参数不是可选装饰，漏一个必然报错，
// 而这张表是逐项列出来的。
//
// ⚠️ 本类有一处**全包唯一**的调用形状：query_punishment_reasons 没有请求参数，所以它的
// 函数**不收 req**（见 8/8 那段）。别的接口都能在签名的请求体里带上 env，这一个连请求体
// 都没有（官方写「请求体：无」）——签名仍然是对**发出去的那份字节**算的，本包发 `{}`，
// 详见那段注释。
//
// 本类**故意不做**的本地校验（都写在对应字段的注释里，这里汇总）：
//
//   - 不查 upload_vp_file 的图片体积（官方给了 1M / 2M 两个上限，但没说清是按原始字节还是
//     按 base64 后长度，也没说 1M 是 1<<20 还是 1e6）。本包不发明度量衡。
//   - 不查 base64 是否合法、也不剥离 "data:image/png;base64," 前缀——剥错了比不剥更糟。
//   - 不查 get_upload_file_sign 里 WxpayURL 的域名与路径前缀（那是微信侧生成的地址，
//     本包按前缀去卡，等于把将来换域名/换版本号的合法地址挡在门外）。
//   - 不查手机号、投诉内容等的长度与字符集（官方没给规则）。
//
// 其余约定与订单类**逐字同义**，不在这里重抄（见 xpay_order.go 文件头）：凭据显式传参、
// env 是请求结构体上的一个裸 int（0=现网 / 1=沙箱）、字段顺序照官方字段表逐行抄、
// 响应**原值返回**、`err == nil` 不等于成功（成败看 resp.ErrCode）、失败时响应为 nil。
//
// 文件按接口分段，每段是「枚举 → 请求结构体 → 本地校验 → 响应结构体 → 调用函数」，
// 读一个接口只需要看一段；本类共用的数据模型与校验放在最前面。

// ComplaintState 是投诉状态。
type ComplaintState string

const (
	ComplaintStatePending    ComplaintState = "PENDING"    // 待处理
	ComplaintStateProcessing ComplaintState = "PROCESSING" // 处理中
	ComplaintStateProcessed  ComplaintState = "PROCESSED"  // 已处理完成
)

// ProblemType 是用户投诉的问题类型。
type ProblemType string

const (
	// ProblemTypeRefund 申请退款。这类单据需要**最高优先处理**。
	ProblemTypeRefund ProblemType = "REFUND"
	// ProblemTypeServiceNotWork 服务权益未生效。
	ProblemTypeServiceNotWork ProblemType = "SERVICE_NOT_WORK"
	// ProblemTypeOthers 其他类型。
	ProblemTypeOthers ProblemType = "OTHERS"
)

// UserTag 是投诉人标签。
type UserTag string

const (
	// UserTagTrusted 该用户满足极速退款条件。
	UserTagTrusted UserTag = "TRUSTED"
	// UserTagHighRisk 高风险投诉，需按运营要求优先妥善处理。
	UserTagHighRisk UserTag = "HIGH_RISK"
)

// ComplaintMediaType 是投诉相关媒体的业务类型。
type ComplaintMediaType string

const (
	ComplaintMediaUserImage      ComplaintMediaType = "USER_COMPLAINT_IMAGE" // 用户提交投诉时上传的图片凭证
	ComplaintMediaOperationImage ComplaintMediaType = "OPERATION_IMAGE"      // 协商解决投诉时上传的图片凭证
)

// ServiceOrderState 是服务订单状态。
//
// ⚠️ 投诉单里记的是**用户发起投诉那一刻的快照**，不会随服务单的真实进展更新——别拿它
// 判断服务单现在的状态。
type ServiceOrderState string

const (
	ServiceOrderDoing   ServiceOrderState = "DOING"   // 服务订单进行中
	ServiceOrderRevoked ServiceOrderState = "REVOKED" // 服务订单已取消
	ServiceOrderWaitPay ServiceOrderState = "WAITPAY" // 服务订单待支付
	ServiceOrderDone    ServiceOrderState = "DONE"    // 服务订单已完成
)

// ComplaintOrderInfo 是投诉单关联的订单信息。
type ComplaintOrderInfo struct {
	// TransactionID 微信支付交易单号。
	TransactionID string `json:"transaction_id"`
	// OutTradeNo 渠道单号，即 QueryOrder 返回的 channel_order_id。
	OutTradeNo string `json:"out_trade_no"`
	// Amount 订单金额，**单位分**。
	Amount int64 `json:"amount"`
	// WxaOutTradeNo 商户单号，商家在拉起支付时传的单号（就是本包下单时的 OutTradeNo）。
	WxaOutTradeNo string `json:"wxa_out_trade_no"`
	// WxOrderID 小程序侧单号。
	WxOrderID string `json:"wx_order_id"`
}

// ComplaintMedia 是一条投诉相关资料。
//
// ⚠️ MediaURL 是**列表**，而且里面的地址在微信支付侧、直接下载会被拒——要先用
// GetUploadFileSign 换出 Authorization 头（见那个函数的说明）。
type ComplaintMedia struct {
	// MediaType 媒体文件对应的业务类型，取值见 ComplaintMediaType。
	MediaType ComplaintMediaType `json:"media_type"`
	// MediaURL 媒体文件请求 URL。
	MediaURL []string `json:"media_url"`
}

// ServiceOrderInfo 是投诉单关联的服务单信息。
type ServiceOrderInfo struct {
	// OrderID 微信支付服务订单号。
	OrderID string `json:"order_id"`
	// OutOrderNo 商户系统内部服务订单号（**不是**交易单号）。
	OutOrderNo string `json:"out_order_no"`
	// State 用户发起投诉那一刻的服务单状态快照，不会实时更新（见 ServiceOrderState）。
	State ServiceOrderState `json:"state"`
}

// Complaint 是一条用户投诉。
//
// get_complaint_list 的列表项与 get_complaint_detail 的详情用的是同一个类型——官方两处的
// 字段表本来就一致。
type Complaint struct {
	// ComplaintID 投诉 ID。本类后续接口（详情/协商历史/回复/结单）全靠它。
	ComplaintID string `json:"complaint_id"`
	// ComplaintTime 投诉时间，格式 yyyy-mm-dd'T'HH:MM:ssXXX。
	ComplaintTime string `json:"complaint_time"`
	// ComplaintDetail 投诉内容。
	ComplaintDetail string `json:"complaint_detail"`
	// ComplaintState 投诉状态，取值见 ComplaintState。
	ComplaintState ComplaintState `json:"complaint_state"`
	// PayerPhone 投诉人联系方式。
	PayerPhone string `json:"payer_phone"`
	// PayerOpenID 投诉人在商户 AppID 下的唯一标识。
	PayerOpenID string `json:"payer_openid"`
	// ComplaintOrderInfo 投诉单关联的订单信息（**列表**：一次投诉可能涉及多笔订单）。
	ComplaintOrderInfo []ComplaintOrderInfo `json:"complaint_order_info"`
	// ComplaintFullRefunded 投诉单下所有订单是否已全部全额退款。
	ComplaintFullRefunded bool `json:"complaint_full_refunded"`
	// IncomingUserResponse 是否有待回复的用户留言。
	//
	// ⚠️ 这是**该不该回**的信号：为 true 说明用户还在等回复，此时直接结单（CompleteComplaint）
	// 通常不会被接受。
	IncomingUserResponse bool `json:"incoming_user_response"`
	// UserComplaintTimes 用户投诉次数：首次记为 1，每继续投诉一次加 1。
	UserComplaintTimes int `json:"user_complaint_times"`
	// ComplaintMediaList 用户上传的投诉相关资料（图片凭证等）。
	ComplaintMediaList []ComplaintMedia `json:"complaint_media_list"`
	// ProblemDescription 用户发起投诉前选择的 FAQ 标题。
	ProblemDescription string `json:"problem_description"`
	// ProblemType 问题类型。ProblemTypeRefund（申请退款）需最高优先处理。
	ProblemType ProblemType `json:"problem_type"`
	// ApplyRefundAmount 当问题类型为申请退款时有值，**单位分**。
	ApplyRefundAmount int64 `json:"apply_refund_amount"`
	// UserTagList 用户标签列表，取值见 UserTag。
	UserTagList []UserTag `json:"user_tag_list"`
	// ServiceOrderInfo 投诉单关联的服务单信息（**列表**）。
	ServiceOrderInfo []ServiceOrderInfo `json:"service_order_info"`
}

// checkDay10Range 校验一对 "yyyy-mm-dd" 日期（官方给的格式，如 "2023-01-01"）。
//
// 只解析，不另做「回写比对」。理由是一条实测过的性质：time.Parse 配 "2006-01-02" 这个
// layout 是**只认这个形状**的——月和日都是定宽两位取的，"2023-1-1" / "2023-01-1" /
// "2023-1-01" / "2023-001-01" 一律直接报 parse 错，"2023-02-30" / "2023-01-32" 报
// out of range，"2023-01-01 " 报 extra text。既然 Parse 收下的串回写一定还是原串，
// 「回写比对」那一步就是不可达的死分支（原先写了它，变异测试里把它删掉测试照样全绿，
// 回头核才确认了上面这条性质），所以删掉——形状由 TestComplaintValidation 的三条日期
// 用例钉住；哪天 Go 把 Parse 放宽了，那三条会先红，届时再把回写比对加回来。
//
// 它留在本文件而不是 xpay_common.go：只有本类的请求体里有这种格式的日期（账单/订单那两类
// 用的是 int64 的 YYYYMMDD 与 yyyyMM 两种别的写法，各自有各自的助手）。
func checkDay10Range(begin, end string) error {
	b, err := time.Parse("2006-01-02", begin)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay: BeginDate %q 不是合法的 yyyy-mm-dd 日期（要 2023-01-01 这样的两位写法）", begin)
	}
	e, err := time.Parse("2006-01-02", end)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay: EndDate %q 不是合法的 yyyy-mm-dd 日期（要 2023-01-01 这样的两位写法）", end)
	}
	if e.Before(b) {
		return fmt.Errorf("wechat_virtualpay: EndDate(%s) 早于 BeginDate(%s)", end, begin)
	}
	return nil
}

// checkLimitOffset 校验分页参数（本类两个列表接口共用）。
//
// Offset 按官方写的「从 0 开始」处理，所以 0 合法、负数不合法。Limit 官方只写了「最多返回
// 条数」，没说 0 是「不要数据」还是「用默认值」；本包按「查询至少要看一条」处理，0 与负数
// 都在本地拦——若实测官方把 0 当默认值，去掉这一条即可。
func checkLimitOffset(offset, limit int) error {
	if offset < 0 {
		return fmt.Errorf("wechat_virtualpay: Offset %d 非法，官方写明从 0 开始", offset)
	}
	if limit < 1 {
		return fmt.Errorf("wechat_virtualpay: Limit %d 非法，官方写的是「最多返回条数」（至少 1 条）", limit)
	}
	return nil
}

// checkComplaintID 校验投诉 ID 非空（本类四个接口共用）。
//
// 只查非空：官方没给这个 ID 的字符集与长度（它是微信生成的），本包不发明格式。
func checkComplaintID(id string) error {
	if id == "" {
		return fmt.Errorf("wechat_virtualpay: ComplaintID 不能为空（来自 GetComplaintList/GetComplaintDetail）")
	}
	return nil
}

// ---------------------------------------------------------------------------
// 1/8  get_complaint_list —— 获取投诉列表
//
//	POST /xpay/get_complaint_list  access_token + pay_sig
// ---------------------------------------------------------------------------

// GetComplaintListRequest 是获取投诉列表的请求体。
type GetComplaintListRequest struct {
	// BeginDate 筛选开始时间，格式 **yyyy-mm-dd**，如 "2023-01-01"。
	BeginDate string `json:"begin_date"`
	// EndDate 筛选结束时间，格式 **yyyy-mm-dd**。
	EndDate string `json:"end_date"`
	// Offset 筛选偏移，从 0 开始（0 就是第一页）。
	Offset int `json:"offset"`
	// Limit 最多返回条数。
	Limit int `json:"limit"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

func (r GetComplaintListRequest) validate() error {
	if err := checkDay10Range(r.BeginDate, r.EndDate); err != nil {
		return err
	}
	return checkLimitOffset(r.Offset, r.Limit)
}

// GetComplaintListResponse 是获取投诉列表的响应体。
type GetComplaintListResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// Total 总条数。翻页靠它（Offset 递增 Limit 条，直到取满）。
	Total int `json:"total"`
	// Complaints 投诉列表。这一天没投诉就是空列表，不是错误。
	Complaints []Complaint `json:"complaints"`
}

// GetComplaintList 获取投诉列表。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证。自研小程序传 GetStableAccessToken 换来的 access_token，第三方
//	             平台代商家调用传 authorizer_access_token——两者在这里是同一种东西。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          日期区间（yyyy-mm-dd）与分页。
//
// ⚠️ 官方这一页的「注意事项」写着用「用户态签名与支付签名」，但它自己的参数表只列了
// pay_sig——本包按参数表实现（见文件头）。「`pre-rewrite` 分支上那份被删掉的实现」本接口
// 同样是 `callPaySig`，且在同一处记了同一句存疑——两次读同一张表，结论一致。真机若回
// 268490003，本函数的签名要跟着变（多收一个 sessionKey），这是本类唯一一处**押错就得改
// 函数签名**的地方。
//
// 官方文档：POST /xpay/get_complaint_list
func GetComplaintList(ctx context.Context, accessToken, appKey string, req GetComplaintListRequest) (*GetComplaintListResponse, error) {
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

	var resp GetComplaintListResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/get_complaint_list", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 2/8  get_complaint_detail —— 获取投诉详情
//
//	POST /xpay/get_complaint_detail  access_token + pay_sig
// ---------------------------------------------------------------------------

// GetComplaintDetailRequest 是获取投诉详情的请求体。
type GetComplaintDetailRequest struct {
	// ComplaintID 投诉 ID，由 GetComplaintList 返回。
	ComplaintID string `json:"complaint_id"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。见 GetComplaintListRequest 的同名字段。
	Env int `json:"env"`
}

func (r GetComplaintDetailRequest) validate() error {
	return checkComplaintID(r.ComplaintID)
}

// GetComplaintDetailResponse 是获取投诉详情的响应体。
type GetComplaintDetailResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// Complaint 投诉详情，字段与 GetComplaintList 的列表项一致；查不到时为 nil。
	//
	// ⚠️ 是**指针**：errcode=0 但这一栏为空是可能的（比如这条投诉已经不可见），
	// 所以它有一个「没有」的状态，值类型表达不出来。判空再读字段。
	Complaint *Complaint `json:"complaint"`
}

// GetComplaintDetail 获取一条投诉的详情。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 GetComplaintList。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**。
//	req          投诉 ID（来自 GetComplaintList）。
//
// 官方文档：POST /xpay/get_complaint_detail
func GetComplaintDetail(ctx context.Context, accessToken, appKey string, req GetComplaintDetailRequest) (*GetComplaintDetailResponse, error) {
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

	var resp GetComplaintDetailResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/get_complaint_detail", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 3/8  get_negotiation_history —— 获取投诉的协商历史
//
//	POST /xpay/get_negotiation_history  access_token + pay_sig
// ---------------------------------------------------------------------------

// OperateType 是投诉协商记录的操作类型。
//
// ⚠️ 有两个取值是**微信官方的拼写**，不是本包抄错：
//
//	OpUserConfirmComplaint = "USER_COMFIRM_COMPLAINT"      少一个 R（confirm 拼成 comfirm）
//	OpMerchantRefuseRefund = "MERCHANT_REFUSE_RERUND"      少一个 F（refund 拼成 rerund）
//
// 比对时必须用这两个常量，照正确拼写去写字符串会永远匹配不上。
type OperateType string

const (
	OpUserCreateComplaint       OperateType = "USER_CREATE_COMPLAINT"                     // 用户提交投诉
	OpUserContinueComplaint     OperateType = "USER_CONTINUE_COMPLAINT"                   // 用户继续投诉
	OpUserResponse              OperateType = "USER_RESPONSE"                             // 用户留言
	OpPlatformResponse          OperateType = "PLATFORM_RESPONSE"                         // 平台留言
	OpMerchantResponse          OperateType = "MERCHANT_RESPONSE"                         // 商户留言
	OpMerchantConfirmComplete   OperateType = "MERCHANT_CONFIRM_COMPLETE"                 // 商户申请结单
	OpUserCreateComplaintSystem OperateType = "USER_CREATE_COMPLAINT_SYSTEM_MESSAGE"      // 用户提交投诉系统通知
	OpFullRefundedSystem        OperateType = "COMPLAINT_FULL_REFUNDED_SYSTEM_MESSAGE"    // 投诉单发起全额退款系统通知
	OpPartialRefundedSystem     OperateType = "COMPLAINT_PARTIAL_REFUNDED_SYSTEM_MESSAGE" // 投诉单发起部分退款系统通知
	OpRefundReceivedSystem      OperateType = "COMPLAINT_REFUND_RECEIVED_SYSTEM_MESSAGE"  // 投诉单退款到账系统通知
	OpUserContinueSystem        OperateType = "USER_CONTINUE_COMPLAINT_SYSTEM_MESSAGE"    // 用户继续投诉系统通知
	OpUserRevokeComplaint       OperateType = "USER_REVOKE_COMPLAINT"                     // 用户主动撤诉（仅历史投诉单）
	OpUserConfirmComplaint      OperateType = "USER_COMFIRM_COMPLAINT"                    // 用户确认投诉解决（仅历史投诉单，微信原文如此拼写）
	OpPlatformHelpApplication   OperateType = "PLATFORM_HELP_APPLICATION"                 // 平台催办
	OpUserApplyPlatformHelp     OperateType = "USER_APPLY_PLATFORM_HELP"                  // 用户申请平台协助
	OpMerchantApproveRefund     OperateType = "MERCHANT_APPROVE_REFUND"                   // 商户同意退款申请
	OpMerchantRefuseRefund      OperateType = "MERCHANT_REFUSE_RERUND"                    // 商户拒绝退款申请（微信原文如此拼写）
	OpUserSubmitSatisfaction    OperateType = "USER_SUBMIT_SATISFACTION"                  // 用户提交满意度调查结果
	OpServiceOrderCancel        OperateType = "SERVICE_ORDER_CANCEL"                      // 服务订单已取消
	OpServiceOrderComplete      OperateType = "SERVICE_ORDER_COMPLETE"                    // 服务订单已完成
)

// NegotiationEntry 是一条投诉协商记录。
type NegotiationEntry struct {
	// LogID 操作流水号。
	LogID string `json:"log_id"`
	// Operator 操作人。
	Operator string `json:"operator"`
	// OperateTime 操作时间，格式 yyyy-mm-dd'T'HH:MM:ssXXX。
	OperateTime string `json:"operate_time"`
	// OperateType 操作类型，取值见 OperateType（注意里面有两个官方拼写错误的值）。
	OperateType OperateType `json:"operate_type"`
	// OperateDetails 该条记录的具体内容。用户/商户留言的正文就在这里。
	OperateDetails string `json:"operate_details"`
	// ComplaintMediaList 执行操作时上传的资料凭证。
	ComplaintMediaList []ComplaintMedia `json:"complaint_media_list"`
}

// GetNegotiationHistoryRequest 是获取协商历史的请求体。
type GetNegotiationHistoryRequest struct {
	// ComplaintID 投诉 ID，由 GetComplaintList 返回。
	ComplaintID string `json:"complaint_id"`
	// Offset 筛选偏移，从 0 开始。
	Offset int `json:"offset"`
	// Limit 最多返回条数。
	Limit int `json:"limit"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。见 GetComplaintListRequest 的同名字段。
	Env int `json:"env"`
}

func (r GetNegotiationHistoryRequest) validate() error {
	if err := checkComplaintID(r.ComplaintID); err != nil {
		return err
	}
	return checkLimitOffset(r.Offset, r.Limit)
}

// GetNegotiationHistoryResponse 是获取协商历史的响应体。
type GetNegotiationHistoryResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// Total 总条数。
	Total int `json:"total"`
	// History 协商历史（按时间先后）。
	History []NegotiationEntry `json:"history"`
}

// GetNegotiationHistory 获取一条投诉的协商历史。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 GetComplaintList。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**。
//	req          投诉 ID 与分页。
//
// 回复之前先看这里：用户最近说了什么、之前商家回过什么，都在 History 里（留言正文在
// OperateDetails，操作方在 Operator/OperateType）。
//
// 官方文档：POST /xpay/get_negotiation_history
func GetNegotiationHistory(ctx context.Context, accessToken, appKey string, req GetNegotiationHistoryRequest) (*GetNegotiationHistoryResponse, error) {
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

	var resp GetNegotiationHistoryResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/get_negotiation_history", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 4/8  response_complaint —— 回复用户投诉
//
//	POST /xpay/response_complaint  access_token + pay_sig
// ---------------------------------------------------------------------------

// ResponseComplaintRequest 是回复用户的请求体。
type ResponseComplaintRequest struct {
	// ComplaintID 投诉 ID。
	ComplaintID string `json:"complaint_id"`
	// ResponseContent 回复内容（文字）。
	//
	// 带 omitempty：它与 ResponseImages 是**二选一**（见 validate），所以它是**可选**的
	// ——本包的规矩是可选字段都带。只回图片时不该把这一栏发出去：发了就是
	// `"response_content":""`，把一个「没填」写成一个**出现了的空串**。万一微信对「出现
	// 但为空」的回复内容报参数错误，只回图片这条路就会稳定失败，而错误信息看不出原因；
	// 「不出现」则永远不可能是非法的。
	ResponseContent string `json:"response_content,omitempty"`
	// ResponseImages 回复的图片，每一项是 UploadVPFile 返回的 FileID。
	//
	// ⚠️ 这里要的是 **FileID**（本包接口换来的），不是图片 URL——用户端看到的是那张图。
	// 传 URL 会被当成无效 file_id。
	//
	// 同样带 omitempty，理由同上（只回文字时 nil 会序列化成 "response_images":null）。
	ResponseImages []string `json:"response_images,omitempty"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。见 GetComplaintListRequest 的同名字段。
	Env int `json:"env"`
}

func (r ResponseComplaintRequest) validate() error {
	if err := checkComplaintID(r.ComplaintID); err != nil {
		return err
	}
	// 文字与图片**至少有一样**：两样都空的「回复」在用户端什么也看不到，等于没回。
	// （这条是从语义推的，官方没写「二者不可同时为空」。）
	if r.ResponseContent == "" && len(r.ResponseImages) == 0 {
		return fmt.Errorf("wechat_virtualpay: ResponseContent 与 ResponseImages 至少要有一个——空的回复在用户端看不到任何东西")
	}
	return nil
}

// ResponseComplaintResponse 是回复用户的响应体。
//
// 它没有自己的字段：微信只回公共头。仍然留着这个类型，因为**没有自己的字段不等于不会
// 失败**——回没回上只能从 errcode 读（同 StartUploadGoodsResponse）。
type ResponseComplaintResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
}

// ResponseComplaint 回复用户投诉。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 GetComplaintList。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**。
//	req          投诉 ID + 回复内容（文字与图片至少有一样）。
//
// 要附图片的话，先用 UploadVPFile 把图换成 FileID，再放进 ResponseImages。
//
// 官方文档：POST /xpay/response_complaint
func ResponseComplaint(ctx context.Context, accessToken, appKey string, req ResponseComplaintRequest) (*ResponseComplaintResponse, error) {
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

	var resp ResponseComplaintResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/response_complaint", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 5/8  complete_complaint —— 完成投诉处理（申请结单）
//
//	POST /xpay/complete_complaint  access_token + pay_sig
// ---------------------------------------------------------------------------

// CompleteComplaintRequest 是完成投诉处理的请求体。
type CompleteComplaintRequest struct {
	// ComplaintID 投诉 ID。
	ComplaintID string `json:"complaint_id"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。见 GetComplaintListRequest 的同名字段。
	Env int `json:"env"`
}

func (r CompleteComplaintRequest) validate() error {
	return checkComplaintID(r.ComplaintID)
}

// CompleteComplaintResponse 是完成投诉处理的响应体。
//
// 它没有自己的字段：微信只回公共头（同 ResponseComplaintResponse）。
type CompleteComplaintResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
}

// CompleteComplaint 完成投诉处理（向微信申请结单）。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 GetComplaintList。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**。
//	req          投诉 ID。
//
// ⚠️ 结单之前先看 Complaint.IncomingUserResponse：为 true 说明用户还在等回复，此时直接
// 结单通常不会被接受（微信会回错误码）——先把话回完（ResponseComplaint）再结。
//
// 官方文档：POST /xpay/complete_complaint
func CompleteComplaint(ctx context.Context, accessToken, appKey string, req CompleteComplaintRequest) (*CompleteComplaintResponse, error) {
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

	var resp CompleteComplaintResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/complete_complaint", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 6/8  upload_vp_file —— 上传媒体文件
//
//	POST /xpay/upload_vp_file  access_token + pay_sig
// ---------------------------------------------------------------------------

// UploadVPFileRequest 是上传媒体文件的请求体。
//
// **Base64Img 与 ImgURL 二选一，且 ImgURL 优先**（官方明写）。两个都填不会报错，但生效的
// 是 ImgURL——另一个是白传的（本包不拦这种「浪费」，只拦两个都空）。
type UploadVPFileRequest struct {
	// Base64Img 经 base64 编码后的图片内容，最多 1M。
	//
	// ⚠️ 体积**不校验**：官方没说这 1M 是按原始字节还是 base64 后的长度，也没说 1M 是
	// 1<<20 还是 1e6——本包不发明度量衡（见文件头的「故意不做」）。
	Base64Img string `json:"base64_img,omitempty"`
	// ImgURL 图片 URL，需能直接下载（不能是 302 跳转之类的），最高 2M。**优先使用本字段**。
	ImgURL string `json:"img_url,omitempty"`
	// FileName 图片名称。
	FileName string `json:"file_name"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。见 GetComplaintListRequest 的同名字段。
	Env int `json:"env"`
}

func (r UploadVPFileRequest) validate() error {
	if r.Base64Img == "" && r.ImgURL == "" {
		return fmt.Errorf("wechat_virtualpay: Base64Img 与 ImgURL 至少要有一个（官方：二选一，ImgURL 优先）")
	}
	if r.FileName == "" {
		return fmt.Errorf("wechat_virtualpay: FileName 不能为空")
	}
	return nil
}

// UploadVPFileResponse 是上传媒体文件的响应体。
type UploadVPFileResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// FileID 返回的文件 ID，用于 ResponseComplaint 的 ResponseImages。
	FileID string `json:"file_id"`
}

// UploadVPFile 上传媒体文件（图片、凭证等），用于回复投诉。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 GetComplaintList。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**。
//	req          图片（Base64Img 或 ImgURL，后者优先）+ 文件名。
//
// 拿到的 FileID 放进 ResponseComplaint 的 ResponseImages。
//
// 官方文档：POST /xpay/upload_vp_file
func UploadVPFile(ctx context.Context, accessToken, appKey string, req UploadVPFileRequest) (*UploadVPFileResponse, error) {
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

	var resp UploadVPFileResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/upload_vp_file", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 7/8  get_upload_file_sign —— 获取微信支付投诉图片的签名头部
//
//	POST /xpay/get_upload_file_sign  access_token + pay_sig
// ---------------------------------------------------------------------------

// GetUploadFileSignRequest 是获取微信支付投诉图片签名头部的请求体。
type GetUploadFileSignRequest struct {
	// WxpayURL 微信支付的图片地址，格式为
	// https://api.mch.weixin.qq.com/v3/merchant-service/images/{xxxxxx}
	// （就是 ComplaintMedia.MediaURL 里的地址）。
	//
	// ⚠️ 域名与路径前缀**不校验**：那是微信侧生成的地址，本包按前缀去卡，等于把将来换
	// 域名或换版本号的合法地址挡在门外（见文件头的「故意不做」）。
	WxpayURL string `json:"wxpay_url"`
	// ConvertCOS 是否转存到 COS，转存后可获得 30 分钟有效的临时下载地址。
	ConvertCOS bool `json:"convert_cos"`
	// ComplaintID 对应的投诉 ID。
	ComplaintID string `json:"complaint_id"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。见 GetComplaintListRequest 的同名字段。
	Env int `json:"env"`
}

func (r GetUploadFileSignRequest) validate() error {
	if r.WxpayURL == "" {
		return fmt.Errorf("wechat_virtualpay: WxpayURL 不能为空（来自投诉详情里的图片地址）")
	}
	return checkComplaintID(r.ComplaintID)
}

// GetUploadFileSignResponse 是获取签名头部的响应体。
type GetUploadFileSignResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// Sign 微信支付图片请求的 Authorization 头部值。
	Sign string `json:"sign"`
	// CosURL 当 ConvertCOS 为 true 时才有意义，转存后的 URL，**30 分钟有效**。
	CosURL string `json:"cos_url"`
}

// GetUploadFileSign 获取微信支付投诉图片的签名头部。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 GetComplaintList。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**。
//	req          图片地址（来自投诉详情）+ 是否转存 COS + 投诉 ID。
//
// 投诉详情里的图片托管在微信支付侧，直接下载会被拒。本函数只**换签名**，图片还得调用方
// 自己去下：拿 Sign 发一个 GET，带上三个头部——
//
//	Authorization: <Sign>
//	Accept: application/json
//	User-Agent: <非空>
//
// ⚠️ 三个头部缺一不可（User-Agent 空着也会被拒），本包不代发这个请求：它打到的是微信
// **支付**的域名，不是本包统一走的那条 /xpay/* 通道，塞进来只会让「本包只管 xpay」这条
// 边界变糊。
//
// 官方文档：POST /xpay/get_upload_file_sign
func GetUploadFileSign(ctx context.Context, accessToken, appKey string, req GetUploadFileSignRequest) (*GetUploadFileSignResponse, error) {
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

	var resp GetUploadFileSignResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/get_upload_file_sign", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 8/8  query_punishment_reasons —— 查询商户被微信支付管控的原因
//
//	POST /xpay/query_punishment_reasons  access_token + pay_sig
//
// ⚠️ 本接口**没有请求参数**，所以本包这个函数**不收 req**——它是全包唯一一个这样的。
// 请求体的处理见下面 punishmentReasonsBody 的注释。
// ---------------------------------------------------------------------------

// RecoverySpecification 是一条管控原因及其解脱路径。
type RecoverySpecification struct {
	// LimitationCaseID 该条管控原因对应的单据号，可与管控流水通知里的 BusinessCode 关联。
	LimitationCaseID string `json:"limitation_case_id"`
	// LimitationReasonType 该条管控原因所属的类型。
	LimitationReasonType string `json:"limitation_reason_type"`
	// LimitationReason 该条管控原因的简要描述。
	LimitationReason string `json:"limitation_reason"`
	// LimitationReasonDescribe 该条管控原因的进一步说明。
	LimitationReasonDescribe string `json:"limitation_reason_describe"`
	// RelateLimitations 在该条管控原因下具体受影响的能力列表。
	//
	// ⚠️ 官方这一栏**自相矛盾**：参数表的类型列写 string，而同一页的返回示例给的是数组
	// （"relate_limitations": [ { … } ]）。本包按**类型列**用 string（那是逐项声明的协议，
	// 示例只是举例）。若实测微信回的是数组，注意后果是**整个响应解析失败**（不是丢这一个
	// 字段），届时改成接受两种表示的自定义反序列化即可。
	RelateLimitations string `json:"relate_limitations"`
	// OtherRelateLimitations 未被标准枚举覆盖的受影响能力的补充说明。
	OtherRelateLimitations string `json:"other_relate_limitations"`
	// RecoverWay 微信支付建议的处理路径。
	RecoverWay string `json:"recover_way"`
	// RecoverWayParam 解脱路径对应的补充参数（尽调单号、申诉单号等）。
	RecoverWayParam string `json:"recover_way_param"`
	// RecoverHelpURL 微信支付提供的进一步说明页面。
	RecoverHelpURL string `json:"recover_help_url"`
	// LimitationActionType 该条管控原因对应的管控生效方式。
	LimitationActionType string `json:"limitation_action_type"`
	// LimitationStartDate 处置方式为延迟管控时返回的预计开始时间。
	LimitationStartDate string `json:"limitation_start_date"`
	// LimitationDate 该条管控原因实际生效的时间。
	LimitationDate string `json:"limitation_date"`
}

// punishmentReasonsBody 是 query_punishment_reasons 的请求体——它**一个字段都没有**。
//
// 官方这一页写明「请求体：无」，但 query 参数表里又要求 pay_sig。而签名是对请求体算的
// （pay_sig = hmac(appKey, uri + "&" + 请求体)），所以总得发一份字节出去。本包发 `{}`：
//
//   - **不补 env**（实现了 xpayNoEnvRequest）：官方那一页里连请求体都没有，自然没有 env
//     这一行——往一个空请求体里塞进一个官方没写的字段，是替协议做主（账单类那两个
//     「字段表里没 env」的接口是同一条道理的更轻版本，见 xpay.go 的说明）。
//   - 签名与发出去的字节是同一份（本包不变量 2），所以这里签的就是 `{}`。
//
// 若真机报 268490003（签名错误），最可能的原因是微信期望的请求体不是 `{}` —— 那就把
// xpayNoEnv 这个方法删掉，让 requestBody 补上 env（发出去与签名的都会变成 {"env":0}）。
//
// 「`pre-rewrite` 分支上那份被删掉的实现」发的也是一份空结构体（序列化成 `{}`），并且在
// 同一处记了同一句存疑——两次读同一页，结论一致。不算独立证据，但至少说明发 `{}` 不是
// 这一次新押的注。
type punishmentReasonsBody struct{}

func (punishmentReasonsBody) xpayNoEnv() {}

// QueryPunishmentReasonsResponse 是查询商户被管控原因的响应体。
type QueryPunishmentReasonsResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// AppID 小程序 AppID。
	AppID string `json:"appid"`
	// NickName 小程序昵称。
	NickName string `json:"nickname"`
	// MerchantCode 微信支付商户号。
	MerchantCode string `json:"merchant_code"`
	// LimitedFunctions 商户被管控能力列表。
	LimitedFunctions []string `json:"limited_functions"`
	// OtherLimitedFunctions 其他被管控能力描述。
	OtherLimitedFunctions string `json:"other_limited_functions"`
	// RecoverySpecifications 被管控原因及解脱路径列表。为空表示没有被管控。
	RecoverySpecifications []RecoverySpecification `json:"recovery_specifications"`
}

// QueryPunishmentReasons 查询商户被微信支付管控的原因。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 GetComplaintList。
//	appKey       商家密钥，用来算 pay_sig。
//
// ⚠️ **没有 req 形参**：本接口没有请求参数（官方写「请求体：无」），所以没有 env 可传，
// appKey 只能是现网那把——与别的接口不同，这里没有现网/沙箱的选择。请求体固定是 `{}`
// （见 punishmentReasonsBody）。
//
// 想给用户解释「为什么被限制、怎么解除」，就看 RecoverySpecifications 里每条的
// LimitationReason 与 RecoverWay。
//
// 官方文档：POST /xpay/query_punishment_reasons
func QueryPunishmentReasons(ctx context.Context, accessToken, appKey string) (*QueryPunishmentReasonsResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	// 没有 env 可传：checkEnv 不适用，appKey 也用 checkAppKeyNoEnv——本接口的请求体固定是
	// `{}`，说「Env=0 须配现网 AppKey」会把调用方支到一个根本不存在的字段上（账单类同样
	// 没有 env，用的是同一个助手）。
	if err := checkAppKeyNoEnv(appKey); err != nil {
		return nil, err
	}

	var resp QueryPunishmentReasonsResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/query_punishment_reasons", punishmentReasonsBody{}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
