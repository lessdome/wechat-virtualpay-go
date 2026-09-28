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

// Client 是虚拟支付的客户端。
//
// 它是并发安全的：只持配置，不带任何可变状态（access_token 的缓存在包级，
// 见 access_token.go）。所有对外的调用都通过 HTTPClient 完成。
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
