package wechat_virtualpay_go

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// 本文件实现「道具直购」的下单参数生成。
//
// 官方 2.1 时序图里这条流程是：服务端把参数拼成一个字符串、算两个签名，交给小程序端
// 由 wx.requestVirtualPayment 拉起支付。其中用户态签名要用 session_key，而 session_key
// 只能由 wx.login 的 code 换来——**这一步本函数替开发者做了**，所以调用方只需要把
// 前端拿到的 code 传进来。

// payMethodRequestVirtualPayment 是拉起支付时固定的签名 uri。
//
// 注意它是**签名用的 uri**，不是 HTTP 路径——下单本身没有服务端请求
// （官方 2.5：「uri | 固定填 requestVirtualPayment」）。
const payMethodRequestVirtualPayment = "requestVirtualPayment"

// PaymentMode 是 wx.requestVirtualPayment 的**顶层参数** mode，不属于 signData。
type PaymentMode string

const (
	// ModeShortSeriesGoods 道具直购。
	ModeShortSeriesGoods PaymentMode = "short_series_goods"
	// ModeShortSeriesCoin 代币充值（其下单参数生成本包尚未实现）。
	ModeShortSeriesCoin PaymentMode = "short_series_coin"
)

// VirtualPaymentParams 是交给小程序端 wx.requestVirtualPayment 的完整参数。
type VirtualPaymentParams struct {
	// SignData 必须**以字符串原样**传给前端，前端不得重新序列化。
	// 它既是被签名的原文，也是微信校验的原文。
	SignData string
	// PaySig 支付签名。
	PaySig string
	// Signature 用户态签名。
	Signature string
	// Mode 支付类型。
	Mode PaymentMode
}

// GoodsPaymentRequest 是道具直购一次下单所需的输入。
type GoodsPaymentRequest struct {
	// ProductID 道具 ID（signData 的 productId）。必填。
	ProductID string
	// GoodsPrice 道具单价，单位**分**（signData 的 goodsPrice）。必填。
	// 微信用它校验与后台配置的道具价格是否一致。
	GoodsPrice int64
	// Quantity 购买数量（signData 的 buyQuantity）。<=0 时按 1 处理。
	Quantity int64
	// ActivitySellingPrice 优惠价，单位**分**。可选。
	// 传了它就是实际下单价格。官方只说「需与 goodsPrice 一起传入」——道具直购下
	// goodsPrice 本来就必填，所以无需额外校验。
	ActivitySellingPrice int64
	// OutTradeNo 业务订单号（signData 的 outTradeNo）。必填。
	// 8–32 字符，只能由数字、大小写字母、_-|*@ 组成，不能以 _ 开头，且每单只能用一次。
	OutTradeNo string
	// Attach 透传数据（signData 的 attach）。必填，发货通知会原样带回。
	Attach string
	// Code 前端 wx.login() 得到的登录凭证。必填。
	//
	// 它只有**五分钟有效、且只能用一次**，所以要现拿现用；本函数内部会用它
	// 调 Code2Session 换出 session_key 来算用户态签名，开发者不必自己换。
	Code string
}

// goodsSignData 是道具直购的 signData 结构。
//
// 字段顺序就是序列化顺序，与官方示例保持一致。
type goodsSignData struct {
	OfferID              string `json:"offerId"`
	BuyQuantity          int64  `json:"buyQuantity"`
	Env                  int    `json:"env"`
	CurrencyType         string `json:"currencyType"`
	ProductID            string `json:"productId"`
	GoodsPrice           int64  `json:"goodsPrice"`
	ActivitySellingPrice int64  `json:"activitySellingPrice,omitempty"`
	OutTradeNo           string `json:"outTradeNo"`
	Attach               string `json:"attach"`
}

// outTradeNoRe 是官方对 outTradeNo 的字符与长度要求。
var outTradeNoRe = regexp.MustCompile(`^[0-9A-Za-z_|*@-]{8,32}$`)

