package wechat_virtualpay_go

// Env 表示虚拟支付环境。
//
// 它同时决定两件事：请求体里的 env 字段（0/1），以及该用哪个 AppKey。
// 混用现网/沙箱密钥是高频错误，本包用 Client.appKey() 把它收敛到一处。
type Env int

const (
	// EnvProduction 现网环境（请求体 env=0）。使用 AppKey。
	EnvProduction Env = 0
	// EnvSandbox 沙箱环境（请求体 env=1）。使用 SandboxKey，
	// 且仅支持开发版/体验版小程序。
	EnvSandbox Env = 1
)

func (e Env) String() string {
	switch e {
	case EnvProduction:
		return "production"
	case EnvSandbox:
		return "sandbox"
	default:
		return "unknown"
	}
}

// defaultAPIBase 是微信开放接口的基础地址。
//
// 虚拟支付的 xpay 服务端接口走微信开放接口（access_token 鉴权），
// 而非微信支付 APIv3（api.mch.weixin.qq.com）。
// 可用 Config.BaseURL 覆盖（测试时指向本地假服务器）。
const defaultAPIBase = "https://api.weixin.qq.com"
