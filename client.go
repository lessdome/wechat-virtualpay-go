package wechat_virtualpay_go

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Config 是 Client 的配置。
type Config struct {
	// AppID 小程序 AppID。
	AppID string
	// OfferID 虚拟支付商户号（在「虚拟支付 → 基本配置」获取）。
	OfferID string
	// AppKey 支付密钥，必填。
	AppKey string
	// AppSecret 小程序密钥。本包据此自动获取并缓存 access_token，必填。
	//
	// 内部走稳定版接口 POST /cgi-bin/stable_token 的普通模式：该模式下有效期内
	// **重复调用不会更新 access_token**，且与旧的 /cgi-bin/token **完全隔离**——因此多实例
	// 各持一份内存缓存是安全的，不需要分布式锁，也不需要集中式缓存。
	AppSecret string
	// HTTPClient 可选，默认使用带 10s 超时的 client。
	//
	// 需要拦截请求、自定义日志或转发到代理时，注入一个带自定义 Transport 的
	// client 即可——这是 Go 的惯用做法，本包不为此另设开关。
	HTTPClient *http.Client
}

// Client 是虚拟支付的客户端。零值不可用，请用 NewClient 构造。
//
// 它是并发安全的：除配置外只持有一份自带互斥锁的 access_token 缓存。
type Client struct {
	cfg  Config
	http *http.Client

	// 以下是 access_token 的缓存，零值即可用，由 accessTokenMu 保护。
	accessTokenMu        sync.Mutex
	cachedAccessToken    string
	accessTokenExpiresAt time.Time
}

// NewClient 校验配置并构造 Client。
func NewClient(cfg Config) (*Client, error) {
	if cfg.AppID == "" {
		return nil, errors.New("wechat_virtualpay_go: AppID 不能为空")
	}
	if cfg.OfferID == "" {
		return nil, errors.New("wechat_virtualpay_go: OfferID 不能为空")
	}
	if cfg.AppKey == "" {
		return nil, errors.New("wechat_virtualpay_go: AppKey 不能为空")
	}
	if cfg.AppSecret == "" {
		return nil, errors.New("wechat_virtualpay_go: AppSecret 不能为空")
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}

	return &Client{cfg: cfg, http: hc}, nil
}

// accessTokenRefreshMargin 是提前刷新的余量。
//
// 拿到新 access_token 后按 (expires_in − margin) 缓存，而不是等它真正过期才换。
// 微信文档说普通模式下平台会**提前 5 分钟**更新 access_token；这里刻意留
// **10 分钟**，是它的两倍——给网络往返和时钟偏差冗余，避免刚好卡在到期临界点
// 上请求被阻塞。对 7200 秒的有效期来说，等于每 1 小时 50 分钟换一次。
const accessTokenRefreshMargin = 10 * time.Minute

// accessToken 返回当前可用的 access_token，必要时刷新。
//
// 用 POST /cgi-bin/stable_token 的**普通模式**（不带 force_refresh）。该模式有两个
// 关键性质：
//
//  1. 有效期内**重复调用不会更新** access_token；
//  2. 与旧的 GET /cgi-bin/token **完全隔离，互不影响**。
//
// 因此多个实例各持一份内存缓存是安全的——不会互相顶掉，也就不需要分布式锁或
// 集中式缓存。这正是选择稳定版而非旧接口的原因。
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.accessTokenMu.Lock()
	defer c.accessTokenMu.Unlock()

	// 串行化并发调用：微信对 stable_token 有频率限制（1 万次/分钟），并发去刷
	// 没有意义，只会浪费配额、且拿回来的还是同一个 access_token。
	if c.cachedAccessToken != "" && time.Now().Before(c.accessTokenExpiresAt) {
		return c.cachedAccessToken, nil
	}

	accessToken, lifetime, err := c.fetchAccessToken(ctx)
	if err != nil {
		return "", err
	}

	// 余量不超过有效期的一半，避免 expires_in 异常偏小时反复刷新。
	margin := accessTokenRefreshMargin
	if margin > lifetime/2 {
		margin = lifetime / 2
	}
	c.cachedAccessToken = accessToken
	c.accessTokenExpiresAt = time.Now().Add(lifetime - margin)
	return accessToken, nil
}

// stableAccessTokenRequest 是 POST /cgi-bin/stable_token 的请求体。
//
// 刻意不带 force_refresh：不传即为 false（普通模式），这正是我们要的。
// 强制刷新会让上次的 access_token 立即失效，且每天限 20 次。
type stableAccessTokenRequest struct {
	GrantType string `json:"grant_type"`
	AppID     string `json:"appid"`
	Secret    string `json:"secret"`
}

func (c *Client) fetchAccessToken(ctx context.Context) (string, time.Duration, error) {
	body, err := marshalNoHTMLEscape(stableAccessTokenRequest{
		GrantType: "client_credential",
		AppID:     c.cfg.AppID,
		Secret:    c.cfg.AppSecret,
	})
	if err != nil {
		return "", 0, fmt.Errorf("wechat_virtualpay_go: 序列化 stable_token 请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		defaultAPIBase+"/cgi-bin/stable_token", bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("wechat_virtualpay_go: 构造 stable_token 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("wechat_virtualpay_go: 请求 stable_token 失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("wechat_virtualpay_go: 读取 stable_token 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", 0, &APIError{
			Code:    resp.StatusCode,
			Message: fmt.Sprintf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
			Raw:     raw,
		}
	}

	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", 0, fmt.Errorf("wechat_virtualpay_go: 解析 stable_token 响应失败: %w（原始响应: %s）", err, raw)
	}
	if out.ErrCode != 0 {
		return "", 0, &APIError{Code: out.ErrCode, Message: out.ErrMsg, Raw: raw}
	}
	if out.AccessToken == "" {
		return "", 0, fmt.Errorf("wechat_virtualpay_go: stable_token 未返回 access_token（原始响应: %s）", raw)
	}
	if out.ExpiresIn <= 0 {
		// 文档说目前是 7200 秒之内；异常值按文档值兜底，避免缓存永不过期。
		out.ExpiresIn = 7200
	}
	return out.AccessToken, time.Duration(out.ExpiresIn) * time.Second, nil
}
