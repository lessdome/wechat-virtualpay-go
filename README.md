# wechat_virtualpay_go

微信小程序「虚拟支付」**全流程**的服务端 Go SDK。**零第三方依赖**，只用标准库。

## 为什么会有这个包

微信小程序的虚拟支付不在「微信支付 APIv3」体系内，它走微信开放接口（`access_token` 鉴权）＋ 一套「双签名」机制，而整条链路横跨多个环节：

```
下单签名 → 拉起支付 → 发货推送 / 查单补发 → 退款 → 账单对账与资金提现
```

现有的 Go 方案大多只覆盖了其中最窄的一段（拼下单参数），把最容易出错的部分——字符串一致性、iOS/Android 差异、查单补发、推送验签与解密——留给了使用者。

这个包的目标是**覆盖整条链路**，并把这些坑在库内部物理性地堵死。

## 当前状态

官方 **33 个服务端接口**与 **6 类推送事件**已全部实现。

| 能力 | 状态 |
| --- | --- |
| 支付签名 `pay_sig`、用户态签名 `signature` | ✅ 已实现 |
| 一致性 JSON 序列化（防 HTML 转义破坏签名） | ✅ 已实现 |
| 下单参数构建 `BuildPaymentParams` | ✅ 已实现 |
| 服务端接口 `/xpay/*`（官方 33 个） | ✅ 已实现 |
| 推送验签、AES 解密与事件解析（6 类事件） | ✅ 已实现 |
| `access_token` 获取与缓存 | ➖ 刻意不内置，见 `TokenProvider` |

⚠️ **尚未完成真机联调。** 以上实现均依据官方文档，签名与加解密算法已用微信文档
正文里的样例做了**逐字节验证**（见各 `*_test.go`），但请求字段、必填项、错误码
这些仍需在真实环境中确认。官方文档本身存在若干自相矛盾之处（9 个商家级接口的
`pay_sig`、`refund_order` 的用户态签名等），本包按更可靠的一方实现并在源码注释中
逐条标注。**上生产前请务必用真实凭证跑一遍。**

## 安装

```bash
go get github.com/lessdome/wechat_virtualpay_go
```

## 快速开始

服务端**不发起支付请求**——它只负责拼参数、算签名，然后把结果交给小程序端，由 `wx.requestVirtualPayment` 拉起支付。

```go
client, err := wechat_virtualpay_go.NewClient(wechat_virtualpay_go.Config{
    AppID:   "wx...",
    OfferID: "1234567890",
    AppKey:  os.Getenv("VIRTUALPAY_APP_KEY"),
    Env:     wechat_virtualpay_go.EnvProduction,
    Tokens:  myTokenProvider, // 自行实现 TokenProvider
})
if err != nil {
    log.Fatal(err)
}

params, err := client.BuildPaymentParams(wechat_virtualpay_go.PrepayRequest{
    ProductID:  "prod_001",
    GoodsPrice: 100, // 单位：分
    OutTradeNo: "ORDER20260101001",
    SessionKey: sessionKey, // 由 code2Session 换取
})
// 把 params.SignData / params.PaySig / params.Signature 交给前端
```

`params.SignData` 必须**原样**传给前端，前端不得重新序列化，否则签名会失配——这是最常见的 `-15006` 来源。

`access_token` 的获取与缓存刻意不内置：缓存策略（内存 / 文件 / Redis）因部署形态而异，内置一种等于替使用者做决定。实现 `TokenProvider` 接口即可。注意微信的 `access_token` 全局唯一且会互相顶掉，**多实例部署务必集中缓存**。

## 调用服务端接口

官方 33 个 `/xpay/*` 接口都挂在 `Client` 上，方法名与接口语义一一对应：

```go
order, err := client.QueryOrder(ctx, wechat_virtualpay_go.QueryOrderRequest{
    OpenID:  "oUser123",
    OrderID: "ORDER20260101001",
})

resp, err := client.RefundOrder(ctx, wechat_virtualpay_go.RefundOrderRequest{
    OpenID:        "oUser123",
    OrderID:       "ORDER20260101001",
    RefundOrderID: "REFUND20260101001",
    LeftFee:       order.LeftFee, // 提示：先查单拿到剩余可退金额
    RefundFee:     100,
    RefundReason:  wechat_virtualpay_go.RefundReasonUserWill,
    RefundFrom:    wechat_virtualpay_go.RefundFromCustomerService,
})
```

请求体里的 `Env` 由 `Client` 按 `Config.Env` 自动填充，**不需要也不应该手动设置**——
这是为了从根上堵住「现网用了沙箱环境」这类事故。

三个**用户态接口**（`QueryUserBalance`、`CurrencyPay`、`CancelCurrencyPay`）额外需要
`SessionKey`：

```go
bal, err := client.QueryUserBalance(ctx, sessionKey, wechat_virtualpay_go.QueryUserBalanceRequest{
    OpenID: "oUser123",
    UserIP: "1.1.1.1",
})
```

