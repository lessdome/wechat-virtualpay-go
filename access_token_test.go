package wechat_virtualpay_go

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// accessTokenRT 拦下换取凭证的请求，把「实际发出去的报文」原样记下来。
// 两个接口的 method / query / body 都不同，所以这几样都要看。
type accessTokenRT struct {
	mu          sync.Mutex
	status      int
	body        string
	calls       int
	lastMethod  string
	lastURL     string
	lastPath    string
	lastCT      string
	lastRawBody string
}

func (f *accessTokenRT) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastMethod = r.Method
	f.lastURL = r.URL.String()
	f.lastPath = r.URL.Path
	f.lastCT = r.Header.Get("Content-Type")
	// GET（旧接口）没有请求体，Body 是 nil——别直接丢给 io.ReadAll，那会空指针。
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}
	f.lastRawBody = string(body)
	st := f.status
	if st == 0 {
		st = 200
	}
	return &http.Response{
		StatusCode: st,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(f.body)),
	}, nil
}

// swapAccessTokenClient 替换包级的 tokenHTTPClient，测试结束恢复。
func swapAccessTokenClient(t *testing.T, rt *accessTokenRT) {
	t.Helper()
	old := tokenHTTPClient
	tokenHTTPClient = &http.Client{Transport: rt, Timeout: 5 * time.Second}
	t.Cleanup(func() { tokenHTTPClient = old })
}

// bothEndpoints 是两个换取接口的调用入口，用来把「两个接口共有的契约」在两边都验一遍。
func bothEndpoints() []struct {
	name string
	call func(context.Context, string, string) (*AccessTokenResponse, error)
} {
	return []struct {
		name string
		call func(context.Context, string, string) (*AccessTokenResponse, error)
	}{
		{"GetStableAccessToken", func(ctx context.Context, id, secret string) (*AccessTokenResponse, error) {
			return GetStableAccessToken(ctx, id, secret, false)
		}},
		{"GetAccessToken", GetAccessToken},
	}
}

// 稳定版：地址、方法、请求体逐字钉住——请求体跟官方示例一模一样（普通模式不带 force_refresh）。
func TestGetStableAccessTokenRequestShape(t *testing.T) {
	rt := &accessTokenRT{body: `{"access_token":"AT1","expires_in":7200}`}
	swapAccessTokenClient(t, rt)

	got, err := GetStableAccessToken(context.Background(), "wxapp", "s3cret", false)
	if err != nil {
		t.Fatal(err)
	}
	if rt.lastMethod != http.MethodPost {
		t.Errorf("方法应为 POST: %s", rt.lastMethod)
	}
	if rt.lastURL != "https://api.weixin.qq.com/cgi-bin/stable_token" {
		t.Errorf("地址或 query 不对（不该带任何参数）: %s", rt.lastURL)
	}
	if rt.lastCT != "application/json" {
		t.Errorf("Content-Type 不对: %s", rt.lastCT)
	}
	const wantBody = `{"grant_type":"client_credential","appid":"wxapp","secret":"s3cret"}`
	if rt.lastRawBody != wantBody {
		t.Errorf("请求体不对:\n 实际 %s\n 期望 %s", rt.lastRawBody, wantBody)
	}
	if got.AccessToken != "AT1" || got.ExpiresIn != 7200 || got.ErrCode != 0 {
		t.Fatalf("解析结果不对: %+v", got)
	}
}

