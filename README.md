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
| 下单参数构建 `BuildVirtualPayment` | ✅ 已实现 |
| 服务端接口 `/xpay/*`（官方 33 个） | ✅ 已实现 |
| 推送验签、AES 解密与事件解析（6 类事件） | ✅ 已实现 |
| `access_token` 获取与缓存 | ➖ 刻意不内置，见 `TokenProvider` |

⚠️ **尚未真机联调，且仓库当前不含测试代码。**

以上实现均依据官方文档。签名与加解密算法在开发阶段用微信文档正文里自带 `assert`
的样例做过验证（包括加密结果与官方密文逐字节一致、205→224 的 PKCS#7 填充长度），
但**这些测试没有被保留在仓库里**，而请求字段、必填项、错误码这些本来也不是单元
测试能覆盖的，必须在真实环境中确认。

官方文档本身存在若干自相矛盾之处，本包按更可靠的一方实现，并在源码注释中逐条
标注了存疑点与改法：

| 位置 | 矛盾所在 |
| --- | --- |
| 广告金 7 个 + `NotifyProvideGoods` + `PresentCurrency` | 请求体 `env` 注释写「仅作为签名校验」，但 query 里**没有 `pay_sig`** |
| `RefundOrder` / `GetComplaintList` | 「注意事项」写「使用用户态签名与支付签名」，参数表只有 `pay_sig` |
| `CurrencyPay` | 必填列**全部标「否」**（`openid`/`amount` 不可能非必填） |
| `QueryPunishmentReasons` | 写「请求体：无」，却又要 `pay_sig` |

**上生产前请务必用真实凭证完整跑一遍。**

## 安装

```bash
go get github.com/lessdome/wechat_virtualpay_go
```

导入路径与包名一致，都是 `wechat_virtualpay_go`。

## 快速开始

### 1. 实现 TokenProvider

`access_token` 的获取与缓存**刻意不内置**：缓存策略（内存 / 文件 / Redis）因部署形态
而异，内置一种等于替使用者做决定。

```go
type RedisToken struct {
    rdb            *redis.Client
    appID, secret  string
}

func (t *RedisToken) Token(ctx context.Context) (string, error) {
    // 1. 先读缓存；
    // 2. 没有或快过期则调用 /cgi-bin/token 刷新；
    // 3. 用分布式锁避免多实例并发刷新互相顶掉。
}
```

> ⚠️ 微信的 `access_token` **全局唯一且会互相顶掉**——多实例部署务必集中缓存，
> 否则 A 实例刷新会让 B 实例手上的 token 立即失效。

### 2. 创建客户端

```go
client, err := wechat_virtualpay_go.NewClient(wechat_virtualpay_go.Config{
    AppID:      "wx...",
    OfferID:    "1234567890",                        // 虚拟支付商户号
    AppKey:     os.Getenv("VIRTUALPAY_APP_KEY"),     // 现网密钥
    SandboxKey: os.Getenv("VIRTUALPAY_SANDBOX_KEY"), // 沙箱密钥
    Env:        wechat_virtualpay_go.EnvProduction,
    Tokens:     myTokenProvider,
})
```

`NewClient` 会校验配置并在缺项时报错。`Env` 同时决定**用哪个密钥**和**请求体里的
`env` 字段**（现网 `0` / 沙箱 `1`）——收敛到一处，避免「现网用了沙箱 Key」这类事故。

`HTTPClient` 可选。需要拦截请求、自定义日志或走代理时，注入一个带自定义
`http.RoundTripper` 的 client 即可——这是 Go 的惯用做法，本包不为此另设开关。

### 3. 构建支付参数（下单）

服务端**不发起支付请求**——它只负责拼参数、算签名，然后交给小程序端，由
`wx.requestVirtualPayment` 拉起支付。

```go
params, err := client.BuildVirtualPayment(wechat_virtualpay_go.VirtualPaymentRequest{
    ProductID:  "prod_001",
    GoodsPrice: 100, // 单位：分
    OutTradeNo: "ORDER20260101001",
    SessionKey: sessionKey, // 由 code2Session 换取
})
// 把 params.SignData / params.PaySig / params.Signature / params.Mode 交给前端
```

