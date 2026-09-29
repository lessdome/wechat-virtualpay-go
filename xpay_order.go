package wechat_virtualpay_go

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// 本文件是官方 /xpay/* 里「订单」这一类 5 个接口：
//
//	POST /xpay/query_order          查询创建的订单      access_token + pay_sig
//	POST /xpay/refund_order         启动订单退款任务    access_token + pay_sig
//	POST /xpay/notify_provide_goods 通知已发货完成      access_token
//	POST /xpay/start_download_order 发起下载订单任务    access_token + pay_sig
//	POST /xpay/query_download_order 查询下载任务结果    access_token + pay_sig
//
// 每个接口对应一个包级函数，凭据**显式传参**：accessToken 必传；除 NotifyProvideGoods
// 外 appKey 也必传，用来算 pay_sig（哪个接口要哪些，以官方 query 参数表为准，见上）。
// 凭据不藏在对象里是本包有意的形态（这一层无状态，理由见 xpay.go 文件头）；access_token
// 在这里是**只收不算**的一个字符串参数——自研小程序的用 GetStableAccessToken 换，
// 第三方平台代商家调用的 authorizer_access_token 由开放平台换，进到本类都一样。
//
// env 是每个接口都有的必填字段：**0=现网（零值即现网）/ 1=沙箱**，就是请求结构体上一个
// 裸 int（为什么不给它具名类型，见 xpay_common.go 的 checkEnv）。两个坑按字段各写各的
// 注释里了——AppKey 与环境绑死、别和响应里的 env_type（1/2 的另一套码）混。本类 5 个
// 请求结构体各自带这个字段；即使哪个结构体整个漏了它，xpay.go 的 requestBody 也会兜底
// 补一个 0。
//
// 字段在结构体里的**顺序照官方字段表逐行抄**，别重排（所以 env 是夹在中间的，不在
// 头也不在尾，5 个接口各在各的位置）。图的是读代码时能拿着文档一行行对下来；顺带
// xpay_order_test.go 的 TestOrderFieldsCoverDoc 把行序也钉住了，重排它会红。
//
// 方法一律返回「响应结构体的**指针** + error」，**响应原值返回**：微信回的字段一个不落，
// 包括公共头里的 errcode/errmsg，本包不解释、不翻译、不吞。**error 只表示这一趟没走
// 通**——参数没过本地校验，或者没拿到可解析的响应；微信的业务失败不是 error。
//
// 所以有一条必须记住：**err == nil 不等于成功**。成功与否看返回的 resp.ErrCode
// （0 才是成功），响应里的其它字段同理——本包不替调用方判断，也就没法替它标出失败。
//
// 失败时返回的响应是 **nil**（此时 err 必然非 nil），所以可以写 `if resp == nil`。它更
// 实际的用处是让**漏看 err 的人当场发现**：碰一个 nil 响应立刻 panic；换成返回值类型，
// 他会拿到一个 errcode=0 的零值响应——那恰好是本包契约里「成功」的样子，静默误导。
//
// 但**别把 nil 当成成功的判据**：`resp != nil` 只说明「拿到了响应」，业务失败（errcode
// 非 0）的响应一样非 nil。两层 nil 各是各的意思，见 QueryOrder 的注释。
//
// 文件按接口分段，每段是「枚举 → 请求/响应结构体 → 本地校验 → 调用方法」，
// 读一个接口只需要看一段。各接口共用的凭据校验在 xpay_common.go，本类共用的
// 校验（下面这一个）放在最前面。

// checkOrderRef 校验 order_id 与 wx_order_id 的「二选一」。
//
// 两个都不传微信找不到单；两个都传则哪个生效没有定义，两种都拦。它留在本文件而不是
// xpay_common.go，是因为只有本类的请求体里有这两个字段。
func checkOrderRef(orderID, wxOrderID string) error {
	switch {
	case orderID == "" && wxOrderID == "":
		return fmt.Errorf("wechat_virtualpay_go: OrderID 与 WxOrderID 必须二选一，当前两个都为空")
	case orderID != "" && wxOrderID != "":
		return fmt.Errorf("wechat_virtualpay_go: OrderID 与 WxOrderID 只能传一个，当前两个都传了")
	}
	return nil
}

