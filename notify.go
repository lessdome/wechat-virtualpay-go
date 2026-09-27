package wechat_virtualpay_go

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// NotifyEvent 是虚拟支付推送的事件类型。
type NotifyEvent string

const (
	// EventGoodsDeliver 用户通过现金购买道具且支付成功后推送。
	//
	// 这是发货的主通道：收到后应做幂等判断并发放道具。
	EventGoodsDeliver NotifyEvent = "xpay_goods_deliver_notify"
	// EventCoinPay 用户代币扣减成功后推送。
	EventCoinPay NotifyEvent = "xpay_coin_pay_notify"
	// EventRefund 退款完成后推送。
	EventRefund NotifyEvent = "xpay_refund_notify"
	// EventComplaint 用户发起投诉后推送。
	EventComplaint NotifyEvent = "xpay_complaint_notify"
	// EventWxpayCallback 发生尽职调查或管控流水等风控事件时推送。
	EventWxpayCallback NotifyEvent = "xpay_wxpay_callback_notify"
	// EventIOSRefundQuery 用户申请退款后、Apple 向开发者发起退款问询时推送。
	//
	// ⚠️ 这类推送要求**3 秒内应答**，且 Apple 会重复问询三次。超时则视为"不确定"，
	// 退款决定权交由 Apple。处理时不要做重活，建议只做本地状态判断。
	EventIOSRefundQuery NotifyEvent = "xpay_subscribe_ios_refund_query_notify"
)

// ErrInvalidSignature 表示推送验签失败。
//
// 用 errors.Is(err, ErrInvalidSignature) 判断。注意**签名不过就绝不能发货**。
var ErrInvalidSignature = errors.New("wechat_virtualpay_go: 推送验签失败")

// Notification 是一次解析后的推送。
//
// 按 Event 判断类型，并从对应的字段取事件数据；其余字段为 nil。
type Notification struct {
	// Event 事件类型。
	Event NotifyEvent
	// Plain 原始请求体。
	//
	// 可直接落库备查：里面有完整的业务字段，将来排查争议时很有用。
	Plain []byte

	GoodsDeliver   *GoodsDeliverNotify
	CoinPay        *CoinPayNotify
	Refund         *RefundNotify
	Complaint      *ComplaintNotify
	WxpayCallback  *WxpayCallbackNotify
	IOSRefundQuery *IOSRefundQueryNotify
}

// CommonNotifyFields 是所有推送共有的字段。
type CommonNotifyFields struct {
	ToUserName   string `json:"ToUserName"`   // 小程序原始 ID
	FromUserName string `json:"FromUserName"` // 消息来源 openid，一般为微信官方
	CreateTime   int64  `json:"CreateTime"`   // 消息发送时间
	MsgType      string `json:"MsgType"`      // 固定为 event
	// Event 事件类型。用 NotifyEvent 而非 string，与 Notification.Event 保持一致。
	Event NotifyEvent `json:"Event"`
}

// WeChatPayInfo 是微信支付信息，非微信支付渠道可能没有。
type WeChatPayInfo struct {
	// MchOrderNo 微信支付商户单号。**发货场景用它做幂等去重**（平台单号）。
	MchOrderNo string `json:"MchOrderNo"`
	// TransactionID 交易单号（微信支付订单号）。
	TransactionID string `json:"TransactionId"`
	// PaidTime 用户支付时间，Linux 秒级时间戳。
	PaidTime int64 `json:"PaidTime"`
}

// GoodsInfo 是道具参数信息。
type GoodsInfo struct {
	// ProductID 道具 ID。
	ProductID string `json:"ProductId"`
	// Quantity 数量。
	Quantity int64 `json:"Quantity"`
	// OrigPrice 物品原始价格，单位分。
	OrigPrice int64 `json:"OrigPrice"`
	// ActualPrice 物品实际支付价格，单位分。
	ActualPrice int64 `json:"ActualPrice"`
	// Attach 透传信息，即下单时传的 attach。
	Attach string `json:"Attach"`
}

// CoinInfo 是代币参数信息。
type CoinInfo struct {
	// Quantity 数量。
	Quantity int64 `json:"Quantity"`
	// OrigPrice 原始价格，单位分。
	OrigPrice int64 `json:"OrigPrice"`
	// ActualPrice 实际支付价格，单位分。
	ActualPrice int64 `json:"ActualPrice"`
	// Attach 透传信息，即下单时传的 attach。
	Attach string `json:"Attach"`
}