> ⚠️ `params.SignData` 必须**原样**传给前端，前端不得重新序列化，否则签名会失配——
> 这是最常见的 `-15006` 来源。

## 服务端接口一览

33 个接口全部挂在 `Client` 上。命名遵循统一约定：

> **方法名 = 官方接口英文名的大驼峰形式**，且每个方法所在的**文件名就是 `/xpay/`
> 后面的那一段**。例如 `QueryOrder` 在 `query_order.go`，对应 `/xpay/query_order`。

请求类型为 `<方法名>Request`，响应类型为 `*<方法名>Response`——**例外见「备注」列**。
请求体里的 `Env` 由 Client 自动填充，**无需也不应手动设置**。

### 代币相关

| 接口名称 | 方法 | 备注 |
| --- | --- | --- |
| 查询代币余额 | `QueryUserBalance` | **需 SessionKey** |
| 扣减代币 | `CurrencyPay` | **需 SessionKey** |
| 代币支付退款 | `CancelCurrencyPay` | **需 SessionKey** |
| 代币赠送 | `PresentCurrency` | |

### 道具相关

| 接口名称 | 方法 | 备注 |
| --- | --- | --- |
| 批量上传道具 | `StartUploadGoods` | 无响应体；一次一个道具 |
| 查询批量上传道具任务 | `QueryUploadGoods` | |
| 启动批量发布道具任务 | `StartPublishGoods` | 无响应体；发布后约 10 分钟生效 |
| 查询批量发布道具任务 | `QueryPublishGoods` | |

### 订单查询

| 接口名称 | 方法 | 备注 |
| --- | --- | --- |
| 查询创建的订单 | `QueryOrder` | **返回 `*Order`**，非 `*QueryOrderResponse` |
| 启动订单退款任务 | `RefundOrder` | 异步，需再查单确认 |
| 通知已发货完成 | `NotifyProvideGoods` | 无响应体 |
| 下载支付订单 | `StartDownloadOrder` | 异步，返回 `task_id` |
| 查询下载订单任务 | `QueryDownloadOrder` | |

### 账单下载

| 接口名称 | 方法 | 备注 |
| --- | --- | --- |
| 下载普通虚拟支付日账单 | `DownloadBill` | 轮询式，URL 有效期半小时 |
| 下载苹果 IAP 支付月账单 | `DownloadIOSBill` | |

### 资金管理

| 接口名称 | 方法 | 备注 |
| --- | --- | --- |
| 创建提现单 | `CreateWithdrawOrder` | 金额单位是**元** |
| 查询提现单 | `QueryWithdrawOrder` | |
| 查询商家账户可提现余额 | `QueryBizBalance` | 金额单位是**元** |

### 广告金

> 这 7 个接口的官方文档存在自相矛盾（见「当前状态」），本包暂按参数表实现（不签名）。

| 接口名称 | 方法 | 备注 |
| --- | --- | --- |
| 查询广告金充值账户 | `QueryTransferAccount` | |
| 查询广告金发放记录 | `QueryAdverFunds` | |
| 充值广告金 | `CreateFundsBill` | 幂等键为 `RequestID` |
| 绑定广告金充值账户 | `BindTransferAccount` | 无响应体 |
| 查询广告金充值记录 | `QueryFundsBill` | |
| 查询广告金回收记录 | `QueryRecoverBill` | |
| 下载广告金对应商户订单信息 | `DownloadAdverFundsOrder` | |

### 微信支付投诉、管控处理

| 接口名称 | 方法 | 备注 |
| --- | --- | --- |
| 获取投诉列表 | `GetComplaintList` | |
| 获取投诉详情 | `GetComplaintDetail` | |
| 获取协商历史 | `GetNegotiationHistory` | |
| 回复用户 | `ResponseComplaint` | 无响应体；图片需先 `UploadVPFile` |
| 完成投诉处理 | `CompleteComplaint` | 无响应体 |
| 上传媒体文件 | `UploadVPFile` | 返回 `file_id` |
| 获取微信支付投诉图片的签名头部 | `GetUploadFileSign` | |
| 商户被管控原因查询 | `QueryPunishmentReasons` | **无请求参数**，签名按空 body 计算 |