// ---------------------------------------------------------------------------
// 1/5  query_order —— 查询创建的订单
//
//	POST /xpay/query_order  access_token + pay_sig
// ---------------------------------------------------------------------------

// OrderStatus 是订单状态（query_order 响应里的 order.status）。
type OrderStatus int

const (
	OrderStatusInit             OrderStatus = 0  // 订单初始化（未创建成功，不可用于支付）
	OrderStatusCreated          OrderStatus = 1  // 订单创建成功
	OrderStatusPaid             OrderStatus = 2  // 订单已支付，待发货
	OrderStatusProviding        OrderStatus = 3  // 订单发货中
	OrderStatusProvided         OrderStatus = 4  // 订单已发货
	OrderStatusRefunded         OrderStatus = 5  // 订单已退款
	OrderStatusClosed           OrderStatus = 6  // 订单已关闭（不可再使用）
	OrderStatusRefundFailed     OrderStatus = 7  // 订单退款失败
	OrderStatusRefundCompleted  OrderStatus = 8  // 用户退款完成
	OrderStatusAdFundsRecovered OrderStatus = 9  // 回收广告金完成
	OrderStatusSettleRollback   OrderStatus = 10 // 分账回退完成
)

// OrderType 是**订单自身的**类型（query_order 响应里的 order.order_type）。
//
// ⚠️ 别和 DownloadOrderType 混：那个是下载任务里的**筛选条件**（1/2/3/4），
// 两套编码只在 1/2 上看着像，含义完全不同。
type OrderType int

const (
	OrderTypeNormal    OrderType = 0 // 普通虚拟支付
	OrderTypeRefund    OrderType = 1 // 普通退款
	OrderTypeIOS       OrderType = 7 // 苹果 iOS 支付
	OrderTypeIOSRefund OrderType = 8 // 苹果 iOS 退款
)

// SettleState 是结算状态。
type SettleState int

const (
	SettleStatePending SettleState = 0 // 未开始结算（官方注明「与 3 相同」）
	SettleStateRunning SettleState = 1 // 结算中
	SettleStateSuccess SettleState = 2 // 结算成功
	SettleStateWaiting SettleState = 3 // 待结算（官方注明「与 0 相同」）
)

// OrderEnvType 是**订单响应里**的环境类型（order.env_type）。
//
// ⚠️ 取值与请求体的 env **不同**：这里是 1=现网 / 2=沙箱，请求体的 env 是 0=现网 /
// 1=沙箱。两套数撞在 1 上含义却相反，不要拿来互相比较，
// 也不要用同一个常量去赋值。之所以给它一个单独的具名类型，就是为了让这两套编码在类型
// 层面就分得开。
type OrderEnvType int

const (
	OrderEnvTypeProduction OrderEnvType = 1 // 现网
	OrderEnvTypeSandbox    OrderEnvType = 2 // 沙箱
)

