package wechat_virtualpay_go

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
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

// notifyContentType 是推送应答的 Content-Type。
//
// 虚拟支付推送只走 JSON：MP 后台「消息推送配置」里的数据格式选 JSON 即可。
// XML 报文本包不解析，也不生成 XML 应答。
const notifyContentType = "application/json; charset=utf-8"

// NotifyConfig 是推送接收端的配置。
//
// 这些值**不是**支付凭据，请在 MP 后台「开发管理 → 消息推送配置」里查看/设置。
type NotifyConfig struct {
	// AppID 小程序 AppID。用于校验解密结果里携带的 appid，防止重放他人报文。
	AppID string
	// Token 消息推送配置里的「Token 令牌」，用于验签。**与 AppKey 无关。**
	Token string
	// EncodingAESKey 消息加解密密钥（43 个字符）。
	//
	// 留空表示只处理**明文模式**；填了才能处理**安全模式**。
	// 兼容模式（明文密文共存）本包不支持——微信本身也不建议用。
	EncodingAESKey string
}

// Notifier 负责推送的验签、解密与解析。它是并发安全的（无可变状态）。
type Notifier struct {
	appID string
	// notifyToken 是 MP 后台「消息推送配置」里的令牌，用于验签。
	// 与 access_token 是两回事，命名上刻意区分开。
	notifyToken string
	aesKey      []byte // nil 表示只支持明文模式
}

// NewNotifier 校验配置并构造 Notifier。
func NewNotifier(cfg NotifyConfig) (*Notifier, error) {
	if cfg.AppID == "" {
		return nil, errors.New("wechat_virtualpay_go: NotifyConfig.AppID 不能为空")
	}
	if cfg.Token == "" {
		return nil, errors.New("wechat_virtualpay_go: NotifyConfig.Token 不能为空（MP 后台消息推送配置里的令牌）")
	}
	n := &Notifier{appID: cfg.AppID, notifyToken: cfg.Token}
	if cfg.EncodingAESKey != "" {
		key, err := decodeAESKey(cfg.EncodingAESKey)
		if err != nil {
			return nil, err
		}
		n.aesKey = key
	}
	return n, nil
}

// SupportsEncrypted 表示是否配置了 EncodingAESKey（即能否处理安全模式）。
func (n *Notifier) SupportsEncrypted() bool { return n.aesKey != nil }

