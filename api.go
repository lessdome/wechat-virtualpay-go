package wechat_virtualpay_go

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// defaultAPIBase 是微信开放接口的基础地址。
//
// 虚拟支付的 xpay 服务端接口走微信开放接口（access_token 鉴权），
// 而非微信支付 APIv3（api.mch.weixin.qq.com）。
const defaultAPIBase = "https://api.weixin.qq.com"

// responseHeader 是所有 /xpay 接口响应的公共部分。
//
// 微信开放接口成功时不返回 errcode；一旦出现非 0 的 errcode 即为失败。
// 各接口自己的响应结构体嵌入本类型以复用解析。
type responseHeader struct {
	// ErrCode 微信错误码。成功响应里没有这个字段，一旦非 0 即为失败。
	ErrCode int `json:"errcode"`
	// ErrMsg 错误信息。
	ErrMsg string `json:"errmsg"`
}

// 官方 33 个 /xpay 接口按鉴权分三级，对应下面三种调用方式：
//
//	callMerchant   仅 access_token                            9 个商家级接口
//	callPaySig     access_token + pay_sig                    21 个
//	callUser       access_token + pay_sig + signature         3 个用户态接口
//
// 分级依据是各接口文档「查询参数表」里**实际列了哪些参数**——33 个接口逐页核对过，
// 全部对得上。最粗的那一级一个签名都不带，因为它们是商家级的（广告金 7 个 +
// notify_provide_goods + present_currency），根本没有用户 session_key 可签。
//
// ⚠️ 但文档在这件事上不自洽，两处要留神：
//
//   - 2.5「签名详解」的总述写「用户态签名和支付签名在服务器 API 中都会涉及」，
//     容易被读成「每个接口都要两个签名」；实际是逐接口不同的。
//   - refund_order 与 get_complaint_list 的「注意事项」写「使用用户态签名与支付签名」，
//     而它们的参数表只列 pay_sig。本包按参数表实现。
//
// 另外，商家级那 9 个接口的请求体 env 注释里有一句「仅作为签名校验」，看着像要签名，
// 但那是**跨页复制的模板文字**——明确需要 pay_sig 的 query_biz_balance 页上同样有它。
//
// 真机联调时若某接口报 268490003（签名错误），把它换到高一级的调用方式即可
// （callMerchant → callPaySig → callUser）。

// 三种调用方式共同的约定：
//
//   - uri：形如 "/xpay/query_order"，**不含** "?" 及其后的 query string。
//     这一点是硬性要求：pay_sig 的签名原文是 uri + "&" + 请求体，
//     uri 带上 query string 会导致签名与微信侧不一致（服务端报 268490003 签名错误）。
//   - body：请求体，会被序列化为 JSON。
//   - out：响应体解析目标，可为 nil（只要成功与否）。
//
// 核心不变量：**参与 pay_sig 计算的字符串与真正发出去的请求体是同一个字节序列**。
// 因此每个入口都只序列化一次，随后复用同一份 []byte，绝不二次序列化——这是本包
// 存在的首要理由（详见 json.go 的说明）。token 失效时的重试（见 send）也复用同一份。

// callMerchant 调一个只带 access_token 的商家级接口。
func (c *Client) callMerchant(ctx context.Context, uri string, body any, out any) error {
	raw, err := marshalNoHTMLEscape(body)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 序列化 %s 的请求体失败: %w", uri, err)
	}
	return c.send(ctx, uri, raw, url.Values{}, out, false)
}

// callPaySig 调一个需要支付签名的接口：access_token + pay_sig。
func (c *Client) callPaySig(ctx context.Context, uri string, body any, out any) error {
	raw, err := marshalNoHTMLEscape(body)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 序列化 %s 的请求体失败: %w", uri, err)
	}
	q := url.Values{}
	q.Set("pay_sig", CalcPaySig(c.cfg.AppKey, uri, string(raw)))
	return c.send(ctx, uri, raw, q, out, false)
}

// callUser 调一个用户态接口：access_token + pay_sig + signature。
//
// sessionKey 由 wx.login 的 code 通过 code2Session 换取，必填。它是签名的一部分，
// 所以漏传**编译不过**——不会像运行时校验那样等到发请求才发现。
func (c *Client) callUser(ctx context.Context, uri string, body any, sessionKey string, out any) error {
	if sessionKey == "" {
		return fmt.Errorf("wechat_virtualpay_go: %s 需要 SessionKey（用户登录态），不能为空", uri)
	}
	raw, err := marshalNoHTMLEscape(body)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 序列化 %s 的请求体失败: %w", uri, err)
	}
	q := url.Values{}
	q.Set("pay_sig", CalcPaySig(c.cfg.AppKey, uri, string(raw)))
	// 注意 signature 不带 uri 前缀，与 pay_sig 不同。
	q.Set("signature", CalcSignature(sessionKey, string(raw)))
	return c.send(ctx, uri, raw, q, out, false)
}

// send 取一次 token、发一次请求、解析响应。
//
// raw 是**已经序列化好的**请求体，签名都由它算出——签名与请求体因此必然一致。
// q 里已经放好了这个接口需要的签名参数（可能一个都没有），access_token 由这里补上。
//
// retried 为 true 表示已经换过新 token 重试过了，不再重试——递归因此最多两层。
func (c *Client) send(ctx context.Context, uri string, raw []byte, q url.Values, out any, retried bool) error {
	token, err := accessToken(ctx, c.http, c.cfg.AppID, c.cfg.AppSecret)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 获取 access_token 失败: %w", err)
	}
	q.Set("access_token", token)

	endpoint := defaultAPIBase + uri + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 构造 %s 请求失败: %w", uri, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 请求 %s 失败: %w", uri, err)
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 读取 %s 响应失败: %w", uri, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wechat_virtualpay_go: %s 返回 HTTP %d %s（原始响应: %s）",
			uri, resp.StatusCode, http.StatusText(resp.StatusCode), rawResp)
	}

	var hdr responseHeader
	if err := json.Unmarshal(rawResp, &hdr); err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 解析 %s 响应失败: %w（原始响应: %s）", uri, err, rawResp)
	}
	if hdr.ErrCode != 0 {
		if isAccessTokenUnusable(hdr.ErrCode) {
			if !retried {
				// 微信说这个 access_token 不能用（多半被别处作废了）：作废缓存、
				// 重新生成一个，再调一次。递归只往下走这一层。
				//
				// 重发是安全的：这个错误码意味着请求在鉴权阶段就被拒了，没有产生
				// 业务副作用；而且请求体与签名与上一次逐字节相同。
				invalidateAccessToken(c.cfg.AppID)
				return c.send(ctx, uri, raw, q, out, true)
			}
			return fmt.Errorf("wechat_virtualpay_go: access_token 不可用（errcode=%d），换新后仍然失败（errmsg: %s）",
				hdr.ErrCode, hdr.ErrMsg)
		}
		// 带上微信的 errmsg：好几个码的说明就是「具体看 errmsg」，不能丢。
		return fmt.Errorf("wechat_virtualpay_go: errcode=%d %s（errmsg: %s）",
			hdr.ErrCode, ErrorCode(hdr.ErrCode).ErrorText(), hdr.ErrMsg)
	}

	if out != nil {
		if err := json.Unmarshal(rawResp, out); err != nil {
			return fmt.Errorf("wechat_virtualpay_go: 解析 %s 响应字段失败: %w（原始响应: %s）", uri, err, rawResp)
		}
	}
	return nil
}