// Order 是 query_order 返回的订单信息（响应体里的 order 对象）。
//
// 金额字段一律是**分**（官方字段表逐个标了「单位分」）。
type Order struct {
	// OrderID 业务侧订单号，官方字段表的**第一项**。
	OrderID string `json:"order_id"`
	// CreateTime 订单创建时间（Unix 秒级时间戳）。以下时间字段同理。
	CreateTime int64 `json:"create_time"`
	// UpdateTime 订单更新时间。
	UpdateTime int64 `json:"update_time"`
	// Status 当前订单状态。
	Status OrderStatus `json:"status"`
	// BizType 业务类型，官方目前只有 0-短剧。
	BizType int `json:"biz_type"`
	// OrderFee 订单金额，单位分。
	OrderFee int64 `json:"order_fee"`
	// CouponFee 订单优惠金额，单位分。官方注明「暂无此字段」——留着是为了将来
	// 微信开始返回时不用改结构体。
	CouponFee int64 `json:"coupon_fee"`
	// PaidFee 用户支付金额，单位分。
	PaidFee int64 `json:"paid_fee"`
	// OrderType 订单类型，注意是 0/1/7/8，见 OrderType。
	OrderType OrderType `json:"order_type"`
	// RefundFee 退款单时表示退款金额，单位分。
	RefundFee int64 `json:"refund_fee"`
	// PaidTime 支付时间（退款单则是退款时间）。
	PaidTime int64 `json:"paid_time"`
	// ProvideTime 发货时间。
	ProvideTime int64 `json:"provide_time"`
	// BizMeta 订单创建时传的商家自定义数据（退款单是 RefundOrderRequest.BizMeta）。
	BizMeta string `json:"biz_meta"`
	// EnvType 环境类型。取值是 1/2，与请求体的 env 不同，见 OrderEnvType。
	EnvType OrderEnvType `json:"env_type"`
	// Token 下单时米大师返回的 token。
	Token string `json:"token"`
	// LeftFee 本单经过退款后剩余的金额，单位分。
	//
	// 这个值就是 RefundOrderRequest.LeftFee 该填的数——想退款先查一次本接口。
	LeftFee int64 `json:"left_fee"`
	// WxOrderID 微信内部单号。可以拿它代替 OrderID 去查/退/发货。
	WxOrderID string `json:"wx_order_id"`
	// ChannelOrderID 渠道单号，即用户微信支付详情页上的「商户单号」。
	ChannelOrderID string `json:"channel_order_id"`
	// WxpayOrderID 微信支付交易单号，即用户微信支付详情页上的「交易单号」。
	WxpayOrderID string `json:"wxpay_order_id"`
	// SettTime 结算时间戳，大于 0 表示结算成功。
	SettTime int64 `json:"sett_time"`
	// SettState 结算状态。
	SettState SettleState `json:"sett_state"`
	// PlatformFeeFen 虚拟支付技术服务费，单位分，仅在 SettState=SettleStateSuccess
	// 时返回。
	PlatformFeeFen int64 `json:"platform_fee_fen"`
	// CpsFeeFen 公众号/视频号平台的 CPS 服务费，单位分，同样仅在结算成功时返回。
	CpsFeeFen int64 `json:"cps_fee_fen"`
}