// 旧接口：GET，参数全在 query，**没有请求体**——这是它与稳定版最容易搞混的地方
func TestGetAccessTokenRequestShape(t *testing.T) {
	rt := &accessTokenRT{body: `{"access_token":"AT0","expires_in":7200}`}
	swapAccessTokenClient(t, rt)

	got, err := GetAccessToken(context.Background(), "wxapp", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if rt.lastMethod != http.MethodGet {
		t.Errorf("方法应为 GET: %s", rt.lastMethod)
	}
	if rt.lastPath != "/cgi-bin/token" {
		t.Errorf("路径不对: %s", rt.lastPath)
	}
	if rt.lastRawBody != "" {
		t.Errorf("旧接口不该有请求体: %q", rt.lastRawBody)
	}
	for _, want := range []string{"appid=wxapp", "secret=s3cret", "grant_type=client_credential"} {
		if !strings.Contains(rt.lastURL, want) {
			t.Errorf("query 缺少 %s: %s", want, rt.lastURL)
		}
	}
	if got.AccessToken != "AT0" || got.ErrCode != 0 {
		t.Fatalf("解析结果不对: %+v", got)
	}
}

// forceRefresh=true：唯一的差别就是请求体多带一个 force_refresh（旧接口没有这个开关）
func TestGetStableAccessTokenForceRefresh(t *testing.T) {
	rt := &accessTokenRT{body: `{"access_token":"AT2","expires_in":7200}`}
	swapAccessTokenClient(t, rt)

	if _, err := GetStableAccessToken(context.Background(), "wxapp", "s3cret", true); err != nil {
		t.Fatal(err)
	}
	const wantBody = `{"grant_type":"client_credential","appid":"wxapp","secret":"s3cret","force_refresh":true}`
	if rt.lastRawBody != wantBody {
		t.Errorf("请求体不对:\n 实际 %s\n 期望 %s", rt.lastRawBody, wantBody)
	}
}

// 两个接口共有的契约：业务失败不是 error，errcode/errmsg 原值返回，凭证为空
func TestAccessTokenBusinessFailure(t *testing.T) {
	for _, ep := range bothEndpoints() {
		t.Run(ep.name, func(t *testing.T) {
			rt := &accessTokenRT{body: `{"errcode":40013,"errmsg":"invalid appid"}`}
			swapAccessTokenClient(t, rt)

			got, err := ep.call(context.Background(), "bad", "s")
			if err != nil {
				t.Fatalf("业务失败不该是 error: %v", err)
			}
			if got == nil {
				t.Fatal("业务失败也要把响应给出来")
			}
			if got.ErrCode != 40013 || got.ErrMsg != "invalid appid" {
				t.Errorf("errcode/errmsg 应原值返回: %+v", got)
			}
			if got.AccessToken != "" {
				t.Errorf("失败时不该有凭证: %+v", got)
			}
		})
	}
}

// 两个接口共有：errcode=0 却没回 access_token —— 报文不对，必须报错，
// 不然调用方会拿着一把空号去调业务接口
func TestAccessTokenMissingAccessToken(t *testing.T) {
	for _, ep := range bothEndpoints() {
		t.Run(ep.name, func(t *testing.T) {
			rt := &accessTokenRT{body: `{"expires_in":7200}`}
			swapAccessTokenClient(t, rt)

			if _, err := ep.call(context.Background(), "a", "s"); err == nil {
				t.Fatal("errcode=0 但没回 access_token，应当报错")
			}
		})
	}
}

// 两个接口共有：HTTP 层失败（非 200 时响应体不是微信报文，原始响应要带出来）
func TestAccessTokenHTTPError(t *testing.T) {
	for _, ep := range bothEndpoints() {
		t.Run(ep.name, func(t *testing.T) {
			rt := &accessTokenRT{status: 502, body: "bad gateway"}
			swapAccessTokenClient(t, rt)

			_, err := ep.call(context.Background(), "a", "s")
			if err == nil || !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "bad gateway") {
				t.Fatalf("期望带出 HTTP 502 与原始响应，实际: %v", err)
			}
		})
	}
}

// 两个接口共有：参数不合法时不该发出请求（也别去花那次配额）
func TestAccessTokenRejectsEmptyArgs(t *testing.T) {
	args := []struct {
		name          string
		appID, secret string
		want          string
	}{
		{"appID", "", "s", "appID"},
		{"appSecret", "a", "", "appSecret"},
	}
	for _, ep := range bothEndpoints() {
		for _, tc := range args {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				rt := &accessTokenRT{body: `{"access_token":"AT","expires_in":7200}`}
				swapAccessTokenClient(t, rt)

				if _, err := ep.call(context.Background(), tc.appID, tc.secret); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
				}
				if rt.calls != 0 {
					t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
				}
			})
		}
	}
}
