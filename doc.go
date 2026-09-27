// Package wechat_virtualpay_go 是微信小程序「虚拟支付」的服务端 Go SDK。
//
// 它覆盖官方全部 xpay 服务端接口、全部回调事件的验签与解析，
// 并且**零第三方依赖**（仅使用 Go 标准库）。
//
// # 这个包解决什么问题
//
// 微信小程序的虚拟支付不在「微信支付 APIv3」体系内，它走的是微信开放接口
// （access_token 鉴权）＋ 一套「双签名」机制。现有 Go 方案大多只做了一层薄封装，
// 且把最容易出错的部分（字符串一致性、iOS/Android 差异、查单补发、回调验签）留给了使用者。
//
// 本包的目标是：**把这些坑在库内部物理性地堵死**。
//
// # 两个签名
//
// 虚拟支付涉及两个 HMAC-SHA256 签名，本包分别实现（见 sign.go）：
//
//	pay_sig   = hex( HMAC-SHA256( AppKey,     uri + "&" + signData ) )
//	signature = hex( HMAC-SHA256( sessionKey, signData ) )
//
// 注意 pay_sig 会拼上 uri 而 signature 不会，两者不可混用同一个函数。
// uri 在拉起支付时固定为 "requestVirtualPayment"；调用服务端接口时是接口路径，
// 如 "/xpay/query_order"，且不带 query string。
//
// # 推送验签是另一套机制
//
// 消息推送的验签**不是** HMAC、也不使用 AppKey：它走微信标准「消息推送配置」通道，
// 用开发者在 MP 后台自填的 Token 做 SHA-1 排序签名（见 notify.go 的 Notifier）。
// 安全模式下报文还会用 EncodingAESKey 做 AES-256-CBC 加密。
//
// 这两套东西容易混，但它们毫无关系。
//
// # 字符串一致性
//
// pay_sig 是对**一段具体的 JSON 字符串**做 HMAC 得到的。服务端构建的字符串、
// 算签名用的字符串、下发给前端的字符串、微信最终校验的字符串，必须字节级一致。
// 为此本包内部一律通过 marshalNoHTMLEscape 序列化（见 json.go），
// 以避免 Go 标准库默认的 HTML 转义破坏签名。
//
// # 典型用法
//
// 服务端只负责「拼参数 + 算签名」，真正的支付由小程序端
// wx.requestVirtualPayment 拉起：
//
//	client, err := wechat_virtualpay_go.NewClient(wechat_virtualpay_go.Config{
//		AppID:   "wx...",
//		OfferID: "1234567890",
//		AppKey:  os.Getenv("VIRTUALPAY_APP_KEY"),
//		AppSecret: os.Getenv("VIRTUALPAY_APP_SECRET"), // 由本包获取并刷新 access_token
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	params, err := client.BuildVirtualPayment(wechat_virtualpay_go.VirtualPaymentRequest{
//		ProductID:  "prod_001",
//		GoodsPrice: 100, // 单位：分
//		OutTradeNo: "ORDER20260101001",
//		SessionKey: sessionKey, // 由 code2Session 换取
//	})
//	// 把 params.SignData / params.PaySig / params.Signature 交给前端
package wechat_virtualpay_go
