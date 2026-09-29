package wechat_virtualpay_go

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// 传输层失败时，error 文案里**不能**出现凭据。
//
// 三个接口把凭据挂在 URL 的 query 上（旧换号接口的 secret、jscode2session 的 secret、
// /xpay/* 的 access_token），而 Go 的 http.Client 会把**整个 URL 连 query** 写进
// url.Error 的文案——原样带出去就等于把 AppSecret 送进调用方的日志与错误上报。
//
// 这张表刻意只收「凭据在 URL 上」的那三条：GetStableAccessToken 的 secret 在**请求体**
// 里，报文不进 error，把它加进来这条测试对它永远成立——无意义地绿，比没有更坏。
func TestTransportErrorDoesNotLeakCredentials(t *testing.T) {
	const secret = "S3CRET-MUST-NOT-APPEAR"
	netErr := errors.New("dial tcp: lookup api.weixin.qq.com: no such host")

	// 三个包级 client 各换一次，注入同一个传输层错误。
	oldToken, oldSession, oldXpay := tokenHTTPClient, sessionHTTPClient, xpayHTTPClient
	tokenHTTPClient = &http.Client{Transport: errRT{err: netErr}}
	sessionHTTPClient = &http.Client{Transport: errRT{err: netErr}}
	xpayHTTPClient = &http.Client{Transport: errRT{err: netErr}}
	t.Cleanup(func() {
		tokenHTTPClient, sessionHTTPClient, xpayHTTPClient = oldToken, oldSession, oldXpay
	})

	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
	}{
		{"GetAccessToken", func() error { // secret 在 query 上
			_, err := GetAccessToken(ctx, "wxapp", secret)
			return err
		}},
		{"Code2Session", func() error { // secret 在 query 上
			_, err := Code2Session(ctx, "wxapp", secret, "code123")
			return err
		}},
		{"QueryOrder", func() error { // access_token 在 query 上（第三个参数故意用 secret 顶）
			_, err := QueryOrder(ctx, secret, "appkey", QueryOrderRequest{OpenID: "oXXXX", OrderID: "order_1"})
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("传输层失败应当报错")
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("凭据泄漏进了 error: %v", err)
			}
			// 摘掉 URL 外壳不能把错误链摘断：是不是网络错、超没超时，是调用方重试策略
			// 要用的**值**（见 xpaySend 的注释），必须还能 errors.Is 到底。
			if !strings.Contains(err.Error(), netErr.Error()) {
				t.Errorf("底层失败原因不该丢: %v", err)
			}
			if !errors.Is(err, netErr) {
				t.Errorf("errors.Is 应该能走到底层错误: %v", err)
			}
		})
	}
}

// 构造请求就失败时，文案里同样不能出现凭据：uri 由调用方传，带个控制字符就能让
// url.Parse 失败，而它回的也是 url.Error——一样带出整个 URL（含 access_token）。
func TestBadURIDoesNotLeakAccessToken(t *testing.T) {
	const token = "ACCESS-TOKEN-MUST-NOT-APPEAR"
	var out NotifyProvideGoodsResponse

	// 这个 uri 不是本包拼的，是调用方给的；\n 让 url.Parse 直接失败，压根发不出去。
	err := PostWithPaySig(context.Background(), token, "appkey", "/xpay/query_order\n", struct{}{}, &out)
	if err == nil {
		t.Fatal("非法 uri 应当报错")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("凭据泄漏进了 error: %v", err)
	}
}

// stripURLError 只剥外壳：内层是什么就还什么。
func TestStripURLErrorKeepsInner(t *testing.T) {
	inner := errors.New("boom")

	// 不是 url.Error 的原样返回（别顺手把别人的错误换掉）。
	if got := stripURLError(inner); got != inner {
		t.Errorf("非 url.Error 应当原样返回，实际: %v", got)
	}
}