// TeamInfo 是拼团信息。
type TeamInfo struct {
	// ActivityID 活动 id。
	ActivityID string `json:"ActivityId"`
	// TeamID 团 id。
	TeamID string `json:"TeamId"`
	// TeamType 团类型：1-支付全部、拼成退款。
	TeamType int `json:"TeamType"`
	// TeamAction 0-创团、1-参团。
	TeamAction int `json:"TeamAction"`
}

// GoodsDeliverNotify 是道具发货推送。
//
// 处理要点（官方推荐流程）：
//  1. 取 WeChatPayInfo.MchOrderNo 作为**幂等键**（平台单号）；
//  2. 查本地订单，若该单号已发货则直接返回成功；
//  3. 否则按 FromUserName 与 GoodsInfo 发货并更新状态；
//  4. 返回成功应答，否则微信会重试（最多 15 次）。
//
// 「发货推送」与「轮询 QueryOrder」至少实现一个，两者结合最可靠——
// success 回调可能丢失（用户异常退出等），推送也可能丢失。
type GoodsDeliverNotify struct {
	CommonNotifyFields
	// OpenID 用户 openid。
	OpenID string `json:"OpenId"`
	// OutTradeNo 业务订单号。
	OutTradeNo string `json:"OutTradeNo"`
	// Env 环境标识。本包只支持现网，恒为 0。
	Env int `json:"Env"`
	// WeChatPayInfo 微信支付信息。非微信支付渠道可能没有。
	WeChatPayInfo *WeChatPayInfo `json:"WeChatPayInfo"`
	// GoodsInfo 道具参数信息。
	GoodsInfo *GoodsInfo `json:"GoodsInfo"`
	// TeamInfo 拼团信息。非拼团场景没有。
	TeamInfo *TeamInfo `json:"TeamInfo"`
}

// CoinPayNotify 是代币支付推送。
type CoinPayNotify struct {
	CommonNotifyFields
	// OpenID 用户 openid。
	OpenID string `json:"OpenId"`
	// OutTradeNo 业务订单号。
	OutTradeNo string `json:"OutTradeNo"`
	// Env 环境标识。本包只支持现网，恒为 0。
	Env int `json:"Env"`
	// WeChatPayInfo 微信支付信息。非微信支付渠道可能没有。
	WeChatPayInfo *WeChatPayInfo `json:"WeChatPayInfo"`
	// CoinInfo 代币参数信息。
	CoinInfo *CoinInfo `json:"CoinInfo"`
	// TeamInfo 拼团信息。非拼团场景没有。
	TeamInfo *TeamInfo `json:"TeamInfo"`
}

// RefundNotify 是退款完成推送。
type RefundNotify struct {
	CommonNotifyFields
	// OpenID 用户 openid。
	OpenID      string `json:"OpenId"`
	WxRefundID  string `json:"WxRefundId"`  // 微信退款单号
	MchRefundID string `json:"MchRefundId"` // 商户退款单号
	WxOrderID   string `json:"WxOrderId"`   // 退款单对应支付单的微信单号
	MchOrderID  string `json:"MchOrderId"`  // 退款单对应支付单的商户单号
	RefundFee   int64  `json:"RefundFee"`   // 退款金额，单位分
	RetCode     int    `json:"RetCode"`     // 退款结果，0 成功，非 0 失败
	RetMsg      string `json:"RetMsg"`      // 退款结果详情，失败时为原因
	// RefundStartTime 开始退款时间，秒级时间戳。
	RefundStartTime int64 `json:"RefundStartTimestamp"`
	// RefundSuccTime 结束退款时间，秒级时间戳。
	RefundSuccTime int64 `json:"RefundSuccTimestamp"`
	// WxpayRefundTxID 退款单的微信支付单号。
	WxpayRefundTxID string `json:"WxpayRefundTransactionId"`
	// RetryTimes 重试次数，从 0 开始。重试间隔 2、4、8、16… 最多 15 次。
	RetryTimes int `json:"RetryTimes"`
	// Attach iOS 退款通知附带：下单/签约时的 attach（选填）。
	Attach string `json:"Attach"`
	// WxTransactionID iOS 退款通知附带：原支付订单的 TransactionId（选填）。
	WxTransactionID string `json:"WxTransactionId"`
	// TeamInfo 拼团信息。非拼团场景没有。
	TeamInfo *TeamInfo `json:"TeamInfo"`
}

