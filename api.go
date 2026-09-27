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

// responseHeader 是所有 /xpay 接口响应的公共部分。
//
// 微信开放接口成功时不返回 errcode；一旦出现非 0 的 errcode 即为失败。
// 各接口自己的响应结构体嵌入本类型以复用解析。
type responseHeader struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// authMode 描述一个接口需要哪种鉴权参数。
//
// 官方 33 个 /xpay 接口按 query 参数分为三级，本包照文档实现：
//
//	authTokenOnly    仅 access_token            —— 9 个商家级接口
//	authPaySig       access_token + pay_sig     —— 21 个
//	authUserAndPaySig access_token + signature + pay_sig —— 3 个用户态接口
//
// ⚠️ 存疑：authTokenOnly 那批（广告金 7 个 + notify_provide_goods +
// present_currency）的文档自相矛盾——请求体里的 env 字段注释写着「仅作为签名
// 校验」，但 query 参数表里并没有 pay_sig。本包暂按文档字面实现（不加签名），
// 待在沙箱/现网实测确认。若实测返回 -15006，把这些接口改回 authPaySig 即可。
type authMode int

const (
	authTokenOnly authMode = iota
	authPaySig
	authUserAndPaySig
)

// call 发起一次 /xpay 接口调用。
//
//   - uri：形如 "/xpay/query_order"，**不含** "?" 及其后的 query string。
//     这一点是硬性要求：pay_sig 的签名原文是 uri + "&" + 请求体，
//     uri 带上 query string 会导致签名与微信侧不一致（-15006）。
//   - body：请求体，会被序列化为 JSON。
//   - mode：鉴权级别，见 authMode。
//   - sessionKey：仅 authUserAndPaySig 需要（用户登录态，由 code2Session 获取）。
//   - out：响应体解析目标，可为 nil（只要成功与否）。
//
// 本方法的核心不变量：**参与 pay_sig 计算的字符串与真正发出去的请求体是同一个
// 字节序列**。因此这里只序列化一次，随后复用同一份 []byte，绝不二次序列化——
// 这是本包存在的首要理由（详见 json.go 的说明）。
func (c *Client) call(ctx context.Context, uri string, body any, mode authMode, sessionKey string, out any) error {
	raw, err := marshalNoHTMLEscape(body)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 序列化 %s 的请求体失败: %w", uri, err)
	}
	signData := string(raw)

	token, err := c.token(ctx)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 获取 access_token 失败: %w", err)
	}

	q := url.Values{}
	q.Set("access_token", token)

	// 除 authTokenOnly 外都需要支付签名。uri 不带 query string。
	if mode != authTokenOnly {
		q.Set("pay_sig", CalcPaySig(c.appKey(), uri, signData))
	}
	// 只有用户态接口需要用户签名 —— 注意 signature 不带 uri 前缀，与 pay_sig 不同。
	if mode == authUserAndPaySig {
		if sessionKey == "" {
			return fmt.Errorf("wechat_virtualpay_go: %s 需要 SessionKey（用户登录态），不能为空", uri)
		}
		q.Set("signature", CalcSignature(sessionKey, signData))
	}

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
		return &APIError{
			Code:    resp.StatusCode,
			Message: fmt.Sprintf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
			Raw:     rawResp,
		}
	}

	var hdr responseHeader
	if err := json.Unmarshal(rawResp, &hdr); err != nil {
		return fmt.Errorf("wechat_virtualpay_go: 解析 %s 响应失败: %w（原始响应: %s）", uri, err, rawResp)
	}
	if hdr.ErrCode != 0 {
		return &APIError{Code: hdr.ErrCode, Message: hdr.ErrMsg, Raw: rawResp}
	}

	if out != nil {
		if err := json.Unmarshal(rawResp, out); err != nil {
			return fmt.Errorf("wechat_virtualpay_go: 解析 %s 响应字段失败: %w（原始响应: %s）", uri, err, rawResp)
		}
	}
	return nil
}