退款是**异步**的：`RefundOrder` 返回成功只表示任务已启动，需再用 `QueryOrder`
查到 `OrderStatusRefundCompleted` 才算最终成功。iOS 订单无法主动退款——Apple IAP
由用户向 App Store 申请，开发者只能被动接收退款问询推送。

### 金额单位

**几乎全部是「分」**，唯一的例外是提现相关：`CreateWithdrawOrder.WithdrawAmount`
与 `BizBalance.Amount` 用「元」（字符串）。按分填会提现出 100 倍金额。

## 签名

虚拟支付涉及两个 HMAC-SHA256 签名，输出 64 位小写十六进制：

```
pay_sig   = hex( HMAC-SHA256( appKey,     uri + "&" + signData ) )
signature = hex( HMAC-SHA256( sessionKey, signData ) )
```

`pay_sig` 会拼上 `uri` 而 `signature` 不会，**两者不可共用一个函数**。拉起支付时 `uri` 固定为字符串 `"requestVirtualPayment"`（这是签名用的 method，不是 HTTP 路径）；调用服务端接口时 `uri` 是接口路径，如 `/xpay/query_order`，且不带 `?` 及其后的 query string。

## 接收推送

推送走的是微信**标准「消息推送配置」通道**（MP 后台 开发管理 → 消息推送配置），
验签机制与支付签名**毫无关系**：

```
明文模式   signature     = sha1( sort([Token, timestamp, nonce]).join("") )
安全模式   msg_signature = sha1( sort([Token, timestamp, nonce, Encrypt]).join("") )
```

`Token` 是你在 MP 后台自填的令牌，**不是 AppKey**；参数在 URL query 上。
安全模式下报文是 AES-256-CBC 加密的，需要 `EncodingAESKey`。

```go
notifier, err := wechat_virtualpay_go.NewNotifier(wechat_virtualpay_go.NotifyConfig{
    AppID:          "wx...",
    Token:          os.Getenv("VIRTUALPAY_NOTIFY_TOKEN"),
    EncodingAESKey: os.Getenv("VIRTUALPAY_AES_KEY"), // 留空则只支持明文模式
})
if err != nil {
    log.Fatal(err)
}

// 在你的 HTTP handler 里：
notif, err := notifier.ParseHTTP(r)
if err != nil {
    // 验签/解密失败：回失败应答让微信重试，**绝不要发货**
    body, ct := wechat_virtualpay_go.AckError(notif.Format, 1, err.Error())
    // ...写回 body / ct
    return
}

switch notif.Event {
case wechat_virtualpay_go.EventGoodsDeliver:
    g := notif.GoodsDeliver
    // 用 g.WeChatPayInfo.MchOrderNo 做幂等去重，发货…
}

body, contentType := wechat_virtualpay_go.Ack(notif.Format) // {"ErrCode":0,"ErrMsg":"success"}
```

三个必须注意的点：

1. **验签失败绝不发货。** 无论出了什么错，回失败应答让微信重试即可；回了成功但没发货，微信不再重试，这单就永久丢了。
2. **用平台单号做幂等。** 发货场景取 `WeChatPayInfo.MchOrderNo`（平台单号）去重——微信会重试，同一单可能推多次。
3. **iOS 退款问询有 3 秒硬限制。** `xpay_subscribe_ios_refund_query_notify` 要求 3 秒内应答，Apple 会问询三次。这条路径上不要查库、不要调外部接口。

「推送」与「轮询 `QueryOrder`」建议都实现：`success` 回调可能丢失（用户异常退出），
推送也可能丢失，两者互补最可靠。

## 一个容易踩的坑

`pay_sig` 是对**一段具体的 JSON 字符串**做 HMAC。服务端构建的串、算签名用的串、下发给前端的串、微信校验的串，必须**字节级一致**。

Go 的 `json.Marshal` 默认会把 `<` `>` `&` 转义成 `<` / `>` / `&`。一旦参数（道具名、`attach` 透传数据等）含有这些字符，转义后的字符串就与微信预期的原文不一致，签名随即失败——而微信只会回 `-15006`「支付签名错误」，极难排查。

本包内部一律通过 `marshalNoHTMLEscape` 序列化来堵死这个问题，绝不直接使用 `json.Marshal`。

## iOS 与 Android 的差异

真实存在且必须知道，但**不影响下单参数**——signData 里没有 platform 字段，设备路由由微信按客户端自动完成（Android/鸿蒙/Windows 走微信支付，iOS 走 Apple 支付）。

| | Android | iOS |
| --- | --- | --- |
| 通道 | 微信支付 | Apple IAP |
| 费率 | ~1% | ~12% |
| 结算 | T+3 | 45–60 天 |
| 退款 | 可主动调用退款接口 | 开发者无法主动退款 |

## License

[MIT](LICENSE)