// ComplaintNotify 是用户投诉推送。
type ComplaintNotify struct {
	CommonNotifyFields
	// OpenID 用户 openid。
	OpenID string `json:"OpenId"`
	// WxOrderID 微信单号。
	WxOrderID string `json:"WxOrderId"`
	// MchOrderID 商户单号。
	MchOrderID string `json:"MchOrderId"`
	// TransactionID 微信支付交易单号。
	TransactionID string `json:"TransactionId"`
	// ComplaintID 投诉单号，用它调 GetComplaintDetail / ResponseComplaint。
	ComplaintID string `json:"ComplaintId"`
	// ComplaintDetail 投诉详情。
	ComplaintDetail string `json:"ComplaintDetail"`
	// ComplaintTime 投诉时间，秒级时间戳。
	ComplaintTime int64 `json:"ComplaintTime"`
	// RetryTimes 重试次数，从 0 开始。
	RetryTimes int `json:"RetryTimes"`
	// RequestID 请求编号。
	RequestID string `json:"RequestId"`
}

// WxpayCallbackEventType 是微信支付风控通知的类型。
type WxpayCallbackEventType string

const (
	WxpayEventDueDiligence WxpayCallbackEventType = "due_diligence" // 尽职调查
	WxpayEventPunishment   WxpayCallbackEventType = "punishment"    // 管控流水
)

// WxpayCallbackNotify 是微信支付风控事件通知。
type WxpayCallbackNotify struct {
	CommonNotifyFields
	// AppID 小程序 AppID。
	AppID string `json:"AppId"`
	// NickName 小程序昵称。
	NickName string `json:"NickName"`
	// MerchantCode 微信支付商户号。
	MerchantCode string `json:"MerchantCode"`
	// MerchantCompanyName 商户全称。
	MerchantCompanyName string `json:"MerchantCompanyName"`
	// BusinessTime 业务发生时间，格式 2026-07-03T12:47:13+08:00。
	BusinessTime string `json:"BusinessTime"`
	// BusinessCode 业务单据号，可与 RecoverySpecification.LimitationCaseID 关联。
	BusinessCode string `json:"BusinessCode"`
	// BusinessState 业务状态枚举。
	BusinessState string `json:"BusinessState"`
	// Remark 备注说明。
	Remark string `json:"Remark"`
	// EventType 通知类型：尽职调查或管控流水。
	EventType WxpayCallbackEventType `json:"EventType"`
	// RetryTimes 重试次数，从 0 开始。
	RetryTimes int `json:"RetryTimes"`
}

// IOSProvideStatus 是 iOS 退款问询里的发货状态。
type IOSProvideStatus string

const (
	IOSProvideNotYet  IOSProvideStatus = "0" // 未发货
	IOSProvideDone    IOSProvideStatus = "1" // 已发货
	IOSProvideOngoing IOSProvideStatus = "2" // 发货中
)

// IOSRefundQueryNotify 是 iOS 退款问询推送。
//
// 注意：本事件的所有字段都是**字符串**且为 snake_case，与其它事件风格不同。
//
// Apple 支付不支持开发者主动退款，用户只能在 App Store 申请。Apple 会向开发者
// 发起**重复三次**的退款问询，开发者可据自身策略响应；但最终结果仍由 Apple 决定。
// 连续 3 次、3 秒内未应答，平台会向 Apple 返回「不确定」。
type IOSRefundQueryNotify struct {
	// RefundTime 问询时间，Unix 时间戳。
	RefundTime string `json:"refund_time"`
	// OrderTime 该笔退款对应的交易时间，Unix 时间戳。
	OrderTime string `json:"order_time"`
	// ChannelBill Apple 支付票据号。
	ChannelBill string `json:"channel_bill"`
	// BundleID 应用的 Apple bundleid。
	BundleID string `json:"bundleid"`
	// ProductID 道具 ID。
	ProductID string `json:"product_id"`
	// PCount 道具/代币数量。
	PCount string `json:"p_count"`
	// RefundRequestReason 用户请求退款的原因。
	RefundRequestReason string `json:"refund_request_reason"`
	// ProvideStatus 发货状态。
	ProvideStatus IOSProvideStatus `json:"provide_status"`
	// PayOrderID 退款对应支付订单号。
	PayOrderID string `json:"pay_order_id"`
}

