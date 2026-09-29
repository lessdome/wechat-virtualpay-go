# wechat_virtualpay

[![CI](https://github.com/lessdome/wechat_virtualpay/actions/workflows/ci.yml/badge.svg)](https://github.com/lessdome/wechat_virtualpay/actions/workflows/ci.yml)

微信小程序**虚拟支付**的服务端 Go SDK。Go 1.21+，**零第三方依赖**（只用标准库）。

它管的是服务端这一半：拼下单参数、算签名、调 `/xpay/*` 接口、收推送。另一半
（`wx.login`、`wx.requestVirtualPayment`）在客户端，不在这里。

包级概览（错误契约、三档鉴权、凭据与环境、金额单位）在
[`doc.go`](https://pkg.go.dev/github.com/lessdome/wechat_virtualpay) 里，`go doc .` 就能读到；
本文件负责的是**入门路径**和**协议层的坑**。单接口的字段表与逐条存疑在各函数自己的注释里。

## 当前状态

| 能力 | 入口 |
| --- | --- |
| 支付签名与用户态签名 | `CalcPaySig` / `CalcSignature` |
| `wx.login` 的 code 换登录态 | `Code2Session` |
| 应用级凭证（稳定版 / 旧版） | `GetStableAccessToken` / `GetAccessToken` |
| 下单参数构建（道具直购 + 代币充值） | `BuildPayment` |
| 业务订单号生成 | `NewOutTradeNo` |
| 推送解析与应答（6 类事件，JSON + 明文模式） | `ParseNotification` / `Ack` / `IOSRefundQueryResponse` |
| `/xpay/*` 服务端接口 | 33 个，见[下面的分类表](#服务端接口) |
| 错误码表 | **没有，也不打算有**。本包不解释 errcode，请查官方文档 |

```bash
go test -race ./...    # 全部测试，无需任何真实凭证（网络层都是本地的 httptest）
```

**未真机联调。** 全历史没有联调记录。签名与验签是拿官方文档正文里自带的样例逐字节对过的
（`sign_test.go`、`notify_verify_test.go`），所以这两处把握最大；其余接口的行为——请求字段、
必填项、返回值——**只有文档，没有实测**，上生产前请用真实凭证完整跑一遍。文档里存在自相
矛盾的地方逐条记在源码注释的 ⚠️ 里（`grep -n "⚠️" *.go`），其中会影响接入的汇总在
[已知存疑](#已知存疑)。

## 安装

```bash
go get github.com/lessdome/wechat_virtualpay
```

导入路径与包名都是 `wechat_virtualpay`（包名带下划线是有意保留的，与模块路径尾一致）。

## 四步走通

### 1. 换应用级凭证

```go
tok, err := wechat_virtualpay.GetStableAccessToken(ctx, appID, appSecret, false)
if err != nil {
    return err // 这一趟没走通
}
if tok.ErrCode != 0 {
    return fmt.Errorf("换 access_token 失败: %d %s", tok.ErrCode, tok.ErrMsg)
}
```

⚠️ **本包不缓存、不刷新 token。** 换来的就是一个字符串，存哪儿由你的进程模型决定。真实代码
应当缓存起来（有效期看 `tok.ExpiresIn`，留 5 分钟余量），别每次调用都换一把。

### 2. 换用户登录态

```go
sess, err := wechat_virtualpay.Code2Session(ctx, appID, appSecret, code)
// sess.OpenID / sess.SessionKey / sess.UnionID
```

code 五分钟有效且只能用一次；`SessionKey` 也会过期（服务端报 268490009、客户端报 -15007），
过期就让前端重新 `wx.login`。这一步的鉴权与 `/xpay/*` 那套**完全不同**：直接用 appid + secret
换，不需要 access_token。

### 3. 拼下单参数

服务端**不发起支付请求**——它只拼参数、算签名，然后把结果交给小程序端拉起支付：

```go
p, err := wechat_virtualpay.BuildPayment(offerID, appKey, sess.SessionKey,
    wechat_virtualpay.PaymentRequest{
        Mode:       wechat_virtualpay.ModeShortSeriesGoods, // 或 ModeShortSeriesCoin
        ProductID:  "prod_001",                                // 仅道具直购
        GoodsPrice: 100,                                       // 单位：分，仅道具直购
        Quantity:   1,
        OutTradeNo: outTradeNo,
        Attach:     "自定义透传数据",
    })
// 把 p.SignData / p.PaySig / p.Signature / p.Mode 交给前端
```

⚠️ `p.SignData` 必须**原样**传给前端，前端不得重新序列化——构建的串、签名算的串、微信校验的
串必须字节级一致，换了字段顺序或转义就会失配。这是最常见的签名错误来源。

`BuildPayment` **不发网络请求**，本地校验的规则（`Mode` 合法性、道具直购与代币充值各自的必填项、
`OutTradeNo` 的字符集与长度、`Attach` 必填、优惠价不得低于原价 40%）都在注释里写明了依据。
`example_test.go` 的 `ExampleBuildPayment` 是**真跑**的示例，不是贴来好看的。

### 4. 收推送

推送走微信标准「消息推送配置」通道（MP 后台 → 开发管理 → 消息推送配置），验签机制与支付签名
**毫无关系**：

```
signature = sha1( sort([Token, timestamp, nonce]).join("") )
```

此处 Token 是你在 MP 后台自填的令牌，**不是 AppKey**；签名参数在 URL query 上。
`ParseNotification` 自己读请求的 body 与这三个参数，验签失败返回 `ErrInvalidSignature`：

```go
notif, err := wechat_virtualpay.ParseNotification(token, r)
if err != nil {
    // 验签或格式不过：回失败应答让微信重试，**绝不要发货**
    writeJSON(w, wechat_virtualpay.Ack{ErrCode: 1, ErrMsg: err.Error()})
    return
}

ack := wechat_virtualpay.Ack{ErrCode: 0, ErrMsg: "success"}
switch notif.Event {
case wechat_virtualpay.EventGoodsDeliver:
    // 幂等键是平台单号 WeChatPayInfo.MchOrderNo；它可能为 nil，取值前先判空
    if err := deliver(notif.GoodsDeliver); err != nil {
        ack = wechat_virtualpay.Ack{ErrCode: 1, ErrMsg: err.Error()} // 微信会重试
    }
case wechat_virtualpay.EventIOSRefundQuery:
    // ⚠️ 这条路径只有 3 秒，不要查库、不要调外部接口
    writeJSON(w, wechat_virtualpay.IOSRefundQueryResponse{
        ResultCode: 0, // 0=放过、建议退款；1=拦截、拒绝退款
        ResultInfo: "已发货，不予退款",
        Evidence:   "该订单已于 2026-01-01 发放并被用户领取", // 必填，退款审计要看
    })
    return
default:
    ack = wechat_virtualpay.Ack{ErrCode: 1, ErrMsg: "未知事件: " + string(notif.Event)}
}
writeJSON(w, ack)
```

> ⚠️ **只支持 JSON 报文 + 明文模式。** MP 后台那两项都要配对：数据格式选 JSON、消息加解密方式选
> 明文。配成 XML 或安全模式，推送会被明确拒绝。

`Ack` 的**零值就是成功应答**（`ErrCode` 零值为 0）——`var ack Ack` 直接 marshal 出去就是
「已处理完，别再推」。两种应答体的语义：

| 应答 | 含义 | 微信的行为 |
| --- | --- | --- |
| `Ack{ErrCode: 0, ...}` | 已处理完毕 | 不再推 |
| `Ack{ErrCode: 1, ErrMsg: ...}` | 没处理成功 | 按 2、4、8、16… 秒重试，最多 15 次 |
| `IOSRefundQueryResponse{...}` | iOS 退款问询的答复 | 只对这条问询有效，**不能**用 `Ack` |

完整可运行的版本见 `ExampleParseNotification`（带 `// Output:`，被 `go test` 实际执行）。

### 四条铁律

1. **验签失败绝不发货。** `ParseNotification` 返回错误时 `Notification` 是 nil，回
   `Ack{ErrCode: 1, ...}` 让微信重试即可。但反过来不成立：**验签通过不等于报文可信**（见下）。
2. **用平台单号做幂等。** 发货取 `WeChatPayInfo.MchOrderNo` 去重，同一单可能推多次。同时确认
   该单在你库里真实存在、金额对得上——**报文体不参与签名**，重放的报文可能是伪造的。
   `WeChatPayInfo` **可能为 nil**（非微信支付渠道可能没有），取值前先判空。
3. **成功应答是承诺，不是默认值。** 回了 `ErrCode: 0` 却没发货，微信不再重试，这笔单就永久丢了。
   拿不准就回非 0。
4. **iOS 退款问询有 3 秒硬限制。** 这条路径上不要查库、不要调外部接口。

第 2 条的完整因果在 `notify_verify.go` 文件头：签的只有 URL 上那三个参数，报文体不在其中，
而三元组就摆在 URL 上，一旦落进访问日志或代理日志，拿到它的人就能反复重发并任意替换 body。
本包**不做时间窗校验**，因为微信的重试跨 2、4、8…最多 15 次、合计可达数小时，时间窗会把合法
重试一并挡掉——弊大于利。**幂等与订单归属校验是调用方的责任，且不可省。**

### 6 类推送事件

| 事件 | `Event` 常量 | 结构体 |
| --- | --- | --- |
| 道具发货推送 | `EventGoodsDeliver` | `GoodsDeliverNotify` |
| 代币支付推送 | `EventCoinPay` | `CoinPayNotify` |
| 退款推送 | `EventRefund` | `RefundNotify` |
| 用户投诉推送 | `EventComplaint` | `ComplaintNotify` |
| 微信支付风控事件通知 | `EventWxpayCallback` | `WxpayCallbackNotify` |
| iOS 退款问询推送 | `EventIOSRefundQuery` | `IOSRefundQueryNotify` |

`Notification` 上只有与 `Event` 对应的那个字段非 nil。未知事件**不报错**（微信将来可能新增），
返回带 `Event` 的 `Notification`、载荷全 nil，由调用方决定怎么办。

## 服务端接口

官方要求「接收发货推送」与「轮询 `QueryOrder`」**至少实现一个**，两者结合最可靠：success 回调
可能丢，推送也可能丢。

33 个 `/xpay/*` 接口按官方分类封成 7 个文件，**已全部封装**：

| 类 | 文件 | 接口数 | 档位 | 一句话 |
| --- | --- | --- | --- | --- |
| 订单 | `xpay_order.go` | 5 | `PostWithPaySig` ×4 + `PostTokenOnly` ×1 | 查单、退款、通知发货、下载订单（触发 / 查询）；`NotifyProvideGoods` 是那个不签名的 |
| 代币 | `xpay_coin.go` | 4 | `PostWithUserSig` ×3 + `PostTokenOnly` ×1 | 查余额、代币扣减、撤销扣减、赠送；`PresentCurrency` 不签名 |
| 资金 | `xpay_funds.go` | 3 | `PostWithPaySig` ×3 | 提现下单 / 查询、商家余额——**金额是元、字符串** |
| 道具 | `xpay_goods.go` | 4 | `PostWithPaySig` ×4 | 上传 / 发布道具及各自的查询（四个都是异步任务） |
| 账单 | `xpay_bill.go` | 2 | `PostWithPaySig` ×2 | 下载虚拟支付日账单、下载苹果月账单（**请求体无 env**） |
| 广告金 | `xpay_adverfunds.go` | 7 | `PostTokenOnly` ×7 | 服务商转账、创建充值单、查充值 / 回收记录、绑账户、下载（**7 个全不签名**） |
| 投诉 | `xpay_complaint.go` | 8 | `PostWithPaySig` ×8 | 投诉列表 / 详情、协商历史、回复 / 完结、上传文件、处罚原因 |

单接口的入参、返回值与逐条存疑看 godoc（`go doc . QueryOrder` 这样查）。档位是接口的**固有
属性**，不是调用方每次自己挑：

| 方法 | query 里带什么 | 本包覆盖的接口里几个 |
| --- | --- | --- |
| `PostTokenOnly` | 只带 access_token | 9 |
| `PostWithPaySig` | access_token + pay_sig | 21 |
| `PostWithUserSig` | access_token + signature + pay_sig | 3（全在代币类） |

**33 这个数是本包的覆盖面，不是官方接口页总数**——官方一共有多少页本包没有独立核实过（开发机
上取不到官方文档）。上面每档几个是逐页核对参数表数出来的（2026-09）。挑错了档位，微信只回一个
签名错误码，而错误码要真机才看得见。

上面这三个装配函数（`PostTokenOnly` / `PostWithPaySig` / `PostWithUserSig`）本身也是导出的：
调用方可以**自己定义响应结构体**直接调（内嵌 `ResponseHeader` 即满足类型约束）。官方**新加**
接口时，这条路让你不必等本包发版；`ExamplePostWithUserSig` 演示的就是这个。

## 金额单位有 3 种

| 单位 | Go 类型 | 用在哪 | 例子 |
| --- | --- | --- | --- |
| 分 | `int64` | 绝大多数金额 | `GoodsPrice: 100` 是 1 元 |
| 元 | `string` | 只有资金类 3 处：`CreateWithdrawOrderRequest.WithdrawAmount`、`QueryWithdrawOrderResponse.WithdrawAmount`、`BizBalance.Amount` | 提现 1 分钱传 `"0.01"` |
| 代币数量 | `int64` | 代币类的金额与数量字段 | 扣 1 个代币传 `1` |

差一个单位就是 100 倍，而「元」那三处**还是字符串**——把 `"0.01"` 写成 `1`，或者把 100 分
按元的直觉填进去，都是不会报错的错。

另外注意**同一个结构体里两种量纲并存**：`GoodsInfo.Quantity` 是数量、`GoodsInfo.OrigPrice`
与 `ActualPrice` 是分。字段名长得像，单位不一样。

## 签名

虚拟支付涉及两个 HMAC-SHA256 签名，输出 64 位小写十六进制：

```
pay_sig   = hex( HMAC-SHA256( appKey,     uri + "&" + signData ) )
signature = hex( HMAC-SHA256( sessionKey, signData ) )
```

`pay_sig` 拼 `uri` 而 `signature` 不拼，**两者不可共用一个函数**。拉起支付时 `uri` 固定为
字符串 `requestVirtualPayment`（那是签名用的 method，不是 HTTP 路径）；调服务端接口时 `uri`
是接口路径，如 `/xpay/query_order`，且**不带** `?` 及其后的 query string（带上会报 268490003）。

两个函数是 `CalcPaySig` 与 `CalcSignature`（一般不用手调，`BuildPayment` 内部算好了）。
**唯一同时要两个签名的是 `PostWithUserSig`**——三档里的最高一档，只有代币类那 3 个接口用。

> 推送回调的验签**不是** HMAC、也不使用 AppKey（见上）。两套机制毫无关系，别互相套。

## 协议层的坑

- **`mode` 是顶层参数，不属于 signData。** 官方 signData 字段表里列了 `mode` 且标必填，但同页
  官方示例的 signData 里没有它。本包按示例走，`TestGoodsSignDataMatchesOfficialExample` 钉着
  这条。
- **`outTradeNo` 的规则**：8–32 位，只能是数字、大小写字母与 `_`、`-`、`|`、`*`、`@`，且不能以
  下划线开头。`NewOutTradeNo` 直接生成合规单号。官方还有一句要留意：单号重复会失败，但
  「极端情况不保证唯一」。
- **`-15007` / `268490009` = session_key 过期**，让前端重新 `wx.login`。本包不回这些码——它们
  是微信侧的回复。
- **两套错误码别混**：服务端接口是 268490xxx，小程序端（`wx.requestVirtualPayment` 的 fail
  回调）是 -150xx，本包只处理前者。
- **基础库 ≥ 2.19.2**：`wx.requestVirtualPayment` 的客户端前提。
- **iOS 与 Android 的退款不一样**：Android 走微信支付，可主动调 `RefundOrder`；iOS 走 Apple
  IAP，**开发者无法主动退款**，只能被动接收 `EventIOSRefundQuery` 问询并及时答复 3 秒。
  其余差异（费率、结算周期）本包源码与注释里一个字都没有——没有可核的来源，不写。
- **`env` 有两套编码并存且不互转**：请求体是 0=现网 / 1=沙箱，响应里 `order.env_type` 是
  1=现网 / 2=沙箱（`OrderEnvType`）。推送载荷里也有一个 `Env`，值域官方没写。
- **`env` 与 AppKey 绑死**：`env=0` 配现网那把、`env=1` 配沙箱那把，混了报 268490003，而报错
  不会告诉你混了。
- **拼写坑**：`/xpay/bind_transfer_accout` 里的 accout 是**官方**的拼写（少一个 n），不是笔误，
  别顺手改成 account。
- **推送报文的 `body` 不参与签名**，所以时间窗与防重放都要自己来（见[四条铁律](#四条铁律)）。

## 已知存疑

官方文档有若干自相矛盾、说不清或本包主动押注的地方。**这里只列会影响接入的**，逐条的依据与
出处写在源码注释的 ⚠️ 里（`grep -n "⚠️" *.go`）。

### 本包与官方文档打架时，选了哪一边

| 位置 | 官方文档的矛盾 | 本包的选择 |
| --- | --- | --- |
| 广告金 7 个 | 请求体 `env` 注释写「仅作为签名校验」，但 query 参数表里**没有 `pay_sig`**；而明确需要 `pay_sig` 的 `query_biz_balance` 页上也有同一句话——那是跨页模板文字 | 按参数表，7 个全不签名。若实测回 268490003，换到 `PostWithPaySig` 即可 |
| `RefundOrder` | 「注意事项」写「使用用户态签名与支付签名」，参数表只有 `pay_sig` | 按参数表，只加 `pay_sig`。若实测 268490003，要改调 `PostWithUserSig`——那是**改函数签名**（多收一个 sessionKey） |
| `GetComplaintList` | 同上 | 同上，同样会改函数签名 |
| `CurrencyPay` | 参数表里**所有**请求体字段的必填列都标「否」（`openid`、`amount` 不可能非必填） | 按实际语义当必填，本地拦 |
| `QueryPunishmentReasons` | 写明「请求体：无」，却又要求 `pay_sig`（签名是对请求体算的） | 发一份 `{}`：不补 `env`，签的就是 `{}`。若实测签名错误，删掉那个 marker 方法让它补 `env` |
| `NotifyProvideGoods` | `order_id` 与 `wx_order_id` 的必填列**都标「是」**，说明列却写「二选一」 | 按二选一的语义实现，本地校验「恰好一个」 |
| `QueryUserBalance` 的 `first_save_flag` | 类型列写 `boolean`、说明列写「0:不满足 1:满足」、示例写 `false`——三处不一致 | 按**类型列**取 `bool`。若微信实际回 0/1，**整个响应**会反序列化失败（不是丢一个字段） |
| `QueryPunishmentReasons` 的 `relate_limitations` | 返回参数表写 `string`，同页返回示例给的是数组 | 按类型列取 `string`，同样是「类型错了整个响应解析失败」 |
| `RecoverBillFilter.BillID` | 必填列标必填，说明文字写「(可选)」 | 按必填处理 |
| `UploadGoodsItem.ID` | 括号前只列字母数字下划线横线，括号里却说「中文算一个字符」 | 两个都不查，只查非空 |

### 本包在本地就拦住你（官方没这么要求）

官方只给了规则的地方本包不额外加码；下面这些是**本地校验**，拦下来时报错里写着依据：

| 拦什么 | 为什么 |
| --- | --- |
| 凭据空值（accessToken / appKey / sessionKey / appID / appSecret） | 空着发出去只会换来一个笼统的参数错误，还白花一次调用 |
| `appKey` 与 `env` 的搭配提示 | 混了报 268490003，报错不会告诉你混了，所以在本地就把「env=N 该配哪把」写进文案 |
| `env` 只能是 0 或 1 | 值域是官方写死的 |
| 各种「必填」「二选一」「恰好一个」（如 `OrderID` / `WxOrderID`） | 官方字段表标了必填，或说明列写了二选一 |
| 枚举字段的取值（`OrderType`、`PayChannel`、`RefundFrom`、`RefundReason`、`RefundStatus`） | 都有配套的常量块，取值来自官方枚举 |
| 时间戳为 0 / 正负、结束早于开始、日期格式（`2023-01-01`、`2023-01`、`20230101`） | 0 只会是漏填（1970 年）；区间颠倒查不出东西 |
| 金额区间（`RefundFee` 落在 (0, LeftFee]、`TransferAmount` 大于 0、`Amount` 大于 0） | 官方写明或语义上无意义 |
| 单号格式（`OutTradeNo`、`RefundOrderID` 的字符集与长度） | 官方写了规则 |
| 下载订单任务的日期区间上限（31 天） | 那一页自己写的上限，只对那个接口生效，别挪到账单类去 |
| `RequestID` 非空且不超过 1024 字符 | 它是幂等键，空着等于放弃重试保护；长度按**字符**数算（一个汉字 3 字节，按字节算会把合法请求拦在本地） |

### 本包有意不查（别指望包替你校验）

| 不查什么 | 为什么 |
| --- | --- |
| `UserIP` 的格式 | 官方只说「形如 1.1.1.1」 |
| 代币类单号的格式 | 那几页没有像下单页那样的字符集规则 |
| `PayItem` 的内容、`Quantity` / `UnitPrice` 的取值范围 | 官方只把它们记进流水，没说能不能为 0 |
| `WithdrawAmount` 的写法（小数位数、前导零） | 官方只说「元、字符串形式」，没给规则；留空是「全额提现」，合法 |
| 道具 `ID` / `Name` / `Remark` 的长度，`ID` 的字符集 | 长度规则自相矛盾、也没说清按什么算 |
| 账单类的时间跨度上限 | 那两页没给「最多查多少天 / 多少个月」 |
| 上传图片的体积、base64 是否合法、URL 的域名与路径前缀 | 官方没说清 1M / 2M 是按原始字节还是 base64 后算，且那地址是微信侧生成的 |
| 手机号、投诉内容等的长度与字符集 | 官方没给规则 |

### 高影响押注（会改签名或让整趟失败）

| 押注 | 万一押错的后果 |
| --- | --- |
| `RefundOrder` 与 `GetComplaintList` 只加 `pay_sig` | 要改**函数签名**（多收 sessionKey），不是改内部 |
| `QueryUserBalance.FirstSaveFlag` 取 `bool` | 微信若回 0/1，**整个响应**反序列化失败 |
| `RelateLimitations` 取 `string` | 微信若回数组，同上 |
| `QueryPunishmentReasons` 发 `{}`、实现 `xpayNoEnvRequest` | 若期望的不是 `{}`，删掉 marker 方法让它补 `env` |
| 广告金 7 个全部走 `PostTokenOnly` | 若哪个要 `pay_sig`，换到 `PostWithPaySig`（不改函数签名，但要改调用方） |
| `BindTransferAccount` 的两栏从「可选」改成必填 | 比旧实现更严：官方页把「可选」写在说明列，本包按收紧实现 |

### 其余押注的找法

`grep -n "⚠️" *.go` ——每一条都在它该在的那行旁边，带着「依据是文档哪一句」和「万一押错了改
哪里」。总数远多于上面这几张表：上面只挑了会影响接入的。

## License

[MIT](LICENSE)
