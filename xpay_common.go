package wechat_virtualpay_go

import (
	"errors"
	"fmt"
	"net/url"
)

// 本文件是全包**共用**的词汇：调用环境，调用凭证与签名的前置校验，以及发请求时的
// 错误收尾（stripURLError）。
//
// 它不含任何一个接口：接口按类分文件（订单类见 xpay_order.go），传输层在 xpay.go。
// 收东西进来只有一条判据——**每一类接口都要用**。某一类自己的规矩（比如订单的
// order_id/wx_order_id 二选一）留在那一类自己的文件里，别往这儿堆；不然这个文件会
// 长成「放不下的都塞这儿」，读的人就得逐条判断哪个跟他要写的接口有关。
//
// 校验都在**本地**拦下来：参数不合法时一个字节都不发出去，错误里带着具体原因，
// 不用等微信侧回一个笼统的「参数错误」。

// checkAccessToken 校验接口调用凭证。
func checkAccessToken(v string) error {
	if v == "" {
		return fmt.Errorf("wechat_virtualpay_go: accessToken 不能为空")
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
		return fmt.Errorf("wechat_virtualpay_go: Env %d 非法，取值只有 0（现网）/ 1（沙箱）", v)
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
		return fmt.Errorf("wechat_virtualpay_go: appKey 不能为空（Env=%d 须配%s AppKey——官方写明两把 key 不能混）", env, which)
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
