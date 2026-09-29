package wechat_virtualpay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 本文件是 /xpay/* 服务端接口的传输层：拼请求体、算签名、发出去、解析响应。
//
// 对外是三个方法，对应官方的**三档鉴权**——差别只在 query 里带哪个签名：
//
//	PostTokenOnly   只带 access_token                    9 个
//	PostWithPaySig  access_token + pay_sig              21 个
//	PostWithUserSig access_token + signature + pay_sig   3 个
//
// 每档几个，是按本包覆盖的 33 个 /xpay/* 接口（见 access_token.go 文件头）逐页核对 query
// 参数表数出来的（2026-09）。**官方一共有多少个接口页，本包没有独立核实过**——文档在
// 开发机上取不到。所以这里说的是「本包覆盖到的接口里各档有多少」，不是官方总数。
//
// 分三档、而不是一个方法挂可选参数，是因为「这个接口要不要签名」是接口的**固有属性**
// （官方参数表写死的），不该由调用方每次自己判断——判断错了微信只回一个签名错误码，而
// 错误码要真机才看得见。哪一档有哪些接口，见各方法注释。
//
// **access_token 一律由调用方传入。** 官方参数表对它只有一句「接口调用凭证，可使用
// access_token、authorizer_access_token」：自研小程序传自己的 access_token（用本包的
// GetStableAccessToken 换，旧接口 GetAccessToken 也有），第三方平台代商家调用传自己的
// authorizer_access_token（由开放平台那边换，见 access_token.go 文件头）。两者在请求里的
// 位置与形状完全一样，都是这一个字符串——所以三个 PostXxx 既不换取、也不缓存它：**换号是
// 单独的、可选的函数**（access_token.go 换应用级凭证、session.go 换用户级），**缓存是
// 调用方的进程模型问题**。
//
// **没有 Client 类型也是有意的**，不是还没来及封：这一层无状态，凭据走参数最直白，
// 调用点上一眼能看出这份凭据是哪来的、是谁的。
//
// 顺带一个性质：本包**无状态，所以每个函数天然并发安全**，从多少 goroutine 里同时调
// 都行，不需要读文档确认。哪天本包真接管了 token 的**缓存与刷新**（现在只换号、不缓存），
// 这一句就得改成「并发安全，靠库里的锁」，并补一个 -race 的并发测试——那正是引入共享可变
// 状态要付的账。
//
// 三个不变量，前两个出自官方《虚拟支付签名》：
//
//  1. pay_sig = hmac_sha256(AppKey, uri + "&" + 请求体)。uri 是**不带** "?" 及其后
//     query string 的接口路径（如 "/xpay/query_order"），带上会报 268490003。
//  2. **参与签名的字节必须就是发出去的字节。** 所以这里只序列化一次，签名与请求体
//     复用同一个 []byte，绝不二次序列化。
//  3. 请求体里的 env **默认** 0（现网），但可以由调用方改成 1（沙箱）——它是请求结构体
//     上的 int 字段。
//     官方写明 env=0 用现网 AppKey、env=1 用沙箱 AppKey——两把不能混，而本包分不出
//     传进来的是哪把，只能靠调用方保证。
//
// **响应原值返回。** 微信回的字段一个不落，包括公共头里的 errcode/errmsg，本包不解释、
// 不翻译、不吞——errcode 是微信的**语义**，藏起来的封装等于让调用方看不见失败。
//
// 所以 error 只有一种含义：**这一趟没走通**——参数没过本地校验，或者没拿到可解析的响应
// （连不上、超时、非 200、响应不是 JSON）。微信的**业务失败不是 error**。
//
// 反过来说，这条契约给调用方添了个义务：**err == nil 不等于成功**，成功与否要看返回的
// resp.ErrCode（0 才是成功）。这个义务必须写在文档里，因为签名没变、编译能过，看漏了是
// 静默的。

// xpayAPIBase 是微信开放接口的基础地址。
//
// /xpay/* 走微信开放接口（access_token 鉴权），不是微信支付 APIv3
// （api.mch.weixin.qq.com）。它是变量而非常量，只为测试能指向本地服务。
var xpayAPIBase = "https://api.weixin.qq.com"

// xpayHTTPClient 是调 /xpay/* 用的 HTTP client。与 sessionHTTPClient 同理，抽成包级
// 变量是为了测试能替换掉它，而不是每次调用都多传一个参数。
var xpayHTTPClient = &http.Client{Timeout: 15 * time.Second}

// ResponseHeader 是微信所有 HTTP 响应的公共头：errcode/errmsg。
//
// 本包**每一处要解析的响应**都把它内嵌在最前面，不只是 /xpay/*：会话凭据（见 session.go
// 的 SessionInfo）、商家级凭证（见 access_token.go）也一样。于是全包只有一条成败判据——看
// ErrCode，是 0 还是别的，读法在每个响应上都相同。
//
// 它以**匿名字段**内嵌在每个响应结构体里（见 xpay_order.go），调用方用 resp.ErrCode
// 直接读——内嵌字段会被提升，包外照样访问得到。
//
// 它是导出的，因为调用方用得上：官方新加、本包还没跟上的接口，调用方自己定义响应结构体时
// 要内嵌这个类型——三个 PostXxx 方法的 out 必须是「内嵌了 ResponseHeader 的结构体指针」
// （约束见 xpayResponse）：
//
//	type BalanceResponse struct {
//		wechat_virtualpay.ResponseHeader
//		Balance int `json:"balance"`
//	}
//
// 本包**不解释它的值**：不拿 errcode 做分支、不翻译、不替调用方判断成败。成功时微信不
// 返回 errcode，所以零值即成功；一旦非 0 就是失败——这条由调用方自己用。
type ResponseHeader struct {
	// ErrCode 微信错误码。成功响应里没有这个字段，非 0 即为失败。
	ErrCode int `json:"errcode"`
	// ErrMsg 错误信息。
	ErrMsg string `json:"errmsg"`
}

// xpayResponse 是「内嵌了 ResponseHeader 的响应结构体」这个集合，用作三个 PostXxx 方法
// 与 xpaySend 的类型约束。
//
// 它不导出：调用方不需要、也没法写它，内嵌了 ResponseHeader 就自动满足。
//
// 约束的意义是让**传错类型**在编译期就过不去。光写 `out *T` 只保证「是个指针」：
// `&Order{}` 之类照样能传，而那样 errcode/errmsg 会被悄悄丢掉，等于把本包唯一的失败信号
// 扔了。方法用**指针接收者**也是故意的：这样值类型不满足约束，传值同样编不过。
type xpayResponse interface {
	header() *ResponseHeader
}

func (h *ResponseHeader) header() *ResponseHeader { return h }

// xpayNoEnvRequest 是「请求体里**没有** env 这个字段」的接口集合，由 requestBody 认。
//
// 官方把 env 标成必填，绝大多数请求体里确实有它，所以那儿的兜底是给它补一个 0。但有三个
// 接口的官方字段表里**没有 env 这一行**：账单类的 DownloadBill / DownloadIOSBill（见
// xpay_bill.go 文件头），以及投诉类的 query_punishment_reasons（那一页连请求体都没有，
// 本包发一份空的 {}，见 punishmentReasonsBody）——给它们补一个，就是凭空多出一个文档里
// 没有的字段。
//
// 所以「不要 env」得由请求结构体自己**显式**标出来。方向不能反过来（默认不补、要的才
// 声明）：那样哪个新接口忘了声明 env，就会静默少发一个官方标为必填的字段，而这个出口
// 没有第二道网能发现。
//
// 它不导出：调用方没法也不需要给自己的请求体打这个标记——要不要 env 是本包按官方参数表
// 决定的事，不是调用方的选择。
type xpayNoEnvRequest interface{ xpayNoEnv() }

// requestBody 把请求体序列化成 JSON，并保证官方的必填字段 env 一定在。
//
// env 是请求结构体上的 int 字段，所以正常路径就是「按结构体
// 序列化完原样发出去」，键序即字段声明顺序。这里只在**结构体没带 env** 时兜底补一个
// 0（现网）——兜底挂在这个唯一的出口上，图的是漏不了：所有请求体都只能从这一个口子
// 出去，将来哪个接口忘了声明这个字段，也不会发出一个缺 env 的请求。
//
// 兜底那条路要走一遍「Marshal → 拆成 map → 再 Marshal」，顶层键因此变成字典序。
// 不影响正确性：签名与请求体是同一份字节，微信按收到的原文校验，不关心键的顺序。
//
// 例外是官方字段表里没有 env 的那三个接口（账单两个 + 投诉的 query_punishment_reasons
// ——后者的官方页连请求体都没有，所以发的是一份空的 {}）：它们实现了 xpayNoEnvRequest，
// 这里就原样发、连键序都不动（上面那句「微信不关心键的顺序」只对补过 env 的那些成立
// ——不补的那些，字节与文档的字段表就应当逐行对得上）。
func requestBody(req any) ([]byte, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay: 序列化请求体失败: %w", err)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		// 传进来的不是 JSON 对象（比如误传了切片、数字，或 nil 指针序列化出的
		// "null"）。必须硬失败：放过去就会发出一个 body 与签名语义对不上的请求。
		//
		// 这一步刻意排在下面那个例外**之前**：声明了「不带 env」不代表可以不讲形状，
		// 那个例外只是不补字段，不是不再检查请求体。
		return nil, fmt.Errorf("wechat_virtualpay: 请求体不是 JSON 对象: %s", raw)
	}
	if _, ok := req.(xpayNoEnvRequest); ok {
		return raw, nil // 显式声明了不带 env：原样发，不补也不重排
	}
	if _, ok := obj["env"]; ok {
		return raw, nil // 结构体自己带了 env，原样发，不碰它
	}

	obj["env"] = json.RawMessage("0")
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay: 序列化请求体失败: %w", err)
	}
	return out, nil
}

// PostTokenOnly 调一个**只带调用凭证**的接口：query 里除 access_token 什么都不带。
//
// 本包覆盖的 33 个 /xpay/* 接口里这一档有 9 个（2026-09 逐页核对），如 notify_provide_goods
// （通知发货完成）、present_currency（代币赠送）、query_adver_funds（广告金发放记录）。
// 它们不碰某个用户的登录态，所以没有可签的东西。三个本包都已封装（见 xpay_order.go /
// xpay_coin.go / xpay_adverfunds.go）。
//
// 入参（都是显式传的，本包不藏状态）：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证。自研小程序传 GetStableAccessToken 换来的 access_token，第三方
//	             平台代商家调用传 authorizer_access_token——两者在这里是同一种东西。
//	uri          接口路径，如 "/xpay/notify_provide_goods"，**不带** "?" 及其后的 query。
//	             形态由本包把着（空、不以 "/" 开头、含 "?" 或 "#" 都在本地拒，见 checkURI）。
//	req          请求数据，传**值**（序列化用不着指针）。只序列化一次，签名与请求体共用那份字节。
//	out          响应结构体**指针**，必须内嵌 ResponseHeader（缘由见 xpayResponse）。
//
// 这一档要哪些凭据：只要 accessToken，与 PostWithPaySig / PostWithUserSig 的差别就在这儿。
func PostTokenOnly[T xpayResponse](ctx context.Context, accessToken, uri string, req any, out T) error {
	raw, err := requestBody(req)
	if err != nil {
		return err
	}
	return xpaySend(ctx, accessToken, uri, raw, url.Values{}, out)
}

// PostWithPaySig 调一个还要**支付签名**的接口：access_token + pay_sig。
//
// 这是多数接口所在的档（33 个里 21 个）：query_order、refund_order、投诉/订阅/账单下载/
// 道具批量上传等。签名用商家的 AppKey 算，见 CalcPaySig。
//
// 入参（都是显式传的，本包不藏状态）：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证。自研小程序传 GetStableAccessToken 换来的 access_token，第三方
//	             平台代商家调用传 authorizer_access_token——两者在这里是同一种东西。
//	appKey       商家密钥，用来算 pay_sig。**必须与环境配套**：请求体 env=0 配现网 AppKey、
//	             env=1 配沙箱 AppKey，两把混用会得到签名错误码。
//	uri          接口路径，如 "/xpay/query_order"，**不带** "?" 及其后的 query（带上会报 268490003）。
//	             形态由本包把着（空、不以 "/" 开头、含 "?" 或 "#" 都在本地拒，见 checkURI）。
//	req          请求数据，传**值**（序列化用不着指针）。只序列化一次，签名与请求体共用那份字节。
//	out          响应结构体**指针**，必须内嵌 ResponseHeader（缘由见 xpayResponse）。
//
// 这一档要哪些凭据：accessToken + appKey——比 PostTokenOnly 多一把商家钥匙。
func PostWithPaySig[T xpayResponse](ctx context.Context, accessToken, appKey, uri string, req any, out T) error {
	raw, err := requestBody(req)
	if err != nil {
		return err
	}
	return xpaySend(ctx, accessToken, uri, raw, paySigQuery(appKey, uri, raw), out)
}

// PostWithUserSig 调一个还要**用户态签名**的接口：access_token + signature + pay_sig。
//
// 官方只有 3 个接口是这一档：currency_pay（扣减代币）、cancel_currency_pay（代币退款）、
// query_user_balance（查代币余额）——**这三个本包已全部封装**（见 xpay_coin.go），所以
// 正常调用用不着这个 PostXxx；留着它，是给「官方新加、本包还没跟上的接口」用（写法见
// ExamplePostWithUserSig）。它们动的是**某个用户**
// 的代币账户，所以除商家的 pay_sig 外还要一把用那个用户的 session_key 签的 signature
// ——两把钥匙都是本方法的参数，缺一不可（见下面的入参表）。
//
// 两把签名的算法不同（官方《签名详解》）：pay_sig 要把 uri 拼在 signData 前面，
// signature = hmac_sha256(sessionKey, signData)、**不拼 uri**；两者的 signData 都是
// **请求体本身**，也就是这里发出去的那份字节——所以仍然只序列化一次（见文件头不变量 2）。
//
// 入参（都是显式传的，本包不藏状态）：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 PostTokenOnly：自研小程序传 access_token，第三方平台传
//	             authorizer_access_token。
//	appKey       商家密钥，签 pay_sig。**必须与环境配套**（env=0 现网、env=1 沙箱）。
//	sessionKey   用户密钥，签 signature。由 Code2Session 用前端 wx.login 的 code 换来，会过期。
//	             它与 appKey **不能互换**：一个签用户、一个签商家，混用会得到签名错误码。
//	uri          接口路径，如 "/xpay/query_user_balance"，**不带** "?" 及其后的 query。
//	             形态由本包把着（空、不以 "/" 开头、含 "?" 或 "#" 都在本地拒，见 checkURI）。
//	req          请求数据，传**值**。只序列化一次：两把签名与请求体共用那一份字节。
//	out          响应结构体**指针**，必须内嵌 ResponseHeader（缘由见 xpayResponse）。
//
// 这一档要哪些凭据：三样都要——accessToken + appKey + sessionKey。
func PostWithUserSig[T xpayResponse](ctx context.Context, accessToken, appKey, sessionKey, uri string, req any, out T) error {
	raw, err := requestBody(req)
	if err != nil {
		return err
	}
	q := paySigQuery(appKey, uri, raw)
	q.Set("signature", CalcSignature(sessionKey, string(raw)))
	return xpaySend(ctx, accessToken, uri, raw, q, out)
}

// paySigQuery 装好那一档签名里共有的部分（pay_sig），PostWithUserSig 再往上加 signature。
func paySigQuery(appKey, uri string, raw []byte) url.Values {
	q := url.Values{}
	q.Set("pay_sig", CalcPaySig(appKey, uri, string(raw)))
	return q
}

// checkURI 校验接口路径（uri）的形态。
//
// 三种都是「本地一眼能拦、发出去要真机才看得见」的：
//
//  1. 空——会打到 https://api.weixin.qq.com? 这个根路径上，回来的错和路径无关，查不出。
//  2. 少了开头的 "/"——拼出来是 https://api.weixin.qq.comxpay/query_order：域名把接口名
//     吃进去了，报的是 DNS 解析失败，完全看不出是自己少打了个斜杠。
//  3. 带 "?" 或 "#"——"?" 的后果有两层：pay_sig 的原文是 uri + "&" + 请求体，uri 里带
//     query 会让签名与微信侧不一致（268490003）；而且拼出来的 URL 会烂成
//     `…/xpay/query_order?foo=1?access_token=…`。"#" 更阴——它后面整段变成 fragment，
//     连 query 一起被 URL 解析器吃掉，请求发出去时 access_token 已经没了。
//
// 前两条是拼错的地址，第三条是拼错的签名原文；都不该等到微信回一个跟原因无关的错误码。
func checkURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("wechat_virtualpay: uri 不能为空，形如 \"/xpay/query_order\"")
	}
	if !strings.HasPrefix(uri, "/") {
		return fmt.Errorf("wechat_virtualpay: uri %q 必须以 \"/\" 开头（它是拼在 %s 后面的路径）", uri, xpayAPIBase)
	}
	if i := strings.IndexAny(uri, "?#"); i >= 0 {
		return fmt.Errorf("wechat_virtualpay: uri %q 不能含 %q——路径只到接口名为止，query 由本包自己拼（带着它签名也会算错）",
			uri, uri[i:i+1])
	}
	return nil
}

// xpaySend 发一次请求、解析响应。
//
// raw 是**已经序列化好的**请求体，签名由它算出——签名与请求体因此必然一致。
// q 里放好了这个接口需要的签名参数（可能一个都没有），access_token 由这里补上。
//
// uri 在这里过一道 checkURI：它是**唯一一个裸着进 URL 的调用方入参**（其余入参要么进
// 请求体、要么被 url.Values 转义），拼错了本地就拦下来，别等微信回一个无关的错误码。
// 放在这个出口上而不是三个 PostXxx 里，图的是漏不了——三个 PostXxx 都从这儿过。
//
// 这里**不看 errcode**：响应整体反序列化进 out，公共头也一起进去，成败由调用方判断
// （见 ResponseHeader）。所以本函数返回的 error 只有一种含义——没拿到可解析的响应。
//
// out 必须是**内嵌了 ResponseHeader 的结构体指针**（可以是本包的响应结构体，也可以是
// 调用方为自己的接口定义的那种），由类型约束 xpayResponse 强制：传值、传裸 nil、传别的
// 类型（`&Order{}` 这种）全都编不过。这不是风格洁癖——本包的契约里 errcode=0 才叫成功，
// 所以「响应压根没解析、结构体还是零值」长得跟成功**一模一样**；而「解析进了错的结构体」
// 更糟，errcode 会被悄悄丢掉。让它编不过，比让它静默通过强。
func xpaySend[T xpayResponse](ctx context.Context, accessToken, uri string, raw []byte, q url.Values, out T) error {
	if err := checkURI(uri); err != nil {
		return err
	}

	q.Set("access_token", accessToken)
	endpoint := xpayAPIBase + uri + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		// 这里也会走 url.Error：uri 由调用方传，带个控制字符就能让 url.Parse 失败，而它的
		// 文案里同样有整个 URL（含 access_token）。所以这条路径也要摘。
		return fmt.Errorf("wechat_virtualpay: %s 构造请求失败: %w", uri, stripURLError(err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := xpayHTTPClient.Do(req)
	if err != nil {
		// 连不上/超时：先摘掉 URL（它带着 access_token，见 stripURLError），再用 %w 挂住
		// 内层错误——errors.Is(err, context.DeadlineExceeded) 照样能用。是不是超时是调用方
		// 重试策略要用的**值**，不能只留在文案里。
		return fmt.Errorf("wechat_virtualpay: %s 请求失败: %w", uri, stripURLError(err))
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay: %s 读取响应失败: %w", uri, err)
	}
	if resp.StatusCode != http.StatusOK {
		// 非 200 的时候响应体不是微信的报文，也就没有 errcode 可给——状态码只能进文案。
		// 原始响应一并带上：网关的 502 页面里常常写着真正的原因。
		return fmt.Errorf("wechat_virtualpay: %s 返回 HTTP %d %s（原始响应: %s）",
			uri, resp.StatusCode, http.StatusText(resp.StatusCode), rawResp)
	}

	// 不再有 `if out != nil` 那种守卫：守卫本身就是一条静默路径——不解析就等于给调用方
	// 留一个零值响应，而零值响应是本包契约里「成功」的样子。现在传进来个 nil，json 会
	// 报 InvalidUnmarshalError，从下面这条路上响亮地失败。
	if err := json.Unmarshal(rawResp, out); err != nil {
		// 业务失败**不会**走到这里：微信的错误响应也是合法 JSON，会被正常解析进 out
		// （errcode/errmsg 就在里面）。走到这里说明响应根本不是微信的报文。
		return fmt.Errorf("wechat_virtualpay: %s 解析响应失败: %w（原始响应: %s）", uri, err, rawResp)
	}
	return nil
}
