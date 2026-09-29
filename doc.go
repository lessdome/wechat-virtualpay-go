// Package wechat_virtualpay_go 是微信小程序「虚拟支付」的服务端 Go SDK。
//
// 它覆盖 33 个 /xpay/* 服务端接口、6 类推送事件的验签与解析，以及下单参数的生成；
// 只用 Go 标准库（Go 1.21+），零第三方依赖。注释一律中文，每处「为什么这么写」和它的
// 依据都写在代码旁边，而不是散在别处。
//
// # 谁负责什么
//
// 微信的虚拟支付由**小程序端拉起**，服务端负责拼参数、签名、以及事后对账：
//
//	小程序端  wx.login 拿 code → 用自己的服务端换 session_key
//	          wx.requestVirtualPayment 用服务端给的下单参数拉起支付
//	本包      [Code2Session] / [GetStableAccessToken] 换两把钥匙
//	          [BuildPayment] 拼下单参数并算两个签名（**不发网络请求**）
//	          [ParseNotification] 收推送：验签、解析、给出应答
//	          33 个 /xpay/* 接口（下单、查单、退款、代币、广告金、投诉……）
//
// 官方要求「接收发货推送」与「轮询 [QueryOrder]」**至少实现一个**，两者结合最可靠：
// success 回调可能丢，推送也可能丢。而**幂等必须调用方自己做**——推送验签只覆盖 URL 上的
// token/timestamp/nonce，报文体不在签名里，所以验签通过**不等于**这条报文没被重放（见
// notify_verify.go 文件头）。发货那四步官方推荐流程写在 [GoodsDeliverNotify] 的注释里。
//
// # 错误契约
//
// error 只有一种含义：**这一趟没走通**——参数没过本地校验，或者没拿到可解析的响应
// （连不上、超时、非 200、不是 JSON）。微信的**业务失败不是 error**：响应原值返回，
//
//	if err != nil { … }         // 这一趟没走通，resp 必然为 nil
//	if resp.ErrCode != 0 { … }  // 走通了，但微信说不行：errcode / errmsg 是原值
//
// 由此有两条不变量：
//
//	resp == nil ⟺ err != nil    // 失败时不会有半个响应给你
//	err == nil ≠ 成功            // 看漏 ErrCode 是**静默**的：签名没变、编译能过
//
// 还有第三层 nil 要分清：resp.Order == nil 是「走通了但查不到这单」（err 与 ErrCode
// 都是 0），与 resp == nil（连响应都没拿到）是两回事。
//
// 本包**不解释 errcode**：不拿它做分支、不翻译、不替调用方判断成败——错误码的含义请查
// 官方文档。同理，响应结构体的类型约束会在**编译期**挡住一类错误：每个响应都必须内嵌
// [ResponseHeader]（约束要求指针接收者），否则 errcode 会被 json 静默丢掉，而它是本包
// 唯一的失败信号。
//
// # 三档鉴权
//
// 官方 /xpay/* 的参数表把每个接口的鉴权写死成三档之一，差别只在 query 里带哪些签名：
//
//   - [PostTokenOnly]：只带 access_token（本包覆盖的 33 个里 9 个）
//   - [PostWithPaySig]：access_token + pay_sig（21 个）
//   - [PostWithUserSig]：access_token + signature + pay_sig（3 个，全在代币类）
//
// 档位是接口的**固有属性**，不是调用方每次自己挑：挑错了微信只回一个签名错误码，而那要
// 真机才看得见。每档具体有哪些接口见各方法的注释；33 这个数是**本包覆盖面**的数，官方一共
// 有多少个接口页本包没有独立核实过（文档在开发机上取不到）。
//
// # 凭据与环境
//
// 三把钥匙都由调用方持有、显式传入每一个函数：
//
//	accessToken  应用级凭证，[GetStableAccessToken] / [GetAccessToken] 换取
//	appKey       商家密钥（商户后台里那把），算 pay_sig
//	sessionKey   用户会话密钥，[Code2Session] 换取，算 signature
//
// **本包不缓存、不刷新、不接管 access_token**：换号是单独的函数，存哪儿由调用方的进程
// 模型决定（单实例 / 多实例 / 一个进程跑多个小程序，本包答不了这些问题）。也没有 Client
// 类型——这一层无状态，凭据走参数最直白，调用点上一眼能看出这份凭据是谁的。顺带一个性质：
// 本包**无状态，所以每个函数天然并发安全**，从多少 goroutine 里同时调都行。
//
// env 有两套编码并存且**不互转**，容易看错：
//
//	请求体的 env 字段    0=现网（默认）/ 1=沙箱；全包没有具名的 Env 类型，就是 int
//	响应里的 order.env_type   1=现网 / 2=沙箱（具名类型 [OrderEnvType]）
//	推送载荷里的 Env     官方没写值域，本包不解读、不过滤
//
// ⚠️ AppKey 与环境绑死：env=0 的请求配现网那把、env=1 的配沙箱那把，两把不能混——混了
// 报 268490003 签名错误，而报错不会告诉你混了。本包分不出哪把是哪把，只能由调用方保证。
// 另外，**账单类的两个接口与 [QueryPunishmentReasons] 的请求体没有 env 字段**，该配哪把
// 由调用方按接口所在环境决定（本包对这些接口也不做 env=0/1 的“补默认”）。
//
// # 金额单位有 3 种
//
// 分（int64，绝大多数接口）、元（**字符串**，只有资金类那 3 个接口）、代币数量（int64，
// 代币类）。差一个单位就是 100 倍——对照表在 README，写代码前先看一眼。
//
// # HTTP 行为与本包不做的事
//
// 域名 https://api.weixin.qq.com。包内三个 *http.Client 只兜底超时：换 access_token
// 与 code2session 10 秒，/xpay/* 15 秒。本包**不重试、不自动换 token、不缓存、不打日志、
// 不读环境变量**——所有失败与所有重试策略都原样交回调用方。
//
// # 序列化
//
// 一律用标准库 json.Marshal，**保留默认的 HTML 转义**（`<`、`>`、`&` 会被写成 Unicode
// 转义序列）。这里不改：微信那边 JSON 解码一次就还原成原字符，转义既不破坏协议也不影响
// 签名（签名算的字节与发出去的字节是同一份），关掉它（SetEscapeHTML(false)）只会引入第二套
// 序列化路径。请求体的字段顺序照官方字段表逐行抄（顺序写在结构体声明里）。
//
// # 文件
//
//	access_token.go     换 access_token：稳定版 / 旧版
//	session.go          code2Session：code → openid / session_key
//	sign.go             两个 HMAC-SHA256 签名：pay_sig 与 signature
//	virtual_payment.go  下单参数生成：[BuildPayment] / [NewOutTradeNo]
//	notify.go           推送解析：验签、事件分发、6 类载荷、原始报文的保留
//	notify_verify.go    推送验签（sha1 字典序拼接，与 pay_sig 无关）
//	notify_ack.go       应答：[Ack] / [IOSRefundQueryResponse]
//	xpay.go             传输层：三个 PostXxx、请求体拼装、响应解析、错误契约
//	xpay_common.go      各类共用的本地校验助手（本文件零导出符号）
//	xpay_order.go       订单类 5 个        xpay_coin.go      代币类 4 个
//	xpay_funds.go       资金类 3 个        xpay_goods.go     道具类 4 个
//	xpay_bill.go        账单类 2 个        xpay_adverfunds.go 广告金类 7 个
//	xpay_complaint.go   投诉类 8 个
//
// 可编译的示例在 example_test.go：[ExampleBuildPayment] 与 [ExampleParseNotification]
// 带输出、可直接跑，另外三个只编译不运行（要打微信的服务器）。
//
// 押注、存疑，以及「本包与官方文档不一致时选了哪边」的清单在 README——联调前值得过一遍。
package wechat_virtualpay_go
