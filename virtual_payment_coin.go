package wechat_virtualpay_go

import (
	"encoding/json"
	"fmt"
)

// 本文件实现「代币充值」的下单参数生成。
//
// 它与道具直购（virtual_payment.go）的区别只有两处，都出自官方《wx.requestVirtualPayment》
// 的 signData 结构表：
//
//   - mode 是 short_series_coin，不是 short_series_goods；
//   - **不带** productId / goodsPrice / activitySellingPrice——官方在那两行明确标注
//     「该字段仅 mode=short_series_goods 时需要必填」。
//
// 其余六个字段（offerId、buyQuantity、env、currencyType、outTradeNo、attach）与道具
// 完全相同，字段表的相对顺序也照旧。
//
// 为什么代币不需要任何「代币标识」：服务端「代币相关」那几个接口的请求体里都没有代币
// ID——连查询代币余额都只要 openid，说明一个小程序只有一种代币。所以 signData 里没有
// 可指的东西，buyQuantity 就是充多少个，单价按后台的代币配置算。
//
// ⚠️ 存疑：文档**没有给代币充值的 signData 示例**（客户端页的示例代码只有道具直购
// 那一条），所以上面「代币不带 productId / goodsPrice」是从字段表的措辞**推**出来的。
// 另有一处反证：客户端错误码 -15018 写的是「代币或者道具 productId 审核不通过」，
// 字面上把 productId 和代币连在了一起。真机联调时要确认这一条。

// CoinPaymentRequest 是代币充值一次下单所需的输入。
type CoinPaymentRequest struct {
	// Quantity 充值数量（signData 的 buyQuantity）。<=0 时按 1 处理。
	//
	// 这里没有价格字段：官方 signData 里就没有——单价由后台的代币配置决定。
	Quantity int64
	// OutTradeNo 业务订单号（signData 的 outTradeNo）。必填。
	// 规则同道具直购：8–32 字符，只能由数字、大小写字母、_-|*@ 组成，不能以 _ 开头，
	// 且每单只能用一次。懒得拼就用本包的 NewOutTradeNo()。
	OutTradeNo string
	// Attach 透传数据（signData 的 attach）。必填。
	//
	// 官方对它的说明是「发货通知时会透传给开发者」——代币充值没有发货环节，它究竟
	// 从哪儿、什么时候回来，文档没写。
	Attach string
}

// coinSignData 是代币充值的 signData 结构。
//
// 字段顺序沿用官方字段表里的相对次序（去掉了道具专有的那几个）。
type coinSignData struct {
	OfferID      string `json:"offerId"`
	BuyQuantity  int64  `json:"buyQuantity"`
	Env          int    `json:"env"`
	CurrencyType string `json:"currencyType"`
	OutTradeNo   string `json:"outTradeNo"`
	Attach       string `json:"attach"`
}

// BuildCoinPayment 生成代币充值的下单参数。
//
// 三个凭据（offerID / appKey / sessionKey）的说明与 BuildGoodsPayment 完全一致：
// sessionKey 用 Code2Session() 换，是会话级凭据、会过期。本函数不联网。
//
//	sess, err := wechat_virtualpay_go.Code2Session(ctx, appID, appSecret, code)
//	if err != nil {
//		return err
//	}
//	p, err := wechat_virtualpay_go.BuildCoinPayment(offerID, appKey, sess.SessionKey,
//		wechat_virtualpay_go.CoinPaymentRequest{
//			Quantity:   100, // 充 100 个代币
//			OutTradeNo: wechat_virtualpay_go.NewOutTradeNo(),
//			Attach:     "自定义透传数据",
//		})
//
// 返回后把 p.SignData / p.PaySig / p.Signature / p.Mode 交给前端，
// 传给 wx.requestVirtualPayment。**SignData 必须原样传**，前端不得重新序列化。
//
// 注意客户端有个前提：wx.requestVirtualPayment 需要基础库 >= 2.19.2。
func BuildCoinPayment(offerID, appKey, sessionKey string, req CoinPaymentRequest) (*VirtualPaymentParams, error) {
	if offerID == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: offerID 不能为空")
	}
	if appKey == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: appKey 不能为空")
	}
	if sessionKey == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: sessionKey 不能为空（用 Code2Session 换取）")
	}
	if err := checkOutTradeNo(req.OutTradeNo); err != nil {
		return nil, err
	}

	qty := req.Quantity
	if qty <= 0 {
		qty = 1
	}

	raw, err := json.Marshal(coinSignData{
		OfferID:      offerID,
		BuyQuantity:  qty,
		Env:          0, // 本包只支持现网环境
		CurrencyType: "CNY",
		OutTradeNo:   req.OutTradeNo,
		Attach:       req.Attach,
	})
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 序列化下单参数失败: %w", err)
	}

	return finishPayment(appKey, sessionKey, ModeShortSeriesCoin, string(raw)), nil
}
