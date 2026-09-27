// Package virtualpay 是微信小程序「虚拟支付」的服务端 Go SDK。
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
// # 三个签名
//
// 虚拟支付涉及三个 HMAC-SHA256 签名，本包分别实现（见 sign.go）：
//
//	pay_sig       = hex( HMAC-SHA256( AppKey,     method + "&" + signData ) )
//	signature     = hex( HMAC-SHA256( sessionKey, signData ) )
//	pay_event_sig = hex( HMAC-SHA256( AppKey,     event  + "&" + payload  ) )
//
// 注意 pay_sig 会拼上 method 而 signature 不会，两者不可混用同一个函数。
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
//	client, err := virtualpay.NewClient(virtualpay.Config{
//		AppID:   "wx...",
//		OfferID: "1234567890",
//		AppKey:  os.Getenv("VIRTUALPAY_APP_KEY"),
//		Env:     virtualpay.EnvProduction,
//		Tokens:  myTokenProvider, // 见 TokenProvider
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	params, err := client.BuildPaymentParams(virtualpay.PrepayRequest{
//		ProductID:  "prod_001",
//		GoodsPrice: 100, // 单位：分
//		OutTradeNo: "ORDER20260101001",
//		SessionKey: sessionKey, // 由 code2Session 换取
//	})
//	// 把 params.SignData / params.PaySig / params.Signature 交给前端
package virtualpay