// QueryOrderRequest 是查询订单的请求体。
//
// OrderID 与 WxOrderID **二选一**。
//
// 本接口的响应体裁很简单：公共头加一个 order 对象。但即便只有两层，响应结构体也不能
// 省——errcode/errmsg 就在那一层，省掉它们，调用方就看不到失败原因了。
type QueryOrderRequest struct {
	// OpenID 下单时的用户 openid。必填。
	OpenID string `json:"openid"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
	// OrderID 下单时的业务单号（下单时传的 OutTradeNo），与 WxOrderID 二选一。
	OrderID string `json:"order_id,omitempty"`
	// WxOrderID 微信内部单号，与 OrderID 二选一。
	WxOrderID string `json:"wx_order_id,omitempty"`
}

func (r QueryOrderRequest) validate() error {
	if r.OpenID == "" {
		return fmt.Errorf("wechat_virtualpay_go: OpenID 不能为空")
	}
	return checkOrderRef(r.OrderID, r.WxOrderID)
}

// QueryOrderResponse 是查询订单的响应体。
//
// **查不到订单时 Order 为 nil**，但那不是错误：微信会按成功应答（errcode=0）而不带
// order 字段，本包照原值返回。所以「有没有这一单」看 resp.Order == nil，「这一趟调用
// 成没成」看 resp.ErrCode，两件事分开。
//
// 注意这跟「resp 本身是 nil」是两回事：resp 为 nil 表示连响应都没拿到（见方法注释）。
type QueryOrderResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// Order 订单对象，查不到时为 nil。
	Order *Order `json:"order"`
}

// QueryOrder 查询创建的订单（现金单，非代币单）。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证。自研小程序传 GetStableAccessToken 换来的 access_token，第三方
//	             平台代商家调用传 authorizer_access_token——两者在这里是同一种东西。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**：env=0 配现网 AppKey、
//	             env=1 配沙箱 AppKey，两把混用会得到签名错误码。
//	req          查询条件：OrderID（业务单号）与 WxOrderID（微信单号）**二选一**。
//
// 这是「查单补发」兜底方案的基础：发货推送可能丢（用户异常退出等），建议定时调用
// 本接口核对已支付但未发货的订单。
//
// 两个 nil 不是一个意思，别混：
//
//	resp == nil        这一趟没走通（err != nil）——校验没过、连不上、非 200
//	resp.Order == nil  走通了，但这单查不到（err == nil、resp.ErrCode == 0）
//
// 查不到订单**不是错误**，也**不是失败**：微信按成功应答而不带 order 字段，属于正常
// 结果，留给调用方判断。
//
// 官方文档：POST /xpay/query_order
func QueryOrder(ctx context.Context, accessToken, appKey string, req QueryOrderRequest) (*QueryOrderResponse, error) {
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

	var resp QueryOrderResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/query_order", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 2/5  refund_order —— 启动订单退款任务
//
//	POST /xpay/refund_order  access_token + pay_sig
// ---------------------------------------------------------------------------

// RefundReason 是退款原因。官方限定这 6 个取值，**必填**。
type RefundReason string

const (
	RefundReasonNone      RefundReason = "0" // 暂无描述
	RefundReasonProduct   RefundReason = "1" // 产品问题，影响使用或效果不佳
	RefundReasonAfterSale RefundReason = "2" // 售后问题，无法满足需求
	RefundReasonUserWill  RefundReason = "3" // 意愿问题，用户主动退款
	RefundReasonPrice     RefundReason = "4" // 价格问题
	RefundReasonOther     RefundReason = "5" // 其他原因
)

// RefundFrom 是退款来源，官方限定这 3 个取值，**必填**。
type RefundFrom string

const (
	RefundFromCustomerService RefundFrom = "1" // 人工客服退款
	RefundFromUser            RefundFrom = "2" // 用户自己发起退款流程
	RefundFromOther           RefundFrom = "3" // 其它
)

// RefundOrderRequest 是发起退款的请求体。
//
// OrderID 与 WxOrderID **二选一**。
type RefundOrderRequest struct {
	// OpenID 下单时的用户 openid。必填。
	OpenID string `json:"openid"`
	// OrderID 下单时的业务单号，与 WxOrderID 二选一。
	OrderID string `json:"order_id,omitempty"`
	// WxOrderID 支付单对应的微信侧单号，与 OrderID 二选一。
	WxOrderID string `json:"wx_order_id,omitempty"`
	// RefundOrderID 本次退款单号。**必填**，长度 8–32，仅允许字母、数字、'_'、'-'。
	//
	// ⚠️ 字符集比 outTradeNo **窄**（没有 | * @），所以不能直接复用 checkOutTradeNo
	// 去校验；也注意它没有「不能以下划线开头」那条限制。
	RefundOrderID string `json:"refund_order_id"`
	// LeftFee 当前单剩余可退金额，单位分。**必填**。
	//
	// 这个数不能自己算，要先调 QueryOrder 取 order.left_fee。微信会拿它做校验。
	LeftFee int64 `json:"left_fee"`
	// RefundFee 本次退款金额，单位分。**必填**，需满足 (0, LeftFee]。
	RefundFee int64 `json:"refund_fee"`
	// BizMeta 商家自定义数据。**必填**（官方标「是」，长度允许 [0,1024]，即可以为
	// 空串），传了之后 QueryOrder 会原样返回。
	BizMeta string `json:"biz_meta"`
	// RefundReason 退款原因。必填，取值见 RefundReason。
	RefundReason RefundReason `json:"refund_reason"`
	// RefundFrom 退款来源。必填，取值见 RefundFrom。
	RefundFrom RefundFrom `json:"req_from"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

// RefundOrderResponse 是发起退款的响应体。
//
// ⚠️ 拿到这 4 个单号只说明**退款任务启动成功**，不代表钱已经退了。最终结果要调
// QueryOrder 查退款单状态，等到 OrderStatusRefundCompleted 才算退成。
type RefundOrderResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// RefundOrderID 退款单号（就是请求里传的 RefundOrderID）。
	RefundOrderID string `json:"refund_order_id"`
	// RefundWxOrderID 退款单的微信侧单号。
	RefundWxOrderID string `json:"refund_wx_order_id"`
	// PayOrderID 该退款单对应的支付单单号。
	PayOrderID string `json:"pay_order_id"`
	// PayWxOrderID 该退款单对应的支付单微信侧单号。
	PayWxOrderID string `json:"pay_wx_order_id"`
}

