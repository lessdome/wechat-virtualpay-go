package wechat_virtualpay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// 本文件是**应用级凭证**的获取：AppID + AppSecret 换 access_token。
//
// 官方有两个接口都能换，本包两个都实现——因为两边都会指错路：
//
//	GetStableAccessToken  POST /cgi-bin/stable_token   推荐用这个
//	GetAccessToken        GET  /cgi-bin/token          旧接口
//
// 官方《access_token 使用说明》原话是「两者都可以正常获取，**推荐使用稳定版接口**」。但虚拟
// 支付那 33 个接口页上 `access_token` 的链接指向的是**旧**接口页（那一页顶上自己又写着「推荐
// 使用 获取稳定版接口调用凭据」）——照页面点进去会落到旧接口，这就是两个都留着的原因。
//
// 差别是「重复获取会不会把上一把弄失效」：
//
//   - 旧接口：使用说明写明「**重复获取将导致上次获取的 access_token 失效**」，所以官方要求
//     「中控服务器统一获取和刷新，其他业务逻辑服务器从存储里取」；多实例各自刷新会互相踢，
//     报 40001 invalid credential … not latest。
//   - 稳定版：与旧接口**完全隔离、互不影响**；普通模式下有效期内重复调用**不会换号**——所以
//     多实例各持一份内存缓存就是安全的，不需要中控，也不需要 Redis。
//
// 用户级那把钥匙（session_key）在 session.go，不是这个文件的事；第三方平台代商家调用的
// authorizer_access_token 两个接口都换不了（官方：本接口不支持第三方平台调用），本包也不做。
//
// 只换号、不缓存：存哪儿由调用方按自己的进程模型决定。

// stableTokenURL / accessTokenURL 是官方给的两个地址。稳定版只支持 POST，旧接口只支持 GET。
const (
	stableTokenURL = "https://api.weixin.qq.com/cgi-bin/stable_token"
	accessTokenURL = "https://api.weixin.qq.com/cgi-bin/token"
)

// tokenHTTPClient 换凭证用的 client，抽成包级变量只为测试能替换掉它。
var tokenHTTPClient = &http.Client{Timeout: 10 * time.Second}

// AccessTokenResponse 是换取凭证的响应。**两个接口的返回体逐字相同**（access_token /
// expires_in），所以共用一个类型，不各写一个——那样只会多一处会写歪的地方。
//
// 公共头（errcode/errmsg）内嵌在最前面，成败看 resp.ErrCode（0 才是成功）。
type AccessTokenResponse struct {
	ResponseHeader
	// AccessToken 获取到的凭证，业务失败时为空。
	AccessToken string `json:"access_token"`
	// ExpiresIn 有效时间，单位秒（目前 7200 之内）。
	//
	// ⚠️ 拿它做缓存**别卡满**：官方写明普通模式下平台会提前 5 分钟更新 token（使用说明里
	// 那 5 分钟新老两把都可用），留出余量再取更稳。
	ExpiresIn int `json:"expires_in"`
}

// GetStableAccessToken 换一把 access_token（**稳定版接口**，优先用它）。
//
// 与 GetAccessToken 的差别见文件头：两者完全隔离，且普通模式下有效期内重复调用不会换号，
// 多实例各持一份内存缓存即可，不必搞中控。
//
// 入参：
//
//	ctx           请求上下文，超时与取消由它管（client 另有 10 秒超时兜底）。
//	appID         小程序的 AppID。
//	appSecret     小程序的密钥。
//	forceRefresh  **传 false 就对了**（普通模式）。传 true 是强制刷新，⚠️ 慎用：官方写明
//	              它会让**上一个 access_token 立刻失效**（多实例部署时，别的实例手上那把
//	              就此作废，它下次调用会拿到 40001），而且每天限用 20 次、连续使用还要
//	              间隔 30 秒。正常路径永远不该传 true——这个开关只留给「凭据泄露、必须
//	              立刻作废」这类运维场合（旧接口没有这个能力）。
//
// 返回值契约与 /xpay/* 一致：err 只表示这一趟没走通（参数不合法、连不上、非 200、不是
// JSON）；**业务失败不是 error**——appid/secret 不对时响应里 ErrCode 非 0（40013 = appid
// 无效、40125 = secret 无效、40164 = 调用 IP 不在白名单），err 是 nil。
//
//	resp, err := wechat_virtualpay.GetStableAccessToken(ctx, appID, appSecret, false)
//	if err != nil {
//		return err // 这一趟没走通
//	}
//	if resp.ErrCode != 0 {
//		return fmt.Errorf("换 access_token 失败: %d %s", resp.ErrCode, resp.ErrMsg)
//	}
//	// 换来的号自己拿着传进业务接口，本包不替你塞。
//	QueryOrder(ctx, resp.AccessToken, appKey, req)
func GetStableAccessToken(ctx context.Context, appID, appSecret string, forceRefresh bool) (*AccessTokenResponse, error) {
	if err := checkCredentials(appID, appSecret); err != nil {
		return nil, err
	}

	// 字段顺序照官方请求参数表。force_refresh 带 omitempty：官方示例的普通模式就是**不传**
	// 它（默认即 false），所以 forceRefresh=false 时发出去的报文与官方示例逐字一致。
	body := struct {
		GrantType    string `json:"grant_type"`
		AppID        string `json:"appid"`
		Secret       string `json:"secret"`
		ForceRefresh bool   `json:"force_refresh,omitempty"`
	}{GrantType: "client_credential", AppID: appID, Secret: appSecret, ForceRefresh: forceRefresh}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay: GetStableAccessToken 序列化请求体失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stableTokenURL, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay: GetStableAccessToken 构造请求失败: %w", stripURLError(err))
	}
	req.Header.Set("Content-Type", "application/json")

	rawResp, err := tokenHTTPDo(req, "GetStableAccessToken")
	if err != nil {
		return nil, err
	}
	return parseAccessToken(rawResp)
}