### 调用示例

```go
// 查单——「查单补发」兜底方案的基础
order, err := client.QueryOrder(ctx, wechat_virtualpay_go.QueryOrderRequest{
    OpenID:  "oUser123",
    OrderID: "ORDER20260101001",
})
if err != nil {
    return err
}
log.Printf("状态=%d 实付=%d 剩余可退=%d", order.Status, order.PaidFee, order.LeftFee)

// 退款——注意是异步的：启动成功 ≠ 退款完成
refund, err := client.RefundOrder(ctx, wechat_virtualpay_go.RefundOrderRequest{
    OpenID:        "oUser123",
    OrderID:       "ORDER20260101001",
    RefundOrderID: "REFUND20260101001",
    LeftFee:       order.LeftFee, // 先查单拿到剩余可退金额
    RefundFee:     100,
    RefundReason:  wechat_virtualpay_go.RefundReasonUserWill,
    RefundFrom:    wechat_virtualpay_go.RefundFromCustomerService,
})
// 之后需轮询 QueryOrder，直到 Status == OrderStatusRefundCompleted

// 用户态接口——注意多一个 sessionKey 参数
bal, err := client.QueryUserBalance(ctx, sessionKey, wechat_virtualpay_go.QueryUserBalanceRequest{
    OpenID: "oUser123",
    UserIP: "1.1.1.1",
})
```

## 接收推送

推送走的是微信**标准「消息推送配置」通道**（MP 后台 开发管理 → 消息推送配置），
验签机制与支付签名**毫无关系**：

```
明文模式   signature     = sha1( sort([Token, timestamp, nonce]).join("") )
安全模式   msg_signature = sha1( sort([Token, timestamp, nonce, Encrypt]).join("") )
```

`Token` 是你在 MP 后台自填的令牌，**不是 AppKey**；参数在 URL query 上。
安全模式下报文是 AES-256-CBC 加密的，需要 `EncodingAESKey`。

### 事件对照

| 推送类型 | `Event` 值 | 结构体 |
| --- | --- | --- |
| 道具发货推送 | `xpay_goods_deliver_notify` | `GoodsDeliverNotify` |
| 代币支付推送 | `xpay_coin_pay_notify` | `CoinPayNotify` |
| 退款推送 | `xpay_refund_notify` | `RefundNotify` |
| 用户投诉推送 | `xpay_complaint_notify` | `ComplaintNotify` |
| 微信支付风控事件通知 | `xpay_wxpay_callback_notify` | `WxpayCallbackNotify` |
| iOS 退款问询推送 | `xpay_subscribe_ios_refund_query_notify` | `IOSRefundQueryNotify` |

`Parse` 返回的 `Notification` 上，只有与 `Event` 对应的那个字段非 nil。

### 处理示例

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
    w.Header().Set("Content-Type", ct)
    w.Write(body)
    return
}

switch notif.Event {
case wechat_virtualpay_go.EventGoodsDeliver:
    g := notif.GoodsDeliver
    // 用 g.WeChatPayInfo.MchOrderNo 做幂等去重，发货…
}