// Notification 是一次解析后的推送。
//
// 按 Event 判断类型，并从对应的字段取事件数据；其余字段为 nil。
type Notification struct {
	// Event 事件类型。
	Event NotifyEvent
	// Plain 解密后的明文原文（明文模式下即原始请求体）。
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

// Parse 解析一次推送：验签（必要时解密）→ 识别事件 → 反序列化为对应结构。
//
// 参数：
//   - query：URL 上的查询参数（含 signature / timestamp / nonce，安全模式还有
//     msg_signature 与 encrypt_type）
//   - body：原始请求体
//
// 任何一步不过都返回错误——调用方应当把错误直接回给微信（不返回成功应答），
// 微信会重试；**绝不要在验签失败时仍然发货**。
func (n *Notifier) Parse(query url.Values, body []byte) (*Notification, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("wechat_virtualpay_go: 推送请求体为空")
	}

	timestamp := query.Get("timestamp")
	nonce := query.Get("nonce")

	if timestamp == "" || nonce == "" {
		return nil, errors.New("wechat_virtualpay_go: 推送请求缺少 timestamp 或 nonce")
	}

	// 安全模式靠 encrypt_type=aes 判定，而不是靠有无 Encrypt——
	// 明文模式同样可能带 signature 参数。
	encrypted := query.Get("encrypt_type") == "aes"

	plain := body
	if encrypted {
		if n.aesKey == nil {
			return nil, errors.New("wechat_virtualpay_go: 收到安全模式推送，但 NotifyConfig 未配置 EncodingAESKey")
		}
		var envelope struct {
			Encrypt string `json:"Encrypt"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("wechat_virtualpay_go: 解析安全模式信封失败: %w", err)
		}
		if envelope.Encrypt == "" {
			return nil, errors.New("wechat_virtualpay_go: 安全模式推送里没有 Encrypt 字段")
		}
		// ⚠️ 安全模式必须用 msg_signature 校验，官方文档明确警告不要用 signature。
		if !verifyEncryptedSignature(n.notifyToken, timestamp, nonce, envelope.Encrypt, query.Get("msg_signature")) {
			return nil, ErrInvalidSignature
		}
		decrypted, err := aesDecrypt(n.aesKey, envelope.Encrypt, n.appID)
		if err != nil {
			return nil, err
		}
		plain = decrypted
	} else {
		if !verifyPlainSignature(n.notifyToken, timestamp, nonce, query.Get("signature")) {
			return nil, ErrInvalidSignature
		}
	}

	return parseNotification(plain)
}

// ParseHTTP 是 Parse 的便捷封装，直接从 *http.Request 取值。
//
// 典型用法：
//
//	notif, err := notifier.ParseHTTP(r)
//	if err != nil {
//		// 回一个失败应答，让微信重试；不要发货
//	}
//	switch notif.Event {
//	case wechat_virtualpay_go.EventGoodsDeliver:
//		// 幂等发货…
//	}
//	body, ct := wechat_virtualpay_go.Ack()
func (n *Notifier) ParseHTTP(r *http.Request) (*Notification, error) {
	body, err := readAllLimited(r)
	if err != nil {
		return nil, err
	}
	return n.Parse(r.URL.Query(), body)
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

// parseNotification 识别事件类型并填充对应结构。
func parseNotification(plain []byte) (*Notification, error) {
	var header struct {
		Event string `json:"Event"`
	}
	if err := json.Unmarshal(plain, &header); err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 解析推送事件头失败: %w", err)
	}

	notif := &Notification{
		Event: NotifyEvent(header.Event),
		Plain: plain,
	}

	// iOS 退款问询是特殊的一类：它的报文**不带 Event 字段**（字段全是 snake_case），
	// 上面的分派识别不到。用它的特征字段兜底认出来。
	if notif.Event == "" {
		if !isIOSRefundQueryPayload(plain) {
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

	if err := json.Unmarshal(plain, target); err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 解析 %s 事件失败: %w", notif.Event, err)
	}
	return notif, nil
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

// ackPayload 是虚拟支付推送的应答体。
type ackPayload struct {
	// ErrCode 应答状态。0 表示成功，其他值微信会重试。
	ErrCode int `json:"ErrCode"`
	// ErrMsg 错误信息，用于调试。
	ErrMsg string `json:"ErrMsg"`
}

// Ack 生成成功应答（等价于 ErrCode=0）。
//
// 应答成功即表示"已处理完，别再推了"。**务必确认发货真的落地了再回成功**——
// 回了成功但没发货，微信不会重试，这笔单就永久丢了。
//
// 第二个返回值是 Content-Type，直接写进响应头即可。
func Ack() (body []byte, contentType string) {
	return marshalAck(ackPayload{ErrCode: 0, ErrMsg: "success"})
}

// AckError 生成失败应答，微信会按 2、4、8、16… 的间隔重试，最多 15 次。
func AckError(errCode int, errMsg string) (body []byte, contentType string) {
	return marshalAck(ackPayload{ErrCode: errCode, ErrMsg: errMsg})
}

func marshalAck(p ackPayload) ([]byte, string) {
	body, err := json.Marshal(p)
	if err != nil {
		// 结构固定（一个 int、一个 string），json.Marshal 不会失败；兜底给个合法应答。
		return []byte(`{"ErrCode":0,"ErrMsg":"success"}`), notifyContentType
	}
	return body, notifyContentType
}

// IOSRefundQueryResponse 是 iOS 退款问询的应答内容。
//
// ⚠️ 必须在 **3 秒内**返回，Apple 会问询三次。不要在这条路径上做耗时操作
// （查库、调外部接口），否则会被判为「不确定」。
type IOSRefundQueryResponse struct {
	// ResultCode 结果码：0-放过，建议退款；1-拦截，拒绝退款。
	ResultCode int32 `json:"result_code"`
	// ResultInfo 结果描述。
	ResultInfo string `json:"result_info"`
	// Evidence 决策凭据（**必填**），业务需给出建议退款/拒绝退款的依据，用于退款审计。
	Evidence string `json:"evidence"`
}

// EncryptResponse 在安全模式下加密一段应答明文。
//
// 安全模式的**普通应答**（ErrCode 那种）无需加密，直接回即可；本方法用于需要返回
// **结构化内容**的场景，典型是 iOS 退款问询。
//
// nonce 要传**请求 URL 上的 nonce 原值**——微信要求回包的 Nonce 与请求一致。
//
// 官方要求的加密回包是**四个字段，缺一不可**：
//
//	Encrypt + MsgSignature + TimeStamp + Nonce
//
// 其中 MsgSignature = sha1( sort([Token, TimeStamp, Nonce, Encrypt]) )。
// 少了任何一个，微信都视为无效应答；iOS 退款问询在「3 秒 / 3 次」的约束下会直接
// 被判为「不确定」。
func (n *Notifier) EncryptResponse(plain []byte, nonce string) ([]byte, error) {
	if n.aesKey == nil {
		return nil, errors.New("wechat_virtualpay_go: 未配置 EncodingAESKey，无法加密应答（明文模式直接返回明文即可）")
	}
	if nonce == "" {
		return nil, errors.New("wechat_virtualpay_go: EncryptResponse 需要请求 URL 上的 nonce 原值")
	}

	random16, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	encrypt, err := aesEncrypt(string(n.aesKey), n.appID, plain, random16)
	if err != nil {
		return nil, err
	}

	timeStamp := time.Now().Unix()
	msgSignature := sha1SortedHex(n.notifyToken, strconv.FormatInt(timeStamp, 10), nonce, encrypt)

	envelope := struct {
		Encrypt      string `json:"Encrypt"`
		MsgSignature string `json:"MsgSignature"`
		TimeStamp    int64  `json:"TimeStamp"`
		Nonce        string `json:"Nonce"`
	}{encrypt, msgSignature, timeStamp, nonce}
	return json.Marshal(envelope)
}

// IsKnownEvent 判断事件类型是否为本包已知的 6 类之一。
//
// Parse 对未知事件不报错（微信可能新增事件），用本方法可以区分
// 「已知事件」与「将来新增的事件」。
func (e NotifyEvent) IsKnownEvent() bool {
	switch e {
	case EventGoodsDeliver, EventCoinPay, EventRefund,
		EventComplaint, EventWxpayCallback, EventIOSRefundQuery:
		return true
	}
	return false
}

// String 便于日志输出。
func (e NotifyEvent) String() string { return string(e) }
