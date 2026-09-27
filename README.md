# wechat-virtualpay-go

微信小程序「虚拟支付」**全流程**的服务端 Go SDK。**零第三方依赖**，只用标准库。

## 为什么会有这个包

微信小程序的虚拟支付不在「微信支付 APIv3」体系内，它走微信开放接口（`access_token` 鉴权）＋ 一套「双签名」机制，而整条链路横跨多个环节：

```
下单签名 → 拉起支付 → 发货推送 / 查单补发 → 退款 → 账单对账与资金提现
```

现有的 Go 方案大多只覆盖了其中最窄的一段（拼下单参数），把最容易出错的部分——字符串一致性、iOS/Android 差异、查单补发、推送验签与解密——留给了使用者。

这个包的目标是**覆盖整条链路**，并把这些坑在库内部物理性地堵死。

## 当前状态

⚠️ **开发中，尚未完成。**

| 能力 | 状态 |
| --- | --- |
| 支付签名 `pay_sig`、用户态签名 `signature` | ✅ 已实现 |
| 一致性 JSON 序列化（防 HTML 转义破坏签名） | ✅ 已实现 |
| 下单参数构建 `BuildPaymentParams` | ✅ 已实现 |
| 服务端接口调用（`/xpay/*`） | ❌ 未实现 |
| 推送验签、AES 解密与事件解析 | ❌ 未实现 |
| `access_token` 获取与缓存 | ❌ 不内置，见 `TokenProvider` |

**在「未实现」的项落地前，请勿在支付链路依赖本包。**

## 安装

```bash
go get github.com/lessdome/wechat-virtualpay-go
```

## 快速开始

服务端**不发起支付请求**——它只负责拼参数、算签名，然后把结果交给小程序端，由 `wx.requestVirtualPayment` 拉起支付。

```go
client, err := virtualpay.NewClient(virtualpay.Config{
    AppID:   "wx...",
    OfferID: "1234567890",
    AppKey:  os.Getenv("VIRTUALPAY_APP_KEY"),
    Env:     virtualpay.EnvProduction,
    Tokens:  myTokenProvider, // 自行实现 TokenProvider
})
if err != nil {
    log.Fatal(err)
}

params, err := client.BuildPaymentParams(virtualpay.PrepayRequest{
    ProductID:  "prod_001",
    GoodsPrice: 100, // 单位：分
    OutTradeNo: "ORDER20260101001",
    SessionKey: sessionKey, // 由 code2Session 换取
})
// 把 params.SignData / params.PaySig / params.Signature 交给前端
```

`params.SignData` 必须**原样**传给前端，前端不得重新序列化，否则签名会失配——这是最常见的 `-15006` 来源。

`access_token` 的获取与缓存刻意不内置：缓存策略（内存 / 文件 / Redis）因部署形态而异，内置一种等于替使用者做决定。实现 `TokenProvider` 接口即可。注意微信的 `access_token` 全局唯一且会互相顶掉，**多实例部署务必集中缓存**。

## 签名

虚拟支付涉及两个 HMAC-SHA256 签名，输出 64 位小写十六进制：

```
pay_sig   = hex( HMAC-SHA256( appKey,     uri + "&" + signData ) )
signature = hex( HMAC-SHA256( sessionKey, signData ) )
```

`pay_sig` 会拼上 `uri` 而 `signature` 不会，**两者不可共用一个函数**。拉起支付时 `uri` 固定为字符串 `"requestVirtualPayment"`（这是签名用的 method，不是 HTTP 路径）；调用服务端接口时 `uri` 是接口路径，如 `/xpay/query_order`，且不带 `?` 及其后的 query string。

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
