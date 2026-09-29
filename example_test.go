package wechat_virtualpay_go_test

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	wechat_virtualpay_go "github.com/lessdome/wechat_virtualpay_go"
)

// ExampleBuildPayment 是下单参数生成的最小可运行示例。
//
// 它有两个作用：一是把 godoc 里那段流程钉住——这里写错就编译不过，不会再出现
// 「文档里的示例照着抄却编译失败」；二是它在**外部测试包**里，只能用到导出的 API，
// 顺带证明了这套 API 从包外确实是可用的。
//
// 这里用固定的 outTradeNo 而不是 NewOutTradeNo()：后者带随机数，输出钉不住。
// 真实代码应当用 NewOutTradeNo() 或自己业务的单号（每单只能用一次）。
func ExampleBuildPayment() {
	p, err := wechat_virtualpay_go.BuildPayment("123", "appkey", "a-session-key",
		wechat_virtualpay_go.PaymentRequest{
			Mode:       wechat_virtualpay_go.ModeShortSeriesGoods,
			ProductID:  "testproductId",
			GoodsPrice: 10,
			Quantity:   1,
			OutTradeNo: "xxxxxx12",
			Attach:     "testdata",
		})
	if err != nil {
		panic(err)
	}
	fmt.Println(p.SignData)
	fmt.Println(p.Mode)
	// Output:
	// {"offerId":"123","buyQuantity":1,"env":0,"currencyType":"CNY","productId":"testproductId","goodsPrice":10,"outTradeNo":"xxxxxx12","attach":"testdata"}
	// short_series_goods
}

// ExampleParseNotification 演示在 HTTP handler 里接一条推送：验签 → 分派 → 应答。
//
// 注意两处：**验签失败绝不能发货**；而验签通过也**不等于**报文可信（报文体不参与
// 签名，见 notify_verify.go），所以发货前必须用 MchOrderNo 做幂等。
func ExampleParseNotification() {
	// 真实代码里 token 从环境变量或配置取，r 是 handler 收到的 *http.Request。
	const token = "AAAAA"
	body := `{"ToUserName":"gh_x","FromUserName":"oUser","CreateTime":1700000000,` +
		`"MsgType":"event","Event":"xpay_goods_deliver_notify",` +
		`"WeChatPayInfo":{"MchOrderNo":"ORDER1"},"GoodsInfo":{"ProductId":"p1"}}`
	// 参数取自官方《消息推送》页的样例一。
	r := signedRequest(token, "1714036504", "1514711492", body)

	notif, err := wechat_virtualpay_go.ParseNotification(token, r)
	if err != nil {
		// 回失败应答让微信重试——绝不要在这里发货。
		fmt.Println("ack err:", err)
		return
	}

	ack := wechat_virtualpay_go.Ack{ErrCode: 0, ErrMsg: "success"}
	switch notif.Event {
	case wechat_virtualpay_go.EventGoodsDeliver:
		// 幂等键是平台单号 WeChatPayInfo.MchOrderNo；它可能为 nil，取值前先判空。
		if notif.GoodsDeliver.WeChatPayInfo != nil {
			fmt.Println("deliver:", notif.GoodsDeliver.WeChatPayInfo.MchOrderNo)
		}
	}
	fmt.Printf("ack: %d %s\n", ack.ErrCode, ack.ErrMsg)
	// Output:
	// deliver: ORDER1
	// ack: 0 success
}

// signedRequest 造一条带合法验签参数的推送请求。
//
// 这里独立算一遍 sha1（不调包里的私有函数），免得用被测代码给自己签发通行证。
func signedRequest(token, ts, nonce, body string) *http.Request {
	parts := []string{token, ts, nonce}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	q := url.Values{
		"timestamp": {ts},
		"nonce":     {nonce},
		"signature": {hex.EncodeToString(sum[:])},
	}
	return httptest.NewRequest(http.MethodPost, "/notify?"+q.Encode(), strings.NewReader(body))
}
