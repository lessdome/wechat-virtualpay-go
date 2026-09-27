package virtualpay

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// TokenProvider 提供 access_token。
//
// 本包**不内置** token 的获取与缓存 —— 因为缓存策略（内存 / 文件 / Redis）
// 因部署形态而异，内置一种等于替使用者做决定。实现这个接口即可：
//
//	type RedisToken struct{ rdb *redis.Client; appID, secret string }
//
//	func (t *RedisToken) Token(ctx context.Context) (string, error) {
//		// 1. 先读缓存；2. 没有或快过期则调用 /cgi-bin/token 刷新；
//		// 3. 用分布式锁避免多实例并发刷新互相顶掉。
//	}
//
// 注意：微信的 access_token 全局唯一且会互相顶掉，**多实例部署务必集中缓存**。
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// Config 是 Client 的配置。
type Config struct {
	// AppID 小程序 AppID。
	AppID string
	// OfferID 虚拟支付商户号（在「虚拟支付 → 基本配置」获取）。
	OfferID string
	// AppKey 现网支付密钥。Env=EnvProduction 时必填。
	AppKey string
	// SandboxKey 沙箱支付密钥。Env=EnvSandbox 时必填。
	SandboxKey string
	// Env 环境，决定使用哪个密钥以及请求体里的 env 字段。
	Env Env
	// Tokens access_token 提供者，必填。
	Tokens TokenProvider
	// HTTPClient 可选，默认使用带 10s 超时的 client。
	HTTPClient *http.Client
	// BaseURL 可选，默认 https://api.weixin.qq.com；测试时可指向本地服务器。
	BaseURL string
	// Debug 可选，打印请求与响应。**绝不会打印 AppKey。**
	Debug bool
}

// Client 是虚拟支付的客户端。
//
// 它是并发安全的：所有可变的调用都通过 HTTPClient 与 TokenProvider 完成，
// Client 自身不持有可变状态。
type Client struct {
	cfg     Config
	http    *http.Client
	baseURL string
}

// NewClient 校验配置并构造 Client。
func NewClient(cfg Config) (*Client, error) {
	if cfg.AppID == "" {
		return nil, errors.New("virtualpay: AppID 不能为空")
	}
	if cfg.OfferID == "" {
		return nil, errors.New("virtualpay: OfferID 不能为空")
	}
	switch cfg.Env {
	case EnvProduction:
		if cfg.AppKey == "" {
			return nil, errors.New("virtualpay: 现网环境（EnvProduction）需要 AppKey")
		}
	case EnvSandbox:
		if cfg.SandboxKey == "" {
			return nil, errors.New("virtualpay: 沙箱环境（EnvSandbox）需要 SandboxKey")
		}
	default:
		return nil, errors.New("virtualpay: Env 非法，只能是 EnvProduction 或 EnvSandbox")
	}
	if cfg.Tokens == nil {
		return nil, errors.New("virtualpay: 需要提供 TokenProvider")
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultAPIBase
	}

	return &Client{cfg: cfg, http: hc, baseURL: base}, nil
}

// appKey 返回当前环境应使用的支付密钥。
//
// 收敛到一处，避免"现网用了沙箱 Key"这类高频事故。
func (c *Client) appKey() string {
	if c.cfg.Env == EnvSandbox {
		return c.cfg.SandboxKey
	}
	return c.cfg.AppKey
}

// envInt 返回请求体里的 env 字段值。
func (c *Client) envInt() int {
	if c.cfg.Env == EnvSandbox {
		return 1
	}
	return 0
}