body, contentType := wechat_virtualpay_go.Ack(notif.Format) // {"ErrCode":0,"ErrMsg":"success"}
```

### 三条铁律

1. **验签失败绝不发货。** 无论出了什么错，回失败应答让微信重试即可；回了成功但没
   发货，微信不再重试，这单就永久丢了。
2. **用平台单号做幂等。** 发货场景取 `WeChatPayInfo.MchOrderNo` 去重——微信会重试，
   同一单可能推多次。
3. **iOS 退款问询有 3 秒硬限制。** `xpay_subscribe_ios_refund_query_notify` 要求 3 秒
   内应答，Apple 会问询三次。这条路径上不要查库、不要调外部接口。

「推送」与「轮询 `QueryOrder`」建议**都实现**：`success` 回调可能丢失（用户异常
退出），推送也可能丢失，两者互补最可靠。

## 错误处理

所有接口失败时返回 `*APIError`，它带上了微信的 `errcode` / `errmsg` 与**原始响应体**：

```go
order, err := client.QueryOrder(ctx, req)
if err != nil {
    var apiErr *wechat_virtualpay_go.APIError
    if errors.As(err, &apiErr) {
        log.Printf("errcode=%d errmsg=%s hint=%s",
            apiErr.Code, apiErr.Message, apiErr.Hint())
        log.Printf("原始响应: %s", apiErr.Raw)
    }
    return err
}
```

`APIError.Hint()` 针对高频错误码给出排查方向，`IsCode` 用于判断特定错误：

```go
if wechat_virtualpay_go.IsCode(err, wechat_virtualpay_go.ErrCodeSessionKeyExpired) {
    // session_key 过期，让前端重新 wx.login
}
```

常见错误码（完整列表见 `errors.go`）：

| 错误码 | 含义 | 排查方向 |
| --- | --- | --- |
| `-15005` | 用户签名 `signature` 错误 | `session_key` 是否最新 |
| `-15006` | 支付签名 `pay_sig` 错误 | AppKey 与环境是否匹配、`signData` 是否字节级一致 |
| `-15007` | `session_key` 过期 | 重新 `wx.login` + `code2Session` |
| `-15011` | 现网版本 `env` 必须为 0 | 检查 `Config.Env` |
| `-15013` | `goodsPrice` 与后台不一致 | 道具价格是否已发布 |
| `-15016` | `signData` 格式有问题 | 是否混入了非协议字段 |

## 容易踩的坑

### 金额单位：几乎全是「分」，提现是「元」

本 SDK 里**绝大多数金额单位是分**（`GoodsPrice`、`OrderFee`、`PaidFee`、`RefundFee`、
代币 `Amount`…），**唯独提现相关的两个是「元」**：

- `CreateWithdrawOrderRequest.WithdrawAmount`（字符串，如 `"0.01"`）
- `BizBalance.Amount`（字符串）

按分填这两个字段会提现出 **100 倍金额**。

### 字符串一致性

`pay_sig` 是对**一段具体的 JSON 字符串**做 HMAC。服务端构建的串、算签名用的串、
下发给前端的串、微信校验的串，必须**字节级一致**。

Go 的 `json.Marshal` 默认会把 `<` `>` `&` 转义成 `<` / `>` / `&`。一旦参数
（道具名、`attach` 透传数据等）含有这些字符，转义后的字符串就与微信预期的原文
不一致，签名随即失败——而微信只会回 `-15006`「支付签名错误」，极难排查。

本包内部一律通过 `marshalNoHTMLEscape` 序列化来堵死这个问题，绝不直接使用
`json.Marshal`。

## 签名

虚拟支付涉及两个 HMAC-SHA256 签名，输出 64 位小写十六进制：

```
pay_sig   = hex( HMAC-SHA256( appKey,     uri + "&" + signData ) )
signature = hex( HMAC-SHA256( sessionKey, signData ) )
```

`pay_sig` 会拼上 `uri` 而 `signature` 不会，**两者不可共用一个函数**。拉起支付时
`uri` 固定为字符串 `"requestVirtualPayment"`（这是签名用的 method，不是 HTTP 路径）；
调用服务端接口时 `uri` 是接口路径，如 `/xpay/query_order`，且**不带** `?` 及其后的
query string。

> 推送回调的验签**不是** HMAC、也不使用 AppKey，见「接收推送」一节。两套机制毫无关系。

## iOS 与 Android 的差异

真实存在且必须知道，但**不影响下单参数**——signData 里没有 platform 字段，设备路由
由微信按客户端自动完成。

| | Android | iOS |
| --- | --- | --- |
| 通道 | 微信支付 | Apple IAP |
| 费率 | ~1% | ~12% |
| 结算 | T+3 | 45–60 天 |
| 退款 | 可主动调用退款接口 | **开发者无法主动退款**，只能接收退款问询 |

## License

[MIT](LICENSE)
