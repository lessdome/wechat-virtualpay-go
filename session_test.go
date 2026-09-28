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

type sessionRT struct {
	mu       sync.Mutex
	status   int
	body     string
	calls    int
	lastURL  string
	lastPath string
}

func (f *sessionRT) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastURL = r.URL.String()
	f.lastPath = r.URL.Path
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

// swapClient 替换包级的 sessionHTTPClient，测试结束恢复。
func swapClient(t *testing.T, rt *sessionRT) {
	t.Helper()
	old := sessionHTTPClient
	sessionHTTPClient = &http.Client{Transport: rt, Timeout: 5 * time.Second}
	t.Cleanup(func() { sessionHTTPClient = old })
}

// 请求必须打对地址与参数——这条钉的是官方给的 URL 规格
func TestCode2SessionRequestShape(t *testing.T) {
	rt := &sessionRT{body: `{"openid":"o1","session_key":"sk1","unionid":"u1"}`}
	swapClient(t, rt)

	got, err := Code2Session(context.Background(), "wxapp", "secret1", "code1")
	if err != nil {
		t.Fatal(err)
	}
	if rt.lastPath != "/sns/jscode2session" {
		t.Fatalf("路径不对: %s", rt.lastPath)
	}
	if !strings.Contains(rt.lastURL, "api.weixin.qq.com") {
		t.Fatalf("域名不对: %s", rt.lastURL)
	}
	for _, want := range []string{"appid=wxapp", "secret=secret1", "js_code=code1", "grant_type=authorization_code"} {
		if !strings.Contains(rt.lastURL, want) {
			t.Errorf("query 缺少 %s: %s", want, rt.lastURL)
		}
	}
	if got.OpenID != "o1" || got.SessionKey != "sk1" || got.UnionID != "u1" {
		t.Fatalf("解析结果不对: %+v", got)
	}
}

// 小程序未绑定开放平台时没有 unionid，不该报错
func TestCode2SessionWithoutUnionID(t *testing.T) {
	rt := &sessionRT{body: `{"openid":"o1","session_key":"sk1"}`}
	swapClient(t, rt)

	got, err := Code2Session(context.Background(), "a", "b", "c")
	if err != nil {
		t.Fatalf("没有 unionid 不该报错: %v", err)
	}
	if got.UnionID != "" {
		t.Fatalf("UnionID 应为空: %+v", got)
	}
}

// 40029 code 无效：错误里要带上错误码与 errmsg
func TestCode2SessionInvalidCode(t *testing.T) {
	rt := &sessionRT{body: `{"errcode":40029,"errmsg":"invalid code"}`}
	swapClient(t, rt)

	_, err := Code2Session(context.Background(), "a", "b", "bad")
	if err == nil || !strings.Contains(err.Error(), "40029") || !strings.Contains(err.Error(), "invalid code") {
		t.Fatalf("期望带出 40029 与 errmsg，实际: %v", err)
	}
}

// errcode=0 但缺关键字段：必须报错，不能返回半个登录态
func TestCode2SessionMissingFields(t *testing.T) {
	for _, body := range []string{
		`{"openid":"o1"}`,
		`{"session_key":"sk1"}`,
	} {
		rt := &sessionRT{body: body}
		swapClient(t, rt)
		if _, err := Code2Session(context.Background(), "a", "b", "c"); err == nil {
			t.Errorf("响应 %s 缺关键字段，应当报错", body)
		}
	}
}

// HTTP 层失败
func TestCode2SessionHTTPError(t *testing.T) {
	rt := &sessionRT{status: 502, body: "bad gateway"}
	swapClient(t, rt)

	_, err := Code2Session(context.Background(), "a", "b", "c")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("期望带出 HTTP 502，实际: %v", err)
	}
}

// 参数不合法时不该发出请求
func TestCode2SessionRejectsEmptyArgs(t *testing.T) {
	cases := []struct {
		name             string
		appID, secret, c string
		want             string
	}{
		{"appID", "", "s", "c", "appID"},
		{"appSecret", "a", "", "c", "appSecret"},
		{"code", "a", "s", "", "code"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &sessionRT{body: `{"openid":"o","session_key":"s"}`}
			swapClient(t, rt)
			if _, err := Code2Session(context.Background(), tc.appID, tc.secret, tc.c); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望报错含 %q，实际: %v", tc.want, err)
			}
			if rt.calls != 0 {
				t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
			}
		})
	}
}