// refundOrderIDRe 是官方对 refund_order_id 的要求：长度 8–32，仅字母、数字、'_'、'-'。
//
// ⚠️ 它比 outTradeNo 窄（没有 | * @），也**没有**「不能以下划线开头」那条，所以不能
// 复用 outTradeNoRe。
var refundOrderIDRe = regexp.MustCompile(`^[0-9A-Za-z_-]{8,32}$`)

func (r RefundReason) valid() bool {
	switch r {
	case RefundReasonNone, RefundReasonProduct, RefundReasonAfterSale,
		RefundReasonUserWill, RefundReasonPrice, RefundReasonOther:
		return true
	}
	return false
}

func (r RefundFrom) valid() bool {
	switch r {
	case RefundFromCustomerService, RefundFromUser, RefundFromOther:
		return true
	}
	return false
}

func (r RefundOrderRequest) validate() error {
	if r.OpenID == "" {
		return fmt.Errorf("wechat_virtualpay_go: OpenID 不能为空")
	}
	if err := checkOrderRef(r.OrderID, r.WxOrderID); err != nil {
		return err
	}
	if !refundOrderIDRe.MatchString(r.RefundOrderID) {
		return fmt.Errorf("wechat_virtualpay_go: RefundOrderID %q 非法，须为 8–32 位字母/数字/_/-", r.RefundOrderID)
	}
	// LeftFee 不能自己算，要从 QueryOrder 拿微信侧记的剩余可退金额。
	if r.LeftFee <= 0 {
		return fmt.Errorf("wechat_virtualpay_go: LeftFee 必须大于 0（用 QueryOrder 取 order.left_fee，不要自己算）")
	}
	if r.RefundFee <= 0 || r.RefundFee > r.LeftFee {
		return fmt.Errorf("wechat_virtualpay_go: RefundFee（%d 分）须落在 (0, LeftFee(%d 分)] 区间内", r.RefundFee, r.LeftFee)
	}
	if !r.RefundReason.valid() {
		return fmt.Errorf("wechat_virtualpay_go: RefundReason %q 非法，取值见 RefundReason 常量", r.RefundReason)
	}
	if !r.RefundFrom.valid() {
		return fmt.Errorf("wechat_virtualpay_go: RefundFrom %q 非法，取值见 RefundFrom 常量", r.RefundFrom)
	}
	return nil
}

