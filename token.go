package wechat_virtualpay_go

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// tokenRefreshMargin 是提前刷新的余量。
//
// 微信文档：稳定版接口在普通模式下会**提前 5 分钟**更新 access_token，
// 因此留同样的余量即可，不必等到真正过期才换。
const tokenRefreshMargin = 5 * time.Minute

// tokenSource 用「稳定版接口调用凭据」自动获取并缓存 access_token。
//
// 用的是 POST /cgi-bin/stable_token 的**普通模式**（不带 force_refresh）。该模式有
// 两个关键性质：
//
//  1. 有效期内**重复调用不会更新** token；
//  2. 与旧的 GET /cgi-bin/token **完全隔离，互不影响**。
//
// 因此多个实例各持一份内存缓存是安全的——不会互相顶掉，也就不需要分布式锁或
// 集中式缓存。这正是选择稳定版而非旧接口的原因。
type tokenSource struct {
	appID  string
	secret string
	http   *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// Token 返回当前可用的 access_token，必要时刷新。
func (t *tokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 串行化并发调用：微信对 stable_token 有频率限制（1 万次/分钟），并发去刷
	// 没有意义，只会浪费配额、且拿回来的还是同一个 token。
	if t.token != "" && time.Now().Before(t.expiresAt) {
		return t.token, nil
	}

	token, lifetime, err := t.fetch(ctx)
	if err != nil {
		return "", err
	}

	// 余量不超过有效期的一半，避免 expires_in 异常偏小时反复刷新。
	margin := tokenRefreshMargin
	if margin > lifetime/2 {
		margin = lifetime / 2
	}
	t.token = token
	t.expiresAt = time.Now().Add(lifetime - margin)
	return token, nil
}

// stableTokenRequest 是 POST /cgi-bin/stable_token 的请求体。
//
// 刻意不带 force_refresh：不传即为 false（普通模式），这正是我们要的。
// 强制刷新会让上次的 token 立即失效，且每天限 20 次。
type stableTokenRequest struct {
	GrantType string `json:"grant_type"`
	AppID     string `json:"appid"`
	Secret    string `json:"secret"`
}

func (t *tokenSource) fetch(ctx context.Context) (string, time.Duration, error) {
	body, err := marshalNoHTMLEscape(stableTokenRequest{
		GrantType: "client_credential",
		AppID:     t.appID,
		Secret:    t.secret,
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

	resp, err := t.http.Do(req)
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