// ParseNotification 解析一次推送：读请求体 → 验签 → 识别事件 → 反序列化为对应结构。
//
// token 是 MP 后台「开发管理 → 消息推送配置」里的 **Token 令牌**——不是支付凭据，
// 也别和 AppKey 搞混。
//
// 只支持 **JSON 报文 + 明文模式**：MP 后台「消息推送配置」里两项都要配对——数据格式
// 选 JSON、消息加解密方式选明文。配成 XML 或安全模式，推送都会被拒，本方法会直接
// 返回明确的错误，而不是含糊地解析失败。
//
// 另外官方要求**应答格式与推送格式一致**，只支持 JSON 时这条自然满足。
//
// 任何一步不过都返回错误，此时 **Notification 为 nil**。返回错误时应当回失败应答
// 让微信重试——**绝不要在验签失败时仍然发货**。
//
// 本方法只管解析，不管应答，也不管你怎么处理：回什么、什么时候回，都由调用方决定。
// 应答体就是两个普通结构体——Ack（普通事件）与 IOSRefundQueryResponse（iOS 退款
// 问询），本包不做任何加工，json.Marshal 写出去即可。
//
// 典型用法：
//
//	notif, err := wechat_virtualpay_go.ParseNotification(token, r)
//	if err != nil {
//		writeJSON(w, wechat_virtualpay_go.Ack{ErrCode: 1, ErrMsg: err.Error()}) // 让微信重试
//		return
//	}
//	ack := wechat_virtualpay_go.Ack{ErrCode: 0, ErrMsg: "success"}
//	switch notif.Event {
//	case wechat_virtualpay_go.EventGoodsDeliver:
//		if err := deliver(notif.GoodsDeliver); err != nil { // 幂等发货…
//			ack = wechat_virtualpay_go.Ack{ErrCode: 1, ErrMsg: err.Error()}
//		}
//	}
//	writeJSON(w, ack)
func ParseNotification(token string, r *http.Request) (*Notification, error) {
	if token == "" {
		return nil, errors.New("wechat_virtualpay_go: token 不能为空（MP 后台「消息推送配置」里的 Token 令牌）")
	}
	body, err := readAllLimited(r)
	if err != nil {
		return nil, err
	}
	query := r.URL.Query()

	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("wechat_virtualpay_go: 推送请求体为空")
	}

	timestamp := query.Get("timestamp")
	nonce := query.Get("nonce")
	if timestamp == "" || nonce == "" {
		return nil, errors.New("wechat_virtualpay_go: 推送请求缺少 timestamp 或 nonce")
	}

	// 本包只支持明文模式。后台若配成安全模式，报文是 AES 加密的，这里直接说清楚，
	// 而不是丢一个含糊的「解析失败」出去。
	if query.Get("encrypt_type") == "aes" {
		return nil, errors.New("wechat_virtualpay_go: 收到安全模式推送，但本包只支持明文模式——请到 MP 后台把「消息加解密方式」改为明文模式")
	}
	if !verifySignature(token, timestamp, nonce, query.Get("signature")) {
		return nil, ErrInvalidSignature
	}

	// 以下：识别事件类型并反序列化到对应结构。
	var header struct {
		Event string `json:"Event"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 解析推送事件头失败: %w", err)
	}

	notif := &Notification{
		Event: NotifyEvent(header.Event),
		Plain: body,
	}

	// iOS 退款问询是特殊的一类：它的报文**不带 Event 字段**（字段全是 snake_case），
	// 上面的分派识别不到。用它的特征字段兜底认出来。
	if notif.Event == "" {
		if !isIOSRefundQueryPayload(body) {
			// 完全没有 Event，说明这根本不是一条推送。
			// ⚠️ 这里**不能**放行：调用方会把「解析成功」当成「收到了真实推送」，
			// 照常回成功应答，微信便不再重推——这条推送就永久丢了。
			return nil, errors.New("wechat_virtualpay_go: 推送报文里没有 Event 字段，无法识别事件类型")
		}
		notif.Event = EventIOSRefundQuery
	}

	// 注意：必须让 notif 的字段与 target 指向**同一个对象**。
	// 写成 `notif.GoodsDeliver, target = &X{}, &X{}` 会创建两个不同的实例，
	// 反序列化填的是 target，而 notif 上挂的是另一个空对象——解析结果永远为空。
	var target any
	switch notif.Event {
	case EventGoodsDeliver:
		v := &GoodsDeliverNotify{}
		notif.GoodsDeliver, target = v, v
	case EventCoinPay:
		v := &CoinPayNotify{}
		notif.CoinPay, target = v, v
	case EventRefund:
		v := &RefundNotify{}
		notif.Refund, target = v, v
	case EventComplaint:
		v := &ComplaintNotify{}
		notif.Complaint, target = v, v
	case EventWxpayCallback:
		v := &WxpayCallbackNotify{}
		notif.WxpayCallback, target = v, v
	case EventIOSRefundQuery:
		v := &IOSRefundQueryNotify{}
		notif.IOSRefundQuery, target = v, v
	default:
		// 未知事件不报错：微信将来可能新增事件类型，报错会让对接方在微信加字段时
		// 突然收不到任何推送。返回带 Event 的 Notification，由调用方决定怎么处理。
		return notif, nil
	}

	if err := json.Unmarshal(body, target); err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 解析 %s 事件失败: %w", notif.Event, err)
	}
	return notif, nil
}

// maxNotifyBodySize 限制推送请求体大小，防止超大报文打爆内存。
const maxNotifyBodySize = 1 << 20 // 1 MiB

func readAllLimited(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, errors.New("wechat_virtualpay_go: 推送请求没有 body")
	}
	defer r.Body.Close()

	body, err := io.ReadAll(io.LimitReader(r.Body, maxNotifyBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 读取推送请求体失败: %w", err)
	}
	if len(body) > maxNotifyBodySize {
		return nil, fmt.Errorf("wechat_virtualpay_go: 推送请求体超过 %d 字节上限", maxNotifyBodySize)
	}
	return body, nil
}

// isIOSRefundQueryPayload 判断一段推送明文是不是 iOS 退款问询。
//
// 该事件的报文没有 Event 字段（官方字段表里全是 refund_time / channel_bill 这类
// snake_case 字段），所以没法靠事件名分派，只能靠特征字段识别。channel_bill
// （Apple 支付票据号）与 bundleid 是它独有的，同时出现即可判定。
//
// ⚠️ 这是**基于文档字段表的推断**：若线上实测该事件其实带 Event 字段，这段兜底就
// 是多余的（但不会误判——正常事件走的是上面的 Event 分派）。
func isIOSRefundQueryPayload(plain []byte) bool {
	var probe struct {
		ChannelBill string `json:"channel_bill"`
		BundleID    string `json:"bundleid"`
	}
	if err := json.Unmarshal(plain, &probe); err != nil {
		return false
	}
	return probe.ChannelBill != "" && probe.BundleID != ""
}

// Ack 是 ErrCode 形态的推送应答，普通推送事件都用它。
//
// 本包对应答不做任何加工——直接 json.Marshal 写出去即可，Content-Type 用
// application/json; charset=utf-8。
//
// 成功（微信不再重推）：
//
//	Ack{ErrCode: 0, ErrMsg: "success"}
//
// ⚠️ **务必确认发货真的落地了再回成功**——回了成功但没发货，微信不会重试，
// 这笔单就永久丢了。
//
// 失败（微信会按 2、4、8、16… 的间隔重试，最多 15 次）：
//
//	Ack{ErrCode: 1, ErrMsg: err.Error()}
type Ack struct {
	// ErrCode 应答状态。0 表示成功，其他值微信会重试。
	ErrCode int `json:"ErrCode"`
	// ErrMsg 错误信息，用于调试。成功时官方示例给的是 "success"。
	ErrMsg string `json:"ErrMsg"`
}

// IOSRefundQueryResponse 是 iOS 退款问询的应答内容。
//
// 这条问询的应答**不是** Ack 那种 ErrCode 形态，只能用它——回错了微信当无效应答，
// 而 Apple 只问询三次、每次 3 秒，错过等于把判定权交出去。
//
// ⚠️ 必须在 **3 秒内**返回；这条路径上不要查库、不要调外部接口，否则会被判为「不确定」。
type IOSRefundQueryResponse struct {
	// ResultCode 结果码：0-放过，建议退款；1-拦截，拒绝退款。
	ResultCode int32 `json:"result_code"`
	// ResultInfo 结果描述。
	ResultInfo string `json:"result_info"`
	// Evidence 决策凭据（**必填**），业务需给出建议退款/拒绝退款的依据，用于退款审计。
	Evidence string `json:"evidence"`
}