// RefundOrder 发起订单退款。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryOrder。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          退款参数：OrderID 与 WxOrderID **二选一**，另有必填的退款单号
//	             （RefundOrderID）、退款原因、退款来源与金额，见 RefundOrderRequest。
//
// ⚠️ 本接口只是**启动退款任务**，返回成功不代表钱已经退了。启动之后要调 QueryOrder
// 查退款单状态，等状态变成 OrderStatusRefundCompleted 才算最终成功。
//
// 退款范围：支付时间 365 天以内的订单可发起；180 天以内平台退还手续费，超过 180 天
// 不退还。
//
// iOS 订单不适用：Apple IAP 由用户向 App Store 申请，开发者**不能主动退款**，只能
// 被动接收 xpay_subscribe_ios_refund_query_notify 问询。
//
// ⚠️ 文档存疑：本接口的「注意事项」写着「使用用户态签名与支付签名」，但它的 query
// 参数表只列了 access_token 与 pay_sig，没有 signature，且那段话与其它接口的模板
// 文字雷同。本包按参数表实现（只加 pay_sig）。若实测 resp.ErrCode 是 268490003
// （签名错误），说明它确实还要用户态签名，那时改调 PostWithUserSig 即可。
//
// 官方文档：POST /xpay/refund_order
func RefundOrder(ctx context.Context, accessToken, appKey string, req RefundOrderRequest) (*RefundOrderResponse, error) {
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

	var resp RefundOrderResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/refund_order", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 3/5  notify_provide_goods —— 通知已发货完成
//
//	POST /xpay/notify_provide_goods  access_token（无签名）
// ---------------------------------------------------------------------------

// NotifyProvideGoodsRequest 是「通知已发货完成」的请求体。
//
// OrderID 与 WxOrderID **二选一**。
//
// ⚠️ 官方这一页自相矛盾：order_id 与 wx_order_id 的**必填列都标「是」**，说明列却写
// 「(与order_id二选一)」。这里按二选一的语义实现（两个字段都带 omitempty）——两者
// 都标必填的话，就成了「必须同时传两个单号」，与说明列直接冲突，没有那样设计的道理。
type NotifyProvideGoodsRequest struct {
	// OrderID 下单时传的业务单号，与 WxOrderID 二选一。
	OrderID string `json:"order_id,omitempty"`
	// WxOrderID 微信内部单号，与 OrderID 二选一。
	WxOrderID string `json:"wx_order_id,omitempty"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

func (r NotifyProvideGoodsRequest) validate() error {
	return checkOrderRef(r.OrderID, r.WxOrderID)
}

// NotifyProvideGoodsResponse 是「通知已发货完成」的响应体。
//
// 它没有自己的字段：微信只回公共头。仍然留着这个类型，因为**没有自己的字段不等于不会
// 失败**——调用方要判断成败，得有一个地方能读到 errcode。
type NotifyProvideGoodsResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
}

// NotifyProvideGoods 通知微信「已发货完成」，仅对现金单有用。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryOrder。
//	req          订单标识：OrderID 与 WxOrderID **二选一**。
//
// 注意它**没有 appKey 参数**（理由见下），所以也不校验 AppKey 与环境是否配套。
//
// 正常情况下，正确应答 xpay_goods_deliver_notify 推送就够了，不需要调本接口。它用于
// 推送没送达、需要手动把订单改成已发货状态的兜底场景。
//
// ⚠️ 注意它**不需要 appKey**：官方 query 参数表里只有 access_token，没有 pay_sig
// ——本批 5 个接口里只有它没有签名参数。这不是漏看，参数表本身就是证据。
// 若实测 resp.ErrCode 是 268490003（签名错误），说明它其实也要 pay_sig，那时改调
// PostWithPaySig。
//
// 官方文档：POST /xpay/notify_provide_goods
func NotifyProvideGoods(ctx context.Context, accessToken string, req NotifyProvideGoodsRequest) (*NotifyProvideGoodsResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp NotifyProvideGoodsResponse
	if err := PostTokenOnly(ctx, accessToken, "/xpay/notify_provide_goods", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 4/5  start_download_order —— 发起下载订单任务
//
//	POST /xpay/start_download_order  access_token + pay_sig
// ---------------------------------------------------------------------------

// DownloadOrderType 是**下载任务里**的订单类型筛选条件。
//
// ⚠️ 别和 OrderType 混：那个是订单自身的类型（0/1/7/8），这个是「我要下载哪一类」。
type DownloadOrderType int

const (
	DownloadOrderCoin         DownloadOrderType = 1 // 代币交易订单
	DownloadOrderGoods        DownloadOrderType = 2 // 道具直购交易订单
	DownloadOrderSubscription DownloadOrderType = 3 // 会员订阅订单
	DownloadOrderRefund       DownloadOrderType = 4 // 退款订单
)

// PayChannel 是支付渠道。
type PayChannel int

const (
	PayChannelNormal PayChannel = 1 // 普通虚拟支付
	PayChannelIAP    PayChannel = 2 // 苹果 IAP
)

// RefundStatusFilter 是下载订单时的退款状态筛选，仅在 DownloadOrderRefund(4) 时有效。
type RefundStatusFilter int

const (
	RefundStatusFilterAll      RefundStatusFilter = 0 // 全部
	RefundStatusFilterRefunded RefundStatusFilter = 2 // 已退款
	RefundStatusFilterOngoing  RefundStatusFilter = 4 // 退款中
	RefundStatusFilterFailed   RefundStatusFilter = 5 // 退款失败
)

// StartDownloadOrderRequest 是发起下载订单明细任务的请求体。
type StartDownloadOrderRequest struct {
	// BeginDs 开始日期，格式 YYYYMMDD，例如 20260420。
	//
	// 字段名跟着官方（ds 是它对这个日期格式的叫法），不改成 BeginDate：这一整个接口
	// 的字段都是文档原样，改一个反而对不上。
	BeginDs int64 `json:"begin_ds"`
	// EndDs 结束日期，格式同上，与 BeginDs 的间隔**不超过 31 天**。
	EndDs int64 `json:"end_ds"`
	// OrderType 要下载哪一类订单。必填，取值见 DownloadOrderType。
	OrderType DownloadOrderType `json:"order_type"`
	// OrderInfo 搜索关键字，按交易单号/商户单号/用户 ID 模糊匹配。可选。
	OrderInfo string `json:"order_info,omitempty"`
	// IsProvided 发货状态筛选：true=已发货 / false=未发货。
	//
	// OrderType 为道具(2)或会员订阅(3)时**必须**传入；其余类型可传可不传，不传按
	// true 处理。
	//
	// 用指针是因为「不传」与「传 false」在官方语义里是两件事（不传会默认成 true），
	// 用 bool 的零值会把「只想筛未发货」变成「筛已发货」。
	IsProvided *bool `json:"is_provided,omitempty"`
	// RefundStatus 退款状态筛选，仅在 OrderType 为退款订单(4) 时有效；不传按
	// RefundStatusFilterAll 处理。
	//
	// ⚠️ 与 IsProvided 不同,这里没用指针：0 本身就是合法的「全部」，不需要区分
	// 「没传」和「传了 0」。
	RefundStatus RefundStatusFilter `json:"refund_status,omitempty"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
	// PayChannel 支付渠道。必填，取值见 PayChannel。
	PayChannel PayChannel `json:"pay_channel"`
}

// StartDownloadOrderResponse 是发起下载任务的响应体。
//
// 任务本身是**异步**的：拿到 TaskID 只是排上了队，要拿着它轮询
// QueryDownloadOrder 直到 Status 变成 DownloadTaskSuccess。
type StartDownloadOrderResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// TaskID 下载任务 ID。
	TaskID string `json:"task_id"`
}

