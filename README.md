# wechat_virtualpay_go

微信小程序「虚拟支付」的服务端 Go SDK。**零第三方依赖**，只用标准库。

> ⚠️ 本仓库**正在按清单逐项重新实现**，所以 README 分两段：
>
> - 前面是**当前已实现、可以直接用**的能力；
> - 最后一段「[尚未实现（规划中）](#尚未实现规划中)」是目标形态，**其中提到的
>   接口/类型现在都不存在**，照着写会编译不过。

## 当前状态

| 能力 | 状态 |
| --- | --- |
| 支付签名 `pay_sig`、用户态签名 `signature` | ✅ `CalcPaySig` / `CalcSignature` |
| 用 `wx.login` 的 code 换用户登录态 | ✅ `Code2Session` |
| 下单参数构建（道具直购 + 代币充值） | ✅ `BuildPayment` |
| 业务订单号生成 | ✅ `NewOutTradeNo` |
| 推送解析（6 类事件，JSON + 明文模式） | ✅ `ParseNotification` |
| 推送应答的数据模型 | ✅ `Ack` / `IOSRefundQueryResponse` |
| 服务端接口 `/xpay/*`（官方 33 个） | ❌ 尚未实现 |
| `access_token` 获取与缓存 | ❌ 尚未实现 |
| 错误码表（`errors.go`） | ❌ 尚未实现 |

**尚未真机联调。** 支付签名与推送验签都用官方文档正文里自带样例做过验证
（见 `sign_test.go`、`notify_verify_test.go`），但**请求字段、必填项、错误码这些
不是单元测试能覆盖的**，上生产前请务必用真实凭证完整跑一遍。

## 安装

```bash
go get github.com/lessdome/wechat_virtualpay_go
```

导入路径与包名一致，都是 `wechat_virtualpay_go`。

## 1. 换取用户登录态

这是整条链路的**第一步**：小程序端 `wx.login()` 拿到 code，交给服务端换
`session_key`。下单签名与用户态接口都要用它。

```go
sess, err := wechat_virtualpay_go.Code2Session(ctx, appID, appSecret, code)
// sess.OpenID / sess.SessionKey / sess.UnionID
```

code **有效期五分钟、且只能用一次**；换来的 `SessionKey` 也会过期（服务端报
`268490009`、客户端报 `-15007`），过期就让前端重新 `wx.login`。

注意它的鉴权方式与 `/xpay/*` 那套**完全不同**：这里直接用 appid + secret 换，
不需要 `access_token`。

## 2. 生成下单参数

服务端**不发起支付请求**——它只负责拼参数、算签名，然后交给小程序端，由
`wx.requestVirtualPayment` 拉起支付。

```go
outTradeNo, err := wechat_virtualpay_go.NewOutTradeNo() // 也可用自己业务的单号
if err != nil {
    return err
}
p, err := wechat_virtualpay_go.BuildPayment(offerID, appKey, sess.SessionKey,
    wechat_virtualpay_go.PaymentRequest{
        Mode:       wechat_virtualpay_go.ModeShortSeriesGoods, // 或 ModeShortSeriesCoin
        ProductID:  "prod_001",                                // 仅道具直购
        GoodsPrice: 100,                                       // 单位：分，仅道具直购
        Quantity:   1,
        OutTradeNo: outTradeNo,
        Attach:     "自定义透传数据",
    })
if err != nil {
    return err
}
// 把 p.SignData / p.PaySig / p.Signature / p.Mode 交给前端
```

> ⚠️ `p.SignData` 必须**原样**传给前端，前端不得重新序列化，否则签名会失配——
> 这是最常见的 `-15006` 来源。

可运行的最小示例见 `ExampleBuildPayment`（`example_test.go`），那个示例是被
`go test` 实际执行的，不是贴来好看的。

### 入参校验

这些都在**本地**就拦下来，报错里带着具体原因，不用等微信侧返回一个笼统的
「参数错误」：

- `Mode` 必须合法（`short_series_goods` 或 `short_series_coin`）；
- 道具直购：`ProductID` 必填、`GoodsPrice` 必须是正数；
- 代币充值：**不该**传 `ProductID` / `GoodsPrice` / `ActivitySellingPrice`
  （它们是道具直购专用的，传了直接报错——静默忽略会让人以为价格生效了）；
- `OutTradeNo`：8–32 位数字/大小写字母/`_-|*@`，不能以 `_` 开头；
- `Attach`：**必填**（官方 signData 字段表的必填列标「是」，两种模式都是）；
- `ActivitySellingPrice`：**不得低于 `GoodsPrice` 的 40%**。

### 金额单位是「分」

`GoodsPrice` 与 `ActivitySellingPrice` 的单位都是**分**。

## 3. 接收推送

推送走的是微信**标准「消息推送配置」通道**（MP 后台 开发管理 → 消息推送配置），
验签机制与支付签名**毫无关系**：

```
signature = sha1( sort([Token, timestamp, nonce]).join("") )
```

`Token` 是你在 MP 后台自填的令牌，**不是 AppKey**；签名参数在 URL query 上。
把它作为**第一个参数**传给 `ParseNotification` 即可——本包没有需要提前构造的对象。

> ⚠️ **本包只支持 JSON 报文 + 明文模式。** MP 后台「消息推送配置」里两项都要配对：
> 数据格式选 **JSON**，消息加解密方式选 **明文**。配成 XML 或安全模式，推送都会
> 被拒绝（库会给出明确的报错，而不是含糊的解析失败）。

### 验签证明了什么、没证明什么

**签的只有 URL 上的 `Token` / `timestamp` / `nonce` 三个参数，报文体不在其中。**
所以验签通过只说明「这条请求来自微信」，**不等于报文可信**：

- 这组三元组就摆在请求 URL 上，一旦落进访问日志、代理日志或浏览器历史，拿到它的
  人就能反复重发，并任意替换 body——比如发一条自造的 `xpay_goods_deliver_notify`
  让你重复发货；
- 本包**不做时间窗校验**（微信的重试跨 2、4、8…最多 15 次、合计可达数小时；
  若重试复用同一组 timestamp/nonce，时间窗会把合法重试一并挡掉，弊大于利）。

**结论：幂等与订单归属校验是调用方的责任，且不可省。**

### 事件对照

| 推送类型 | `Event` 值 | 结构体 |
| --- | --- | --- |
| 道具发货推送 | `xpay_goods_deliver_notify` | `GoodsDeliverNotify` |
| 代币支付推送 | `xpay_coin_pay_notify` | `CoinPayNotify` |
| 退款推送 | `xpay_refund_notify` | `RefundNotify` |
| 用户投诉推送 | `xpay_complaint_notify` | `ComplaintNotify` |
| 微信支付风控事件通知 | `xpay_wxpay_callback_notify` | `WxpayCallbackNotify` |
| iOS 退款问询推送 | `xpay_subscribe_ios_refund_query_notify` | `IOSRefundQueryNotify` |

`ParseNotification` 解析出的 `Notification` 上，只有与 `Event` 对应的那个字段非 nil。
未知事件**不报错**（微信将来可能新增），会返回带 `Event` 的 `Notification`、载荷全 nil，
由调用方决定怎么处理。

### 处理示例

本包只做两件事：**解析**和**给你应答用的结构体**。中间怎么编排——回什么、什么时候
回、要不要先落库再应答——全在你的 handler 里。应答体就是普通 struct，
`json.Marshal` 写出去即可（完整可运行版本见 `ExampleParseNotification`）。

```go
func writeJSON(w http.ResponseWriter, v any) {
    body, _ := json.Marshal(v)
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.Write(body)
}

notif, err := wechat_virtualpay_go.ParseNotification(os.Getenv("VIRTUALPAY_NOTIFY_TOKEN"), r)
if err != nil {
    // 验签/格式不过：回失败应答让微信重试，**绝不要发货**
    writeJSON(w, wechat_virtualpay_go.Ack{ErrCode: 1, ErrMsg: err.Error()})
    return
}

ack := wechat_virtualpay_go.Ack{ErrCode: 0, ErrMsg: "success"}
switch notif.Event {
case wechat_virtualpay_go.EventGoodsDeliver:
    if err := deliver(notif.GoodsDeliver); err != nil {
        ack = wechat_virtualpay_go.Ack{ErrCode: 1, ErrMsg: err.Error()} // 微信会重试
    }

case wechat_virtualpay_go.EventIOSRefundQuery:
    // ⚠️ 这条路径只有 3 秒，不要查库、不要调外部接口
    writeJSON(w, wechat_virtualpay_go.IOSRefundQueryResponse{
        ResultCode: 0, // 0=放过、建议退款；1=拦截、拒绝退款
        ResultInfo: "已发货，不予退款",
        Evidence:   "该订单已于 2026-01-01 发放并被用户领取", // 必填，退款审计要看
    })
    return

default:
    // 不认识的事件别静默 ack——回失败让它出现在日志里
    ack = wechat_virtualpay_go.Ack{ErrCode: 1, ErrMsg: "未知事件: " + string(notif.Event)}
}
writeJSON(w, ack)
```

两种应答体，覆盖你要决定的全部语义：

| 结构体 | 含义 | 微信的行为 |
| --- | --- | --- |
| `Ack{ErrCode: 0, ...}` | 已处理完毕 | 不再推 |
| `Ack{ErrCode: 1, ErrMsg: err.Error()}` | 没处理成功 | 按 2、4、8、16… 重试，最多 15 次 |
| `IOSRefundQueryResponse{...}` | iOS 退款问询的答复 | 只对这条问询有效，**不能**用 `Ack` |

`Ack` 的**零值就是成功应答**（`ErrCode` 的零值是 0）——`var ack Ack` 未经赋值
marshal 出去就是「已处理完，别再推了」。

### 四条铁律

1. **验签失败绝不发货。** `ParseNotification` 返回错误时 `Notification` 是 nil，回
   `Ack{ErrCode: 1, ...}` 让微信重试即可。但注意**反过来不成立**：验签通过也不等于
   报文可信（见上），别把「验签过了」当作跳过后两条的理由。
2. **用平台单号做幂等。** 发货场景取 `WeChatPayInfo.MchOrderNo` 去重——微信会重试，
   同一单可能推多次。同时**确认该订单在你自己库里真实存在、金额对得上**：报文体不参与
   签名，重放的报文可能是伪造的。注意 `WeChatPayInfo` **可能为 nil**（官方注明：
   非微信支付渠道可能没有），取值前先判空。
3. **成功应答是承诺，不是默认值。** 回了 `ErrCode: 0` 但没发货，微信不再重试，这笔单
   就永久丢了。拿不准就回非 0，让微信重推。
4. **iOS 退款问询有 3 秒硬限制。** `xpay_subscribe_ios_refund_query_notify` 要求 3 秒
   内应答，Apple 会问询三次。这条路径上不要查库、不要调外部接口，直接回一个
   `IOSRefundQueryResponse` 即可。

## 签名

虚拟支付涉及两个 HMAC-SHA256 签名，输出 64 位小写十六进制：

```
pay_sig   = hex( HMAC-SHA256( appKey,     uri + "&" + signData ) )
signature = hex( HMAC-SHA256( sessionKey, signData ) )
```

`pay_sig` 会拼上 `uri` 而 `signature` 不会，**两者不可共用一个函数**。拉起支付时
`uri` 固定为字符串 `"requestVirtualPayment"`（这是签名用的 method，不是 HTTP 路径）；
将来调服务端接口时 `uri` 是接口路径，如 `/xpay/query_order`，且**不带** `?` 及其后的
query string。

两个函数分别是 `CalcPaySig` 与 `CalcSignature`（一般不用手调，`BuildPayment` 内部
已经算好了）。

> ⚠️ **字符串一致性**：`pay_sig` 是对**一段具体的 JSON 字符串**做 HMAC。构建的串、
> 算签名用的串、下发给前端的串、微信校验的串，必须**字节级一致**。`SignData` 一旦
> 被重新序列化（换了字段顺序、多了转义），签名就会失配。所以那句「原样传」不是客套话。

> 推送回调的验签**不是** HMAC、也不使用 AppKey，见「[接收推送](#3-接收推送)」一节。
> 两套机制毫无关系。

## iOS 与 Android 的差异

真实存在且必须知道，但**不影响下单参数**——signData 里没有 platform 字段，设备路由
由微信按客户端自动完成。

| | Android | iOS |
| --- | --- | --- |
| 通道 | 微信支付 | Apple IAP |
| 费率 | ~1% | ~12% |
| 结算 | T+3 | 45–60 天 |
| 退款 | 可主动调用退款接口 | **开发者无法主动退款**，只能接收退款问询 |

---

## 尚未实现（规划中）

> ⚠️ **下面这些现在都不存在**，是本仓库的目标形态，列在这里是为了说明设计意图。
> 别照着写代码——编译不过。

### 服务端接口与 `access_token`

计划提供 `NewClient(Config{...})`，把 token 的获取、缓存与刷新包在内部，33 个官方
`/xpay/*` 接口全部挂在 `Client` 上，方法名 = 官方接口英文名的大驼峰形式
（`QueryOrder` 对应 `/xpay/query_order`），按官方 7 大类分文件：

`xpay_coin.go`（代币）、`xpay_goods.go`（道具）、`xpay_order.go`（订单）、
`xpay_bill.go`（账单）、`xpay_funds.go`（资金）、`xpay_adverfunds.go`（广告金）、
`xpay_complaint.go`（投诉）。

token 计划走微信**稳定版**接口 `POST /cgi-bin/stable_token` 的普通模式。选它而不是旧的
`GET /cgi-bin/token`，是因为它有两个关键性质：有效期内**重复调用不会更新**
`access_token`；与旧接口**完全隔离、互不影响**。因此多实例各持一份内存缓存是安全的，
不需要分布式锁或 Redis（旧接口才有「A 实例刷新会让 B 实例手上的 token 失效」的问题）。

还计划在收到 `40001`/`40014`/`42001` 时**就地作废该 AppID 的缓存、换一个新 token
重发一次**，调用方无感。重发是安全的：这个错误码意味着请求在**鉴权阶段**就被拒了，
没有产生业务副作用。

### 错误码表

计划在 `errors.go` 里给出 23 个 `268490xxx` 错误码常量与中文说明，用
`ErrorCode.ErrorText()` 取：

```go
code := wechat_virtualpay_go.ErrCodeSessionKeyExpired
log.Printf("errcode=%d %s", int(code), code.ErrorText())
```

⚠️ 虚拟支付有**两套错误码**，别混：服务端接口是 `268490xxx`；
小程序端（`wx.requestVirtualPayment` 的 fail 回调）是 `-150xx`，本库不会返回。
`-15006` 是签名失配、`-15016` 是 signData 格式有问题。

### 金额单位的坑

除提现相关字段外，绝大多数金额单位是**分**。计划的三个例外用「元」且是字符串：
`CreateWithdrawOrderRequest.WithdrawAmount`、`QueryWithdrawOrderResponse.WithdrawAmount`、
`BizBalance.Amount`。把「元」按「分」的直觉填进去会差 **100 倍**。

### 官方文档的自相矛盾

官方文档存在若干自相矛盾之处，本包按更可靠的一方实现，并在源码注释里逐条标注。
以下是**已调研、但涉及尚未实现的接口**的部分，留待实现时对照：

| 位置 | 矛盾所在 |
| --- | --- |
| 广告金 7 个 | 请求体 `env` 注释写「仅作为签名校验」，但 query 里**没有 `pay_sig`**（该句是跨页模板文字——明确需要 `pay_sig` 的 `QueryBizBalance` 页上也有它） |
| `RefundOrder` / `GetComplaintList` | 「注意事项」写「使用用户态签名与支付签名」，参数表只有 `pay_sig` |
| `CurrencyPay` | 必填列**全部标「否」**（`openid`/`amount` 不可能非必填） |
| `QueryPunishmentReasons` | 写「请求体：无」，却又要 `pay_sig` |
| `NotifyProvideGoods` | `order_id` 与 `wx_order_id` 的必填列**都标「是」**，说明列却写「二选一」 |
| `QueryUserBalance` 的 `first_save_flag` | 类型列写 `boolean`，说明列写「0:不满足 1:满足」，示例写 `false`——三处不一致 |
| `QueryPunishmentReasons` 的 `relate_limitations` | 返回参数表写 `string`，同页返回示例却是数组 `[{…}]` |

最后两行是**类型层面的矛盾**，性质比前几行重：计划按参数表的类型列实现，若微信
实际返回的是另一种表示，**整个响应会反序列化失败**（不是丢一个字段）。实现这两处前
建议先抓一次真实响应确认。

### 已知存疑：`mode` 放在哪一层

官方 `wx.requestVirtualPayment` 页的 signData 字段表里列了 `mode` 且标为必填，但
**同页的官方示例中 signData 里没有 `mode`**。本包按示例走（`mode` 是顶层参数，不属于
signData），测试 `TestGoodsSignDataMatchesOfficialExample` 也钉着这条。同一张表还把
`env` 标为必填「否」，而本包固定填 `0`（现网）。真机联调时值得再确认 `mode` 到底放哪儿。

## License

[MIT](LICENSE)
