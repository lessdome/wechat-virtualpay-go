package wechat_virtualpay_go

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// 本文件实现「用 wx.login 的 code 换用户登录态」——也就是**用户级凭据**的获取。
//
// 它与 access_token.go（商家级凭据：AppID + AppSecret 换 access_token）是一对：两个文件都只
// 负责换号，换完把号交给调用方，谁都不替调用方把号塞进业务请求。
//
// 它是整条链路的**第一步**：虚拟支付的下单签名、三个用户态接口，都要 `session_key`。
// 链路是（官方《小程序登录凭证校验》一页）：
//
//	小程序端 wx.login() → code（有效期五分钟、且只能用一次）
//	        ↓ 把 code 交给开发者服务器
//	服务端 GET /sns/jscode2session → openid + session_key + unionid
//
// 注意它的鉴权方式与 /xpay/* 那些接口**完全不同**：这里直接用 appid + secret 换，
// 不需要 access_token。

// code2SessionURL 是官方给的完整地址（appid / secret / js_code / grant_type 都在
// query 上）。它不属于 /xpay/*，所以单独写在这里。
const code2SessionURL = "https://api.weixin.qq.com/sns/jscode2session"

// sessionHTTPClient 是换取登录态用的 HTTP client。
//
// 抽成包级变量是为了**测试能替换掉它**——本函数不做 client 注入，免得每次调用都要
// 多传一个参数。
var sessionHTTPClient = &http.Client{Timeout: 10 * time.Second}

// SessionInfo 是一次 code2Session 的结果。
//
// 公共头（errcode/errmsg）内嵌在最前面，读法与 /xpay/* 的响应一致：**err == nil 不等于
// 成功**，成功与否看 sess.ErrCode（0 才是成功，见 ResponseHeader）。
type SessionInfo struct {
	ResponseHeader
	// OpenID 用户在当前小程序的唯一标识。
	OpenID string
	// SessionKey 本次登录的会话密钥，用来算用户态签名 signature。
	SessionKey string
	// UnionID 用户在开放平台的唯一标识。**小程序未绑定开放平台时不会返回**，此时为空。
	UnionID string
}

// Code2Session 用 wx.login 的 code 换取用户登录态。
//
// 入参：
//
//	ctx        请求上下文，超时与取消由它管（client 另有 10 秒超时兜底）。
//	appID      小程序的 AppID（MP 后台「开发设置」里查看）。
//	appSecret  小程序的密钥（同一处查看与重置）。⚠️ 别和 AppKey 搞混——那是虚拟支付
//	           商户后台的，两者不通用。
//	code       小程序端 `wx.login()` 拿到的 code，**有效期五分钟且只能用一次**（这一条
//	           官方写明）。⚠️ 至于「换失败的那一次算不算已经用掉」，官方没写、本包也测不了：
//	           拿不准就别拿同一把 code 重试，让前端重新 wx.login 换一把更稳妥。
//
// 换出来的 SessionKey 给谁用：下单签名（BuildPayment，算 signature）与三个用户态接口
// （见 PostWithUserSig）。它是**会话级**凭据、不是每单一个，同一次登录态可以下多笔单；
// 它也会**过期**，过期后要让前端重新 wx.login。
//
// **返回值同样遵守本包的错误契约**：err 只表示这一趟没走通（参数不合法、连不上、非
// 200、不是 JSON），**微信的业务失败不是 error**——code 无效/已过期/已被使用时，返回的
// SessionInfo 里 ErrCode 非 0、OpenID 与 SessionKey 为空，err 是 nil：
//
//	sess, err := wechat_virtualpay_go.Code2Session(ctx, appID, appSecret, code)
//	if err != nil {
//		return err // 这一趟没走通
//	}
//	if sess.ErrCode != 0 {
//		// 业务失败：40029（code 无效）最常见，让前端重新 wx.login 再换一次
//	}
//
// 本接口自己的错误码只有 40029（code 无效），其余指向官方《通用错误码》一页。码就在
// sess.ErrCode 上，可以按值分支，不用去抠 error 的文案。
func Code2Session(ctx context.Context, appID, appSecret, code string) (*SessionInfo, error) {
	if appID == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: appID 不能为空")
	}
	if appSecret == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: appSecret 不能为空")
	}
	if code == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: code 不能为空（由小程序端 wx.login 获取）")
	}

	q := url.Values{}
	q.Set("appid", appID)
	q.Set("secret", appSecret)
	q.Set("js_code", code)
	q.Set("grant_type", "authorization_code")
	endpoint := code2SessionURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 构造 code2Session 请求失败: %w", stripURLError(err))
	}

	resp, err := sessionHTTPClient.Do(req)
	if err != nil {
		// 先摘掉 URL：appSecret 与 js_code 都挂在 query 上，Go 的错误文案会连它们一起
		// 带出来（见 stripURLError）。
		return nil, fmt.Errorf("wechat_virtualpay_go: 请求 code2Session 失败: %w", stripURLError(err))
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 读取 code2Session 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wechat_virtualpay_go: code2Session 返回 HTTP %d %s（原始响应: %s）",
			resp.StatusCode, http.StatusText(resp.StatusCode), raw)
	}

	// 公共头直接内嵌 ResponseHeader，不在这儿重抄一遍 errcode/errmsg 的 JSON 名——那样
	// 就多了一处会写错、会与 xpay 那边分叉的地方。
	var out struct {
		ResponseHeader
		OpenID     string `json:"openid"`
		SessionKey string `json:"session_key"`
		UnionID    string `json:"unionid"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 解析 code2Session 响应失败: %w（原始响应: %s）", err, raw)
	}
	if out.ErrCode != 0 {
		// **业务失败不是 error**（与 /xpay/* 同一条契约，见 xpay.go 文件头）：err 只有
		// 「这一趟没走通」一种含义，所以这里把 errcode/errmsg **原值**装进响应返回，
		// err 给 nil。于是调用方可以按值分支（40029 → 让前端重新 wx.login），
		// 而不是去 error 的文案里抠数字。
		//
		// 不在这儿解释错误码：写进库里的说明会过期，还可能和官方冲突——冲突时它就在
		// 你的代码里，还带自动补全，比文档更有说服力。含义查官方页面。
		return &SessionInfo{ResponseHeader: out.ResponseHeader}, nil
	}
	if out.OpenID == "" || out.SessionKey == "" {
		// errcode=0 却没回关键字段：这不是业务失败，是报文不对（也没有半个登录态可给），
		// 按「没拿到可用的响应」处理——err 非 nil、响应为 nil。
		return nil, fmt.Errorf("wechat_virtualpay_go: code2Session 未返回 openid 或 session_key（原始响应: %s）", raw)
	}

	return &SessionInfo{OpenID: out.OpenID, SessionKey: out.SessionKey, UnionID: out.UnionID}, nil
}
