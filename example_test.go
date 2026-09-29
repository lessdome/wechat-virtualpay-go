package wechat_virtualpay_go_test

import (
	"context"
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
	p, err := wechat_virtualpay_go.BuildPayment(
		"123",           // offerID：虚拟支付商户号
		"appkey",        // appKey：商家密钥，用来算 pay_sig
		"a-session-key", // sessionKey：用户密钥，用来算 signature（真实代码用 Code2Session 换）
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

	notif, err := wechat_virtualpay_go.ParseNotification(
		token, // MP 后台「消息推送配置」里的 Token 令牌（不是 AppKey）
		r)     // 微信推过来的这次请求；body 与 URL 上的 timestamp/nonce/signature 都会被读
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

// ExampleQueryOrder 演示查单——「查单补发」兜底的主流程。
//
// 它**故意不写 `// Output:`**：真实调用要打微信的服务器，输出钉不住，所以这个示例只
// **编译**、不运行。它钉的是另外两件事：一是这段用法**编得过**（照着 godoc 抄下来的调用
// 能通过编译，不会再出现「文档里的示例照着抄却编译失败」）；二是它在**外部测试包**里，只够得着
// 导出的 API——`resp.ErrCode`/`resp.Order` 这套到底能不能从包外读，内嵌的未导出字段会不会
// 提升出来，这种事只有外部包能证明。
func ExampleQueryOrder() {
	resp, err := wechat_virtualpay_go.QueryOrder(
		context.Background(), // ctx：超时与取消由它管
		"ACCESS_TOKEN",       // accessToken：调用凭证（GetStableAccessToken 换来的）
		"APP_KEY",            // appKey：商家密钥，算 pay_sig；必须与请求体 env 配套
		wechat_virtualpay_go.QueryOrderRequest{
			OpenID:  "oXXXX",
			OrderID: "order_1",
		})
	if err != nil {
		// 只有「这一趟没走通」才进来：参数没过本地校验，或没拿到可解析的响应。
		panic(err)
	}
	if resp.ErrCode != 0 {
		// ⚠️ err == nil 不等于成功：微信的业务失败在这儿，errcode/errmsg 是原值。
		fmt.Println("查单失败:", resp.ErrCode, resp.ErrMsg)
		return
	}
	if resp.Order == nil {
		// 走通了但查不到这单——**这不是错误**，也不是失败。
		fmt.Println("查不到这单")
		return
	}
	fmt.Println(resp.Order.Status, resp.Order.LeftFee)
}

// ExampleGetStableAccessToken 演示应用级凭证从换到用的一条龙。
//
// 它**故意不写 `// Output:`**（要打微信的服务器，输出钉不住），只钉两件事：这段用法编得过，
// 以及「换号」与「调用」在本包是分开的两件事——换来的 token 是自己拿着的字符串，传给谁、
// 存哪儿都由调用方决定（本包不封缓存，因为它答不了「一个进程里跑多个小程序怎么办」）。
//
// 它也是**外部测试包**，所以顺带证明这些 API 从包外够得着。
func ExampleGetStableAccessToken() {
	ctx := context.Background()

	// 1. 换号。真实代码里应该缓存起来（有效期见 resp.ExpiresIn，留 5 分钟余量再换），
	//    别每次调用都换一把——虽然普通模式下重复调用不会换新号，但白花配额。
	tok, err := wechat_virtualpay_go.GetStableAccessToken(
		ctx,         // ctx：超时与取消由它管
		"wxAPPID",   // appID
		"APPSECRET", // appSecret
		false)       // forceRefresh：普通模式传 false；true 会让上一把 token 立刻失效，慎用
	if err != nil {
		panic(err) // 这一趟没走通
	}
	if tok.ErrCode != 0 {
		// 业务失败不是 error：appid/secret 不对时走这儿（40013 / 40125）。
		fmt.Println("换 access_token 失败:", tok.ErrCode, tok.ErrMsg)
		return
	}

	// 2. 用号。它只是个字符串参数，与用户级的 session_key 无关。
	resp, err := wechat_virtualpay_go.QueryOrder(ctx,
		tok.AccessToken, // accessToken：上一步换来的凭证，原样传进来
		"APP_KEY",       // appKey：商家密钥，算 pay_sig
		wechat_virtualpay_go.QueryOrderRequest{OpenID: "oXXXX", OrderID: "order_1"})
	if err != nil {
		panic(err)
	}
	if resp.ErrCode != 0 {
		fmt.Println("查单失败:", resp.ErrCode, resp.ErrMsg)
		return
	}
	if resp.Order == nil {
		fmt.Println("查不到这单")
		return
	}
	fmt.Println("订单状态:", resp.Order.Status)
}

// ExamplePostWithUserSig 演示用最高那一档直接调一个接口——**不经过本包的封装**。
//
// 三个 PostXxx 对应官方的三档鉴权，哪一档由接口自己的参数表决定，不是调用方每次挑：
//
//	PostTokenOnly   query 里只有 access_token              （33 个里 9 个）
//	PostWithPaySig  access_token + pay_sig                （21 个）
//	PostWithUserSig access_token + signature + pay_sig    （3 个）
//
// 本包覆盖的 33 个 /xpay/* 服务端接口**已全部封装**（本档三个见 xpay_coin.go 的
// QueryUserBalance / CurrencyPay / CancelCurrencyPay；33 是这层覆盖面的数，不是官方接口页
// 总数——官方还有没有别的接口页本包没有独立核实过，见 xpay.go 文件头），所以这个示例演示的
// 不是「还没封装的接口」，而是另一条仍然成立的路：**调用方自己定义响应结构体**直接交给
// PostXxx。下面用的 uri 与请求体与官方《签名详解》里那份参考脚本**同形**（字段与 uri 都
// 一样，取值不同；那份脚本钉在 sign_test.go）——想看封装后的用法，换成 QueryUserBalance
// 即可。
//
// 它**故意不写 `// Output:`**：真实调用要打微信的服务器，输出钉不住，所以这个示例只
// **编译**、不运行。它是**外部测试包**，因此还证明了一件只有包外才验得了的事：响应结构体
// 可以由**调用方自己定义**——只要内嵌 ResponseHeader，就满足 PostXxx 的类型约束。约束挡的
// 是「忘了内嵌公共头」这种错：那样 errcode 会被悄悄丢掉，而 errcode 是本包唯一的失败信号。
// 这条路的价值不在于绕过封装，而在于官方**新加**接口时调用方不必等本包发版。
//
// access_token 同样是调用方传进来的：自研小程序的用它调 GetStableAccessToken 换（见
// ExampleGetStableAccessToken），第三方平台代商家调用的 authorizer_access_token 由
// 开放平台换——两者在这个参数上是同一种东西，本包不替调用方换取或缓存。
func ExamplePostWithUserSig() {
	// 自己的响应结构体：公共头 + 这个接口自己的字段（按官方返回参数表写）。
	type BalanceResponse struct {
		wechat_virtualpay_go.ResponseHeader
		Balance int `json:"balance"`
	}

	var resp BalanceResponse
	err := wechat_virtualpay_go.PostWithUserSig(context.Background(),
		"ACCESS_TOKEN", // 调用凭证
		"APP_KEY",      // 商家那把钥匙，签 pay_sig
		"SESSION_KEY",  // 用户那把钥匙（code2Session 换来的 session_key），签 signature
		"/xpay/query_user_balance",
		map[string]any{ // 请求体：用什么类型都行，只要有 env
			"openid":  "oXXXX",
			"user_ip": "1.2.3.4",
			"env":     0,
		},
		&resp)
	if err != nil {
		panic(err) // 这一趟没走通：参数没过本地校验，或没拿到可解析的响应
	}
	if resp.ErrCode != 0 {
		fmt.Println("微信报错:", resp.ErrCode, resp.ErrMsg)
		return
	}
	fmt.Println("代币余额:", resp.Balance)
}