// BuildGoodsPayment 生成道具直购的下单参数。
//
// 调用方只需要准备**前端 wx.login 拿到的 code**——本函数内部调 Code2Session 换出
// session_key，再用它算用户态签名，最后连同支付签名一起返回：
//
//	p, err := wechat_virtualpay_go.BuildGoodsPayment(ctx, appID, appSecret, offerID, appKey,
//		wechat_virtualpay_go.GoodsPaymentRequest{
//			ProductID:  "prod_001",
//			GoodsPrice: 100, // 单位：分
//			OutTradeNo: "ORDER20260101001",
//			Attach:     "自定义透传数据",
//			Code:       loginCode, // 前端 wx.login() 拿到的
//		})
//
// 返回后把 p.SignData / p.PaySig / p.Signature / p.Mode 交给前端，
// 传给 wx.requestVirtualPayment。**SignData 必须原样传**，前端不得重新序列化。
//
// 注意客户端有个前提：wx.requestVirtualPayment 需要基础库 >= 2.19.2。
func BuildGoodsPayment(ctx context.Context, appID, appSecret, offerID, appKey string, req GoodsPaymentRequest) (*VirtualPaymentParams, error) {
	// 先把本地能验的都验掉，再动网络——code 只能用一次，别为了一条明显不合法
	// 的请求把它烧掉。
	if offerID == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: offerID 不能为空")
	}
	if appKey == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: appKey 不能为空")
	}
	if req.Code == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: Code 不能为空（前端 wx.login 拿到的登录凭证）")
	}
	if req.ProductID == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: 道具直购必须提供 ProductID")
	}
	if req.GoodsPrice <= 0 {
		return nil, fmt.Errorf("wechat_virtualpay_go: 道具直购必须提供正数 GoodsPrice（单位：分）")
	}
	if err := checkOutTradeNo(req.OutTradeNo); err != nil {
		return nil, err
	}

	// 换登录态。用 code 换 session_key，用它算用户态签名。
	sess, err := Code2Session(ctx, appID, appSecret, req.Code)
	if err != nil {
		return nil, err
	}

	qty := req.Quantity
	if qty <= 0 {
		qty = 1
	}

	raw, err := json.Marshal(goodsSignData{
		OfferID:              offerID,
		BuyQuantity:          qty,
		Env:                  0, // 本包只支持现网环境
		CurrencyType:         "CNY",
		ProductID:            req.ProductID,
		GoodsPrice:           req.GoodsPrice,
		ActivitySellingPrice: req.ActivitySellingPrice,
		OutTradeNo:           req.OutTradeNo,
		Attach:               req.Attach,
	})
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 序列化下单参数失败: %w", err)
	}

	return finishPayment(appKey, sess.SessionKey, ModeShortSeriesGoods, string(raw)), nil
}

// finishPayment 把已经序列化好的 signData 补成完整参数：算两个签名、填 Mode。
//
// 它不含任何业务判断——两种支付模式都走这一条，区别只在外层拼哪个结构体。
func finishPayment(appKey, sessionKey string, mode PaymentMode, signData string) *VirtualPaymentParams {
	return &VirtualPaymentParams{
		SignData:  signData,
		PaySig:    CalcPaySig(appKey, payMethodRequestVirtualPayment, signData),
		Signature: CalcSignature(sessionKey, signData),
		Mode:      mode,
	}
}

// checkOutTradeNo 按官方对 outTradeNo 的要求做校验。
func checkOutTradeNo(v string) error {
	if v == "" {
		return fmt.Errorf("wechat_virtualpay_go: OutTradeNo 不能为空")
	}
	if !outTradeNoRe.MatchString(v) {
		return fmt.Errorf("wechat_virtualpay_go: OutTradeNo %q 非法，须为 8–32 位数字/大小写字母/_-|*@", v)
	}
	if v[0] == '_' {
		return fmt.Errorf("wechat_virtualpay_go: OutTradeNo 不能以下划线开头")
	}
	return nil
}

// NewOutTradeNo 生成一个符合微信规范的业务订单号。
//
// 规则取自官方（signData 的 outTradeNo 一栏）：8–32 字符；只能由数字、大小写字母、
// 符号 _-|*@ 组成；不能以下划线开头；**每个订单号只能用一次**。
//
// 格式是「14 位时间戳 + 16 位十六进制随机数」，共 30 字符：时间前缀让单号大致可按时间
// 排序、便于排查，后 16 位（64 位熵）保证同一秒内也不会撞号。
//
// 另外那句官方备注要留意：outTradeNo 重复会失败，但括号里写着「极端情况不保证唯一，
// 不建议业务强依赖唯一性」——所以不要指望「用重复单号一定会被拒」来防重，该做的幂等
// 还是要做。
func NewOutTradeNo() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("wechat_virtualpay_go: 生成订单号失败: %w", err)
	}
	return time.Now().Format("20060102150405") + hex.EncodeToString(b[:]), nil
}
