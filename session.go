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

// 本文件实现「用 wx.login 的 code 换用户登录态」。
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
type SessionInfo struct {
	// OpenID 用户在当前小程序的唯一标识。
	OpenID string
	// SessionKey 本次登录的会话密钥，用来算用户态签名 signature。
	SessionKey string
	// UnionID 用户在开放平台的唯一标识。**小程序未绑定开放平台时不会返回**，此时为空。
	UnionID string
}

// Code2Session 用 wx.login 的 code 换取用户登录态。
//
// code 由小程序端 `wx.login()` 获取，**有效期五分钟且只能用一次**；换来的
// SessionKey 会过期，过期后要让前端重新 wx.login。
//
// 本接口自己的错误码只有 40029（code 无效），其余指向官方《通用错误码》一页
// （常见的有 code 已过期、code 已被使用）。
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
		return nil, fmt.Errorf("wechat_virtualpay_go: 构造 code2Session 请求失败: %w", err)
	}

	resp, err := sessionHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 请求 code2Session 失败: %w", err)
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

	var out struct {
		OpenID     string `json:"openid"`
		SessionKey string `json:"session_key"`
		UnionID    string `json:"unionid"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 解析 code2Session 响应失败: %w（原始响应: %s）", err, raw)
	}
	if out.ErrCode != 0 {
		// 40029 是本接口自己的错误码；其余要走官方《通用错误码》一页查。
		return nil, fmt.Errorf("wechat_virtualpay_go: code2Session 失败 errcode=%d errmsg=%s（40029 为 code 无效；其余见官方通用错误码表）",
			out.ErrCode, out.ErrMsg)
	}
	if out.OpenID == "" || out.SessionKey == "" {
		return nil, fmt.Errorf("wechat_virtualpay_go: code2Session 未返回 openid 或 session_key（原始响应: %s）", raw)
	}

	return &SessionInfo{OpenID: out.OpenID, SessionKey: out.SessionKey, UnionID: out.UnionID}, nil
}