func (t DownloadOrderType) valid() bool {
	switch t {
	case DownloadOrderCoin, DownloadOrderGoods, DownloadOrderSubscription, DownloadOrderRefund:
		return true
	}
	return false
}

func (f RefundStatusFilter) valid() bool {
	switch f {
	case RefundStatusFilterAll, RefundStatusFilterRefunded,
		RefundStatusFilterOngoing, RefundStatusFilterFailed:
		return true
	}
	return false
}

func (c PayChannel) valid() bool {
	switch c {
	case PayChannelNormal, PayChannelIAP:
		return true
	}
	return false
}

// checkDateRange 校验下载任务的日期区间（官方：两个日期都是 YYYYMMDD，间隔不超过 31 天）。
func checkDateRange(begin, end int64) error {
	// 走 time.Parse 而不是自己数位数：它会连着月、日的合法范围一起校验，
	// 20261320 这种「8 位数但不是日期」也被拦下来。
	b, err := time.Parse("20060102", strconv.FormatInt(begin, 10))
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: BeginDs %d 不是合法的 YYYYMMDD 日期", begin)
	}
	e, err := time.Parse("20060102", strconv.FormatInt(end, 10))
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: EndDs %d 不是合法的 YYYYMMDD 日期", end)
	}
	if e.Before(b) {
		return fmt.Errorf("wechat_virtualpay_go: EndDs(%d) 早于 BeginDs(%d)", end, begin)
	}
	// 用 UTC 解析，天数差因此是精确的 24 小时整数倍，不受夏令时影响。
	// 边界按「含」处理：20260420 到 20260521 算 31 天，放行。
	if days := int(e.Sub(b).Hours() / 24); days > 31 {
		return fmt.Errorf("wechat_virtualpay_go: BeginDs 与 EndDs 相隔 %d 天，官方上限是 31 天", days)
	}
	return nil
}

