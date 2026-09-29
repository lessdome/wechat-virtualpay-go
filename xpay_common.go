package wechat_virtualpay

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// 本文件是全包**共用**的词汇：调用环境，调用凭证与签名的前置校验，以及发请求时的
// 错误收尾（stripURLError）。
//
// 它不含任何一个接口：接口按类分文件（订单类见 xpay_order.go），传输层在 xpay.go。
// 收东西进来只有一条判据——**每一类接口都要用**。某一类自己的规矩（比如订单的
// order_id/wx_order_id 二选一）留在那一类自己的文件里，别往这儿堆；不然这个文件会
// 长成「放不下的都塞这儿」，读的人就得逐条判断哪个跟他要写的接口有关。
//
// 上面那条判据有一条**明文例外**：日期解析（parseDay8 / checkDayRange）。它不属于任何
// 一类接口，而有两个类要用（订单类的 start_download_order 与账单类的两个接口），两边
// 各抄一份的话，同一句文案迟早会走样——那正是这条判据想防的事。除了「跨类共用的小工具」
// 这一类，别的事照旧不许往这儿放。
//
// 校验都在**本地**拦下来：参数不合法时一个字节都不发出去，错误里带着具体原因，
// 不用等微信侧回一个笼统的「参数错误」。

// checkAccessToken 校验接口调用凭证。
func checkAccessToken(v string) error {
	if v == "" {
		return fmt.Errorf("wechat_virtualpay: accessToken 不能为空")
	}
	return nil
}

// checkEnv 校验调用环境。官方标必填，取值只有 0（现网）/ 1（沙箱）。
//
// 为什么 env 就是个裸 int、没给它一个具名类型：值域只有两个数，调用方直接写 0/1 最省事，
// 含义写在**各个请求结构体的字段注释**里（那才是调用方会看的地方）。
//
// 那「两套环境编码不能互赋值」这道保险呢？它不靠这个字段——响应的 env_type 是 1=现网 /
// 2=沙箱 的另一套码，它仍然是具名的 OrderEnvType，所以 req.Env 与 resp.Order.EnvType
// 之间怎么互相赋值都编不过（两侧类型不同名）。保险还在，只是挂在了它该挂的那一侧。
func checkEnv(v int) error {
	if v != 0 && v != 1 {
		return fmt.Errorf("wechat_virtualpay: Env %d 非法，取值只有 0（现网）/ 1（沙箱）", v)
	}
	return nil
}

// checkAppKey 校验支付签名用的 AppKey。
//
// 报错时带上 env 与该配的那把 key：AppKey 与环境绑死（env=0 配现网、env=1 配沙箱），
// 直接说清用哪把比笼统的「不能为空」有用。
//
// ⚠️ 调用它之前必须先过 checkEnv——下面按 env 取名字，默认按现网。
func checkAppKey(appKey string, env int) error {
	if appKey == "" {
		which := "现网"
		if env == 1 { // 沙箱
			which = "沙箱"
		}
		return fmt.Errorf("wechat_virtualpay: appKey 不能为空（Env=%d 须配%s AppKey——官方写明两把 key 不能混）", env, which)
	}
	return nil
}

// checkAppKeyNoEnv 校验支付签名用的 AppKey——给**请求体里没有 env 字段**的接口用。
//
// 与 checkAppKey 的差别只在报错文案：那些接口没有 env 可配，说「Env=0 须配现网 AppKey」
// 会把调用方支到一个根本不存在的字段上（账单类的两个接口就是，见 xpay_bill.go 文件头）。
// 该配哪把 key 由接口所在的环境决定——调用方自己知道，本包这里只拦空串。
func checkAppKeyNoEnv(appKey string) error {
	if appKey == "" {
		return fmt.Errorf("wechat_virtualpay: appKey 不能为空（算 pay_sig 用；本接口的请求体没有 env 字段，key 与环境由调用方自己配对）")
	}
	return nil
}

// checkSessionKey 校验用户会话密钥。
//
// 它与 accessToken / appKey 并列，是本包的三种凭据之一，所以放在这儿与另外两把一起：
// 「凭证校验都在这儿」这条比「按接口类分文件」更该守——漏了它，这个文件的名字就名不
// 副实，而同一句要给人看的文案迟早会在各处走样。
//
// 空串一律拦下。空 sessionKey 会算出一份**算得出来的** signature（HMAC 拿空 key 照样
// 出值），本地看着自洽，要到微信侧才报 268490003。过期是另一回事：过期的 session_key
// 非空，本地查不出来，表现为服务端 268490009（或客户端 -15007），只能靠调用方重新
// Code2Session 换一把。
func checkSessionKey(v string) error {
	if v == "" {
		return fmt.Errorf("wechat_virtualpay: sessionKey 不能为空（用 Code2Session 换取）")
	}
	return nil
}

// parseDay8 把一个 YYYYMMDD 形式的日期（如 20230801）解析出来。
//
// 走 time.Parse 而不是自己数位数：它会连着月、日的合法范围一起校验，20261320 这种
// 「8 位数但不是日期」也被拦下来。用 UTC 解析，所以后面算天数差是精确的 24 小时整数倍，
// 不受夏令时影响。
//
// int64 先转成字符串再解析：官方这几页把日期写成数字（20230801），而 time.Parse 只吃
// 字符串。这个转换不会丢东西——YYYYMMDD 没有前导零的写法。
func parseDay8(v int64) (time.Time, error) {
	return time.Parse("20060102", strconv.FormatInt(v, 10))
}

// checkDayRange 校验一对 YYYYMMDD 日期：两个都得是合法日期，且 EndDs 不早于 BeginDs。
//
// 它**不带**「最多隔多少天」那条：那是各接口自己的规矩（下载订单任务限 31 天，见
// checkDateRange；账单类的两个接口没有这个上限）。上限得算天数差，只有需要的那个接口
// 自己算。
//
// 报错文案里的字段名写死成 BeginDs / EndDs——需要它的两个接口（start_download_order
// 与 download_bill）的字段名恰好是同一个，也就不必为了通用而多传参数。
func checkDayRange(begin, end int64) error {
	b, err := parseDay8(begin)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay: BeginDs %d 不是合法的 YYYYMMDD 日期", begin)
	}
	e, err := parseDay8(end)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay: EndDs %d 不是合法的 YYYYMMDD 日期", end)
	}
	if e.Before(b) {
		return fmt.Errorf("wechat_virtualpay: EndDs(%d) 早于 BeginDs(%d)", end, begin)
	}
	return nil
}

// stripURLError 去掉 http.Client 错误文案里的 URL，只留内层的失败原因。
//
// 为什么非做不可：传输层失败（DNS、连不上、超时）时 Go 回的是 *url.Error，文案形如
//
//	Get "https://api.weixin.qq.com/cgi-bin/token?appid=…&secret=…": dial tcp: …
//
// ——**整个 URL 连 query 一起**。而本包有三处凭据就挂在 query 上：旧换号接口的 secret、
// jscode2session 的 secret、/xpay/* 的 access_token。原样带出去，等于把 AppSecret 写进
// 调用方的日志、错误上报和工单里；它比 access_token 严重得多，因为 AppSecret 不会过期。
//
// 摘掉外壳不会丢信息：判断「是不是超时、是不是被取消」要靠内层的 err（context.Canceled、
// context.DeadlineExceeded、net.Error 都在它上面），errors.Is / errors.As 照样走得通；
// 而 URL 里那点信息本来就没用——调用方的 error 文案里已经写了是哪个接口。
func stripURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
