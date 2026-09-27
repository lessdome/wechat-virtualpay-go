package wechat_virtualpay_go

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// payMethodRequestVirtualPayment 是拉起虚拟支付时固定的签名 method 值。
// 注意：这是**签名用的 method**，不是 HTTP 路径；下单本身没有服务端请求。
const payMethodRequestVirtualPayment = "requestVirtualPayment"

// 关于「平台」：signData **没有** platform 字段。
//
// 官方 wx.requestVirtualPayment 文档给出的 signData 示例为：
//
//	{"offerId":"123","buyQuantity":1,"env":0,"currencyType":"CNY",
//	 "productId":"testproductId","goodsPrice":10,"outTradeNo":"xxxxxx","attach":"testdata"}
//
// 设备路由（Android/鸿蒙/Windows 走微信支付，iOS 走 Apple 支付）由微信**按客户端
// 设备类型自动完成**，不需要也不应该在 signData 里指定平台。早期部分实现凭空加了
// platform，那会让 signData 多出字段、与微信预期不一致（-15016 signData 格式有问题）。
//
// iOS 与 Android 的差异是**业务层面**的，真实存在且必须知道，但不影响下单参数：
// iOS 走 Apple IAP，费率约 12%、结算 45–60 天、开发者无法主动退款；
// Android 走微信支付，费率约 1%、T+3、可主动退款。
//
// 同样地，mode（道具直购/代币充值）也不是 signData 的一部分，而是
// wx.requestVirtualPayment 的**顶层参数**，随 PayMode 单独交给前端。

// PayMode 是 wx.requestVirtualPayment 的 mode 参数。
// 注意：它**不属于 signData**，不参与签名，但要一并交给前端。
type PayMode string

const (
	// ModeShortSeriesGoods 道具直购。
	ModeShortSeriesGoods PayMode = "short_series_goods"
	// ModeShortSeriesCoin 代币充值。
	ModeShortSeriesCoin PayMode = "short_series_coin"
)

// PrepayRequest 是构建支付参数所需的输入。
type PrepayRequest struct {
	// ProductID 道具 ID。Mode=ModeShortSeriesGoods 时必填。
	ProductID string
	// GoodsPrice 道具单价，单位：分。Mode=ModeShortSeriesGoods 时必填。
	// 用于微信侧校验价格是否与后台一致。
	GoodsPrice int64
	// Quantity 购买数量，<=0 时按 1 处理。
	Quantity int
	// OutTradeNo 业务订单号，8–32 字符，由数字/大小写字母及 _-|*@ 组成，
	// 不能以下划线开头，且每个订单号只能使用一次。
	OutTradeNo string
	// Attach 透传数据，发货通知会原样带回。
	Attach string
	// SessionKey 由 wx.login 的 code 通过 code2Session 换取，用于计算 signature。必填。
	SessionKey string
	// Mode 支付模式，留空默认 ModeShortSeriesGoods。
	Mode PayMode
	// ActivitySellingPrice 优惠价（分），可选，须不低于道具价格的 40%。
	ActivitySellingPrice int64
}

// PrepayParams 是交给小程序端 wx.requestVirtualPayment 的完整参数。
type PrepayParams struct {
	// SignData 必须**以字符串原样**传给前端，前端不得重新序列化，
	// 否则签名会失配（这是最常见的 -15006 来源）。
	SignData  string
	PaySig    string
	Signature string
	Mode      PayMode
}

// prepayBody 是下单请求体的内部结构。
//
// 字段顺序即签名串顺序，**不可随意调整**；且必须用结构体（而非 map），
// 以保证序列化顺序稳定。
type prepayBody struct {
	OfferID              string `json:"offerId"`
	BuyQuantity          int    `json:"buyQuantity"`
	Env                  int    `json:"env"`
	CurrencyType         string `json:"currencyType"`
	ProductID            string `json:"productId,omitempty"`
	GoodsPrice           int64  `json:"goodsPrice,omitempty"`
	ActivitySellingPrice int64  `json:"activitySellingPrice,omitempty"`
	OutTradeNo           string `json:"outTradeNo"`
	Attach               string `json:"attach"`
}