func (r StartDownloadOrderRequest) validate() error {
	if err := checkDateRange(r.BeginDs, r.EndDs); err != nil {
		return err
	}
	if !r.OrderType.valid() {
		return fmt.Errorf("wechat_virtualpay_go: OrderType %d 非法，取值见 DownloadOrderType 常量", int(r.OrderType))
	}
	// 道具(2)和会员订阅(3)才有「发没发货」的概念，官方要求必须显式指定；其余类型没有。
	// 不传会被默认成 true——那会把「只想筛未发货」悄悄变成「筛已发货」。
	if r.IsProvided == nil && (r.OrderType == DownloadOrderGoods || r.OrderType == DownloadOrderSubscription) {
		return fmt.Errorf("wechat_virtualpay_go: OrderType=%d 时必须传 IsProvided（不传按 true 处理）", int(r.OrderType))
	}
	// 退款状态只在下载退款订单时有意义。传了就不该被静默忽略——那会让人以为筛生效了。
	if r.RefundStatus != RefundStatusFilterAll && r.OrderType != DownloadOrderRefund {
		return fmt.Errorf("wechat_virtualpay_go: RefundStatus 只在 OrderType=%d（退款订单）时有意义，当前 OrderType=%d",
			int(DownloadOrderRefund), int(r.OrderType))
	}
	if !r.RefundStatus.valid() {
		return fmt.Errorf("wechat_virtualpay_go: RefundStatus %d 非法，取值见 RefundStatusFilter 常量", int(r.RefundStatus))
	}
	if !r.PayChannel.valid() {
		return fmt.Errorf("wechat_virtualpay_go: PayChannel %d 非法，取值见 PayChannel 常量", int(r.PayChannel))
	}
	return nil
}

// StartDownloadOrder 发起下载小程序订单明细的任务。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryOrder。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          筛选条件：订单类型、起止时间、支付渠道、是否已发货等，见
//	             StartDownloadOrderRequest。符合条件的订单多时，任务耗时会明显变长。
//
// 任务是**异步**的：本方法返回只说明排上了队，要拿返回的 TaskID 去轮询
// QueryDownloadOrder，直到 Status 变成 DownloadTaskSuccess 再下载 DownloadURL。
//
// 官方文档：POST /xpay/start_download_order
func StartDownloadOrder(ctx context.Context, accessToken, appKey string, req StartDownloadOrderRequest) (*StartDownloadOrderResponse, error) {
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

	var resp StartDownloadOrderResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/start_download_order", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 5/5  query_download_order —— 查询下载任务结果
//
//	POST /xpay/query_download_order  access_token + pay_sig
// ---------------------------------------------------------------------------

// DownloadTaskStatus 是下载任务的状态。
type DownloadTaskStatus int

const (
	DownloadTaskInit    DownloadTaskStatus = 0 // 初始化
	DownloadTaskRunning DownloadTaskStatus = 1 // 运行中
	DownloadTaskSuccess DownloadTaskStatus = 2 // 成功
	DownloadTaskFailed  DownloadTaskStatus = 3 // 失败
)

// QueryDownloadOrderRequest 是查询下载任务的请求体。
type QueryDownloadOrderRequest struct {
	// TaskID 由 StartDownloadOrder 返回的下载任务 ID。必填。
	TaskID string `json:"task_id"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

// QueryDownloadOrderResponse 是查询下载任务的响应体。
type QueryDownloadOrderResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// TaskID 下载任务 ID，与请求参数对应。
	TaskID string `json:"task_id"`
	// Status 任务状态。
	Status DownloadTaskStatus `json:"status"`
	// DownloadURL 下载文件 URL，**仅** Status=DownloadTaskSuccess 时有值。
	DownloadURL string `json:"download_url"`
	// ExpireAt URL 的过期时间（Unix 秒级时间戳）。下载要赶在它之前。
	ExpireAt int64 `json:"expire_at"`
}

func (r QueryDownloadOrderRequest) validate() error {
	if r.TaskID == "" {
		return fmt.Errorf("wechat_virtualpay_go: TaskID 不能为空（由 StartDownloadOrder 返回）")
	}
	return nil
}

// QueryDownloadOrder 查询下载任务的结果。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryOrder。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          只有 TaskID（由 StartDownloadOrder 返回）与 Env。
//
// Status 为 DownloadTaskSuccess 时 DownloadURL 才有值，且它会在 ExpireAt 过期，
// 拿到就尽快下载。
//
// 官方文档：POST /xpay/query_download_order
func QueryDownloadOrder(ctx context.Context, accessToken, appKey string, req QueryDownloadOrderRequest) (*QueryDownloadOrderResponse, error) {
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

	var resp QueryDownloadOrderResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/query_download_order", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
