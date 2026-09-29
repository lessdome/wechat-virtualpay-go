package wechat_virtualpay_go

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// 本文件实现「虚拟支付」下单参数的生成——道具直购与代币充值两种模式共用一套。
//
// 官方 2.1 时序图里这条流程是：服务端把参数拼成一个字符串、算两个签名，交给小程序端
// 由 wx.requestVirtualPayment 拉起支付。它**不发起任何网络请求**。
//
// 两种模式的差别只有两处（出自官方《wx.requestVirtualPayment》的 signData 结构表）：
// mode 不同，以及 productId / goodsPrice / activitySellingPrice 三个字段「仅
// mode=short_series_goods 时需要必填」。其余六个字段完全相同，所以合成一个方法。

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
	// ModeShortSeriesCoin 代币充值。
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

// PaymentRequest 是一次下单的**订单参数**。凭据（offerID / appKey / sessionKey）不走
// 这里，是 BuildPayment 的参数。
//
// 字段与官方 signData 结构表一一对应。表里共 9 个字段，其中两个不按单变化，由本包
// 从凭据固定：
//
//	offerId        ← 函数参数 offerID（商户级）
//	currencyType   ← 固定 CNY（官方目前只支持这一种）
//
// 剩下 7 个按单变化的都在下面（含环境 env）。
type PaymentRequest struct {
	// Mode 支付类型。必填——选错模式会走错流程（买道具还是充代币，金额的含义完全不同），
	// 所以本包不替你猜默认值。
	//
	// ⚠️ 存疑：官方**没有给代币充值的 signData 示例**（客户端页的示例代码只有道具直购
	// 那一条），所以「代币不带 productId / goodsPrice」是从字段表的措辞推出来的。
	// 另有一处反证：客户端错误码 -15018 写「代币或者道具 productId 审核不通过」，
	// 字面上把两者连在一起。真机联调时要确认。
	Mode PaymentMode
	// Env 调用环境（signData 的 env）：**0=现网（默认）/ 1=沙箱**，不填即现网。
	//
	// ⚠️ **AppKey 与环境绑死**：env=0 必须配现网 AppKey、env=1 必须配沙箱 AppKey，两把
	// 混用微信侧会判签名错误。本包分不出传进来的是哪一把（都是普通字符串），这条只能靠
	// 调用方自己保证。
	Env int
	// ProductID 道具 ID。**仅道具直购需要**；代币充值传了会报错。
	ProductID string
	// GoodsPrice 道具单价，单位**分**。**仅道具直购需要**；代币充值传了会报错。
	// 微信用它校验与后台配置的道具价格是否一致。
	GoodsPrice int64
	// Quantity 购买数量（signData 的 buyQuantity）。<=0 时按 1 处理。
	//
	// 道具直购是买几个道具；代币充值是充多少个代币——代币的单价由后台的代币配置
	// 决定，官方 signData 里没有价格字段。
	Quantity int64
	// ActivitySellingPrice 优惠价，单位**分**。可选，**仅道具直购**。
	// 传了它就是实际下单价格。官方另外要求它**不得低于 GoodsPrice 的 40%**——
	// 越界会在微信侧才被拒（客户端 -15016、服务端 268490002），所以这里先拦下来。
	ActivitySellingPrice int64
	// OutTradeNo 业务订单号（signData 的 outTradeNo）。必填。
	// 8–32 字符，只能由数字、大小写字母、_-|*@ 组成，不能以 _ 开头，且每单只能用一次。
	// 懒得拼就用本包的 NewOutTradeNo()。
	OutTradeNo string
	// Attach 透传数据（signData 的 attach）。必填。
	//
	// 官方 signData 字段表的必填列标「是」，且不分模式——代币充值也要传，传空
	// BuildPayment 会直接报错。
	//
	// 官方对它的说明是「发货通知时会透传给开发者」——代币充值没有发货环节，它究竟
	// 从哪儿、什么时候回来，文档没写。
	Attach string
}

// paymentSignData 是 signData 的序列化结构。
//
// 字段顺序与官方字段表一致。三个道具专有的字段带 omitempty，所以代币充值天然不会
// 带上它们——两种模式共用这一个结构体。
type paymentSignData struct {
	OfferID              string `json:"offerId"`
	BuyQuantity          int64  `json:"buyQuantity"`
	Env                  int    `json:"env"`
	CurrencyType         string `json:"currencyType"`
	ProductID            string `json:"productId,omitempty"`
	GoodsPrice           int64  `json:"goodsPrice,omitempty"`
	ActivitySellingPrice int64  `json:"activitySellingPrice,omitempty"`
	OutTradeNo           string `json:"outTradeNo"`
	Attach               string `json:"attach"`
}

// outTradeNoRe 是官方对 outTradeNo 的字符与长度要求。
var outTradeNoRe = regexp.MustCompile(`^[0-9A-Za-z_|*@-]{8,32}$`)

