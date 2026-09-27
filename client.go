package wechat_virtualpay_go

import (
	"errors"
	"net/http"
	"time"
)

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
	// AccessToken 开放接口调用凭证，必填。
	//
	// ⚠️ 微信的 access_token **有效期约 2 小时**，且全局唯一、会互相顶掉。本包把它
	// 当静态值用，**不会替你刷新**——长驻服务需在过期前重建 Client（NewClient 很轻，
	// 不发网络请求）。多实例部署务必集中缓存，否则 A 实例刷新会让 B 实例手上的
	// token 立即失效。
	AccessToken string
	// HTTPClient 可选，默认使用带 10s 超时的 client。
	//
	// 需要拦截请求、自定义日志或转发到代理时，注入一个带自定义 Transport 的
	// client 即可——这是 Go 的惯用做法，本包不为此另设开关。
	HTTPClient *http.Client
}

// Client 是虚拟支付的客户端。
//
// 它是并发安全的：所有可变的调用都通过 HTTPClient 与 AccessToken 完成，
// Client 自身不持有可变状态。
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient 校验配置并构造 Client。
func NewClient(cfg Config) (*Client, error) {
	if cfg.AppID == "" {
		return nil, errors.New("wechat_virtualpay_go: AppID 不能为空")
	}
	if cfg.OfferID == "" {
		return nil, errors.New("wechat_virtualpay_go: OfferID 不能为空")
	}
	switch cfg.Env {
	case EnvProduction:
		if cfg.AppKey == "" {
			return nil, errors.New("wechat_virtualpay_go: 现网环境（EnvProduction）需要 AppKey")
		}
	case EnvSandbox:
		if cfg.SandboxKey == "" {
			return nil, errors.New("wechat_virtualpay_go: 沙箱环境（EnvSandbox）需要 SandboxKey")
		}
	default:
		return nil, errors.New("wechat_virtualpay_go: Env 非法，只能是 EnvProduction 或 EnvSandbox")
	}
	if cfg.AccessToken == "" {
		return nil, errors.New("wechat_virtualpay_go: AccessToken 不能为空")
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}

	return &Client{cfg: cfg, http: hc}, nil
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