var outTradeNoRe = regexp.MustCompile(`^[0-9A-Za-z_|*@-]{8,32}$`)

// BuildPaymentParams 构建支付参数（即业务所说的「下单」）。
//
// 它**不发起任何网络请求** —— 虚拟支付的下单是服务端算好参数、
// 由小程序端 wx.requestVirtualPayment 拉起的。本方法返回的 SignData / PaySig /
// Signature 交给前端即可。
//
// 本方法保证：算签名用的字符串与返回的 SignData 是**同一个字节序列**，
// 从根上杜绝「签名串与下发串不一致」这一高频错误。
func (c *Client) BuildPaymentParams(req PrepayRequest) (*PrepayParams, error) {
	if err := validatePrepay(req); err != nil {
		return nil, err
	}

	qty := req.Quantity
	if qty <= 0 {
		qty = 1
	}
	mode := req.Mode
	if mode == "" {
		mode = ModeShortSeriesGoods
	}

	body := prepayBody{
		OfferID:              c.cfg.OfferID,
		BuyQuantity:          qty,
		Env:                  c.envInt(),
		CurrencyType:         "CNY",
		ProductID:            req.ProductID,
		GoodsPrice:           req.GoodsPrice,
		ActivitySellingPrice: req.ActivitySellingPrice,
		OutTradeNo:           req.OutTradeNo,
		Attach:               req.Attach,
	}

	raw, err := marshalNoHTMLEscape(body)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 序列化支付参数失败: %w", err)
	}

	// 唯一真身：这份字符串既用于签名，也原样下发。
	signData := string(raw)

	return &PrepayParams{
		SignData:  signData,
		PaySig:    CalcPaySig(c.appKey(), payMethodRequestVirtualPayment, signData),
		Signature: CalcSignature(req.SessionKey, signData),
		Mode:      mode,
	}, nil
}

func validatePrepay(req PrepayRequest) error {
	if req.SessionKey == "" {
		return errors.New("wechat_virtualpay_go: SessionKey 不能为空（需先 code2Session 换登录态）")
	}
	if req.OutTradeNo == "" {
		return errors.New("wechat_virtualpay_go: OutTradeNo 不能为空")
	}
	if !outTradeNoRe.MatchString(req.OutTradeNo) {
		return fmt.Errorf("wechat_virtualpay_go: OutTradeNo %q 非法，须为 8–32 位数字/字母/_-|*@", req.OutTradeNo)
	}
	if strings.HasPrefix(req.OutTradeNo, "_") {
		return errors.New("wechat_virtualpay_go: OutTradeNo 不能以下划线开头")
	}
	mode := req.Mode
	if mode == "" {
		mode = ModeShortSeriesGoods
	}
	switch mode {
	case ModeShortSeriesGoods:
		if req.ProductID == "" {
			return errors.New("wechat_virtualpay_go: 道具直购必须提供 ProductID")
		}
		if req.GoodsPrice <= 0 {
			return errors.New("wechat_virtualpay_go: 道具直购必须提供正数 GoodsPrice（单位：分）")
		}
	case ModeShortSeriesCoin:
		// 代币充值不需要 productId / goodsPrice。
	default:
		return fmt.Errorf("wechat_virtualpay_go: Mode %q 非法", mode)
	}

	if req.ActivitySellingPrice > 0 {
		// 优惠价不得低于道具价格的 40%。用整数比较避免浮点误差：
		// activity >= 0.4 * goods  <=>  activity*10 >= goods*4
		if req.ActivitySellingPrice*10 < req.GoodsPrice*4 {
			return errors.New("wechat_virtualpay_go: ActivitySellingPrice 不得低于 GoodsPrice 的 40%")
		}
	}
	return nil
}