// BuildPayment 生成一次虚拟支付的下单参数（道具直购与代币充值共用）。
//
// 入参：
//
//	offerID     商户号（signData 的 offerId），在虚拟支付商户后台查看。必填。
//	appKey      商家密钥，用来算 pay_sig。必填。
//	sessionKey  用户会话密钥，用来算 signature；由本包的 Code2Session() 用前端 wx.login 的
//	            code 换来。必填。两点要留意：它是**会话级**凭据、不是每单一个——同一次登录态
//	            可以下多笔单；它会**过期**，过期的表现是服务端报 268490009、客户端报 -15007，
//	            届时让前端重新 wx.login 再换一把。
//	req         订单参数（环境、模式、道具/数量、价格、单号…），字段与官方 signData 字段
//	            表一一对应，见 PaymentRequest。⚠️ 它的 Env 与 appKey **必须配套**：env=0
//	            配现网 AppKey、env=1 配沙箱 AppKey，混用会得到签名错误码。
//
// 本函数**不发起任何网络请求**。典型用法（完整可编译可运行的版本见 ExampleBuildPayment）：
//
//	outTradeNo, err := wechat_virtualpay_go.NewOutTradeNo() // 也可用自己业务的单号
//	if err != nil {
//		return err
//	}
//	p, err := wechat_virtualpay_go.BuildPayment(offerID, appKey, sess.SessionKey,
//		wechat_virtualpay_go.PaymentRequest{
//			Mode:       wechat_virtualpay_go.ModeShortSeriesGoods, // 或 ModeShortSeriesCoin
//			ProductID:  "prod_001",                                // 仅道具直购
//			GoodsPrice: 100,                                       // 单位：分，仅道具直购
//			Quantity:   1,
//			OutTradeNo: outTradeNo,
//			Attach:     "自定义透传数据",
//		})
//	if err != nil {
//		return err
//	}
//
// 返回后把 p.SignData / p.PaySig / p.Signature / p.Mode 交给前端，传给
// wx.requestVirtualPayment。**SignData 必须原样传**，前端不得重新序列化。
//
// 注意客户端有个前提：wx.requestVirtualPayment 需要基础库 >= 2.19.2。
func BuildPayment(offerID, appKey, sessionKey string, req PaymentRequest) (*VirtualPaymentParams, error) {
	if offerID == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: offerID 不能为空")
	}
	// env 必须排在 appKey 前面：appKey 的报错文案里要点出「该配现网还是沙箱那把」，
	// 那是按 env 取的（见 checkAppKey）。
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if sessionKey == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: sessionKey 不能为空（用 Code2Session 换取）")
	}
	if err := checkOutTradeNo(req.OutTradeNo); err != nil {
		return nil, err
	}
	// Attach 在官方字段表里是必填、且不分模式。不拦的话会拿到一份 attach 为空串的
	// signData 与自洽签名，本地无声通过，直到微信侧才以参数错误拒掉。
	if req.Attach == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: Attach 不能为空（官方 signData 字段表标为必填，发货通知会把它透传回来）")
	}

	switch req.Mode {
	case ModeShortSeriesGoods:
		if req.ProductID == "" {
			return nil, fmt.Errorf("wechat_virtualpay_go: 道具直购必须提供 ProductID")
		}
		if req.GoodsPrice <= 0 {
			return nil, fmt.Errorf("wechat_virtualpay_go: 道具直购必须提供正数 GoodsPrice（单位：分）")
		}
		// 优惠价不得低于道具价格的 40%。用整数比较避免浮点误差：
		// activity >= 0.4*goods  <=>  activity*10 >= goods*4
		if req.ActivitySellingPrice > 0 && req.ActivitySellingPrice*10 < req.GoodsPrice*4 {
			return nil, fmt.Errorf("wechat_virtualpay_go: ActivitySellingPrice（%d 分）不得低于 GoodsPrice（%d 分）的 40%%",
				req.ActivitySellingPrice, req.GoodsPrice)
		}
	case ModeShortSeriesCoin:
		// 代币充值不带这三个道具字段。传了就是误用——静默忽略会让人以为价格生效了，
		// 而金额是这里最不能含糊的东西，所以直接报错。
		if req.ProductID != "" || req.GoodsPrice != 0 || req.ActivitySellingPrice != 0 {
			return nil, fmt.Errorf("wechat_virtualpay_go: 代币充值不该传 ProductID / GoodsPrice / ActivitySellingPrice（它们是道具直购专用的）")
		}
	default:
		return nil, fmt.Errorf("wechat_virtualpay_go: Mode %q 非法，应为 %q 或 %q",
			req.Mode, ModeShortSeriesGoods, ModeShortSeriesCoin)
	}

	qty := req.Quantity
	if qty <= 0 {
		qty = 1
	}

	raw, err := json.Marshal(paymentSignData{
		OfferID:              offerID,
		BuyQuantity:          qty,
		Env:                  req.Env, // 零值即现网；沙箱必须同时换沙箱 AppKey
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

	return finishPayment(appKey, sessionKey, req.Mode, string(raw)), nil
}

// finishPayment 把已经序列化好的 signData 补成完整参数：算两个签名、填 Mode。
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
