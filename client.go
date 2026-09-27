package wechat_virtualpay_go

import (
	"context"
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
	// AppSecret 小程序密钥。填了则由本包自动获取并刷新 access_token（推荐）。
	//
	// 内部走稳定版接口 POST /cgi-bin/stable_token 的普通模式：该模式下有效期内
	// **重复调用不会更新 token**，且与旧的 /cgi-bin/token **完全隔离**——因此多实例
	// 各持一份内存缓存是安全的，不需要分布式锁，也不需要集中式缓存。
	AppSecret string
	// AccessToken 自定义 token 来源。仅当 AppSecret 这条路走不通时才需要填，
	// 典型场景是第三方平台代商家调用（需要用 authorizer_access_token）。
	//
	// 与 AppSecret 同时提供时，以本字段为准。
	AccessToken func(ctx context.Context) (string, error)
	// HTTPClient 可选，默认使用带 10s 超时的 client。
	//
	// 需要拦截请求、自定义日志或转发到代理时，注入一个带自定义 Transport 的
	// client 即可——这是 Go 的惯用做法，本包不为此另设开关。
	HTTPClient *http.Client
}

// Client 是虚拟支付的客户端。
//
// 它是并发安全的：除配置外只持有一个自带互斥锁的 token 缓存（走 AppSecret 内置
// 获取时），所有可变的调用都通过 HTTPClient 完成。
type Client struct {
	cfg  Config
	http *http.Client
	// token 是已解析好的 token 来源：要么是调用方给的 AccessToken，
	// 要么是内部基于 AppSecret 构造的稳定版 tokenSource。
	token func(ctx context.Context) (string, error)
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
	if cfg.AccessToken == nil && cfg.AppSecret == "" {
		return nil, errors.New("wechat_virtualpay_go: 需要提供 AppSecret 或 AccessToken")
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}

	// token 来源：显式传入的 AccessToken 优先，其次用 AppSecret 构造内置来源。
	var token func(ctx context.Context) (string, error)
	if cfg.AccessToken != nil {
		token = cfg.AccessToken
	} else {
		token = (&tokenSource{appID: cfg.AppID, secret: cfg.AppSecret, http: hc}).Token
	}

	return &Client{cfg: cfg, http: hc, token: token}, nil
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