// GetAccessToken 换一把 access_token（**旧接口** GET /cgi-bin/token）。
//
// 入参：
//
//	ctx        请求上下文，超时与取消由它管（client 另有 10 秒超时兜底）。
//	appID      小程序的 AppID。
//	appSecret  小程序的密钥。
//
// 注意它**没有** forceRefresh 这类开关：旧接口没有强制刷新的能力，每次调用都会换一把新
// 的——这正是它「重复获取会让上一把立刻失效」的由来，不是本包少封了一个参数。
//
// 能用，但优先用 GetStableAccessToken。这个接口的经典坑是**重复获取会让上一把立刻失效**：
// 多实例各自刷新会互相踢（40001 invalid credential … not latest），所以官方要求「中控
// 服务器统一获取和刷新，其他业务逻辑服务器从存储里取」。部署里没有这样一个中控，就用稳定版。
//
// 另外两条官方错误码：调用 IP 不在白名单 → 40164；AppSecret 被冻结 → 40243。
// 返回值契约同 GetStableAccessToken。
func GetAccessToken(ctx context.Context, appID, appSecret string) (*AccessTokenResponse, error) {
	if err := checkCredentials(appID, appSecret); err != nil {
		return nil, err
	}

	// 旧接口是 GET，参数全在 query 上，没有请求体（官方参数表：请求体「无」）。
	q := url.Values{
		"appid":      {appID},
		"secret":     {appSecret},
		"grant_type": {"client_credential"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, accessTokenURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay: GetAccessToken 构造请求失败: %w", stripURLError(err))
	}
	rawResp, err := tokenHTTPDo(req, "GetAccessToken")
	if err != nil {
		return nil, err
	}
	return parseAccessToken(rawResp)
}

// checkCredentials 校验两个接口共用的必填参数。
func checkCredentials(appID, appSecret string) error {
	if appID == "" {
		return fmt.Errorf("wechat_virtualpay: appID 不能为空")
	}
	if appSecret == "" {
		return fmt.Errorf("wechat_virtualpay: appSecret 不能为空")
	}
	return nil
}

// tokenHTTPDo 发一次请求并把响应体读出来。两个接口共用，所以 uri 只是个**报错时用的名字**
// （调用方函数名），这样 error 里看得出是哪条路走的。
func tokenHTTPDo(req *http.Request, name string) ([]byte, error) {
	resp, err := tokenHTTPClient.Do(req)
	if err != nil {
		// 先摘掉 URL：旧接口把 secret 挂在 query 上，Go 的错误文案会连它一起带出来
		// （见 stripURLError）。
		return nil, fmt.Errorf("wechat_virtualpay: %s 请求失败: %w", name, stripURLError(err))
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay: %s 读取响应失败: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK {
		// 非 200 时响应体不是微信的报文，状态码只能进文案；原始响应一并带上（网关的 502
		// 页面里常写着真正的原因）。
		return nil, fmt.Errorf("wechat_virtualpay: %s 返回 HTTP %d %s（原始响应: %s）",
			name, resp.StatusCode, http.StatusText(resp.StatusCode), rawResp)
	}
	return rawResp, nil
}

// parseAccessToken 把响应体收尾成 AccessTokenResponse。两个接口共用——它们的返回体一模一样，
// 成败判定自然也该一模一样。
func parseAccessToken(rawResp []byte) (*AccessTokenResponse, error) {
	var out AccessTokenResponse
	if err := json.Unmarshal(rawResp, &out); err != nil {
		return nil, fmt.Errorf("wechat_virtualpay: 解析响应失败: %w（原始响应: %s）", err, rawResp)
	}
	if out.ErrCode != 0 {
		return &out, nil // 业务失败不是 error：errcode/errmsg 原值返回
	}
	if out.AccessToken == "" {
		// errcode=0 却没回凭证：报文不对，按「没拿到可用的响应」处理——放过去，调用方会
		// 拿着一把空号去调业务接口。
		return nil, fmt.Errorf("wechat_virtualpay: 响应里没有 access_token（原始响应: %s）", rawResp)
	}
	return &out, nil
}
