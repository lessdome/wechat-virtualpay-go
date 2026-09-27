package virtualpay

import (
	"context"
	"strings"
	"testing"
)

type stubTokens struct{}

func (stubTokens) Token(_ context.Context) (string, error) { return "tok", nil }

func testClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(Config{
		AppID:   "wxtest0000000000",
		OfferID: "1234567890",
		AppKey:  "test_app_key_1234567890",
		Env:     EnvProduction,
		Tokens:  stubTokens{},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// TestBuildPaymentParams_MatchesVector 是端到端断言：
// 用 BuildPaymentParams 产出的 SignData 与 PaySig，必须等于 Python 交叉验证的向量。
// 它把「参数构建 → 序列化 → 签名」整条链路锁死。
func TestBuildPaymentParams_MatchesVector(t *testing.T) {
	c := testClient(t)

	params, err := c.BuildPaymentParams(PrepayRequest{
		ProductID:  "a<b>&c",
		GoodsPrice: 100,
		OutTradeNo: "ORDER20260101002",
		Attach:     "x&y",
		SessionKey: "test_session_key_abcdef",
	})
	if err != nil {
		t.Fatalf("BuildPaymentParams: %v", err)
	}

	const wantSignData = `{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY","productId":"a<b>&c","goodsPrice":100,"outTradeNo":"ORDER20260101002","attach":"x&y"}`
	if params.SignData != wantSignData {
		t.Fatalf("SignData = %s\nwant      = %s", params.SignData, wantSignData)
	}
	if params.PaySig != "540b7ce535b85c51f5419207c1a833f10765d1050254f21a1cfa23a2bcf4e271" {
		t.Fatalf("PaySig = %s", params.PaySig)
	}
	if params.Signature != "26c3c7d42d4edb1c86f744c67ce45945028520ad3295e4465ecccfa5f8f3a071" {
		t.Fatalf("Signature = %s", params.Signature)
	}
	if params.Mode != ModeShortSeriesGoods {
		t.Fatalf("Mode 默认值错误: %s", params.Mode)
	}
}

func TestBuildPaymentParams_Defaults(t *testing.T) {
	c := testClient(t)
	params, err := c.BuildPaymentParams(PrepayRequest{
		ProductID:  "prod_001",
		GoodsPrice: 100,
		OutTradeNo: "ORDER20260101003",
		SessionKey: "sk",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Quantity 留空应默认为 1
	if !strings.Contains(params.SignData, `"buyQuantity":1`) {
		t.Fatalf("SignData 应包含 buyQuantity:1，实际 %s", params.SignData)
	}
}

func TestValidatePrepay(t *testing.T) {
	c := testClient(t)
	base := PrepayRequest{
		ProductID:  "prod_001",
		GoodsPrice: 100,
		OutTradeNo: "ORDER20260101001",
		SessionKey: "sk",
	}
	cases := []struct {
		name    string
		mutate  func(*PrepayRequest)
		wantErr bool
	}{
		{"合法", func(*PrepayRequest) {}, false},
		{"缺 SessionKey", func(r *PrepayRequest) { r.SessionKey = "" }, true},
		{"OutTradeNo 太短", func(r *PrepayRequest) { r.OutTradeNo = "abc" }, true},
		{"OutTradeNo 以下划线开头", func(r *PrepayRequest) { r.OutTradeNo = "_ORDER0001" }, true},
		{"OutTradeNo 含非法字符", func(r *PrepayRequest) { r.OutTradeNo = "ORDER#0011" }, true},
		{"道具直购缺 ProductID", func(r *PrepayRequest) { r.ProductID = "" }, true},
		{"道具直购缺 GoodsPrice", func(r *PrepayRequest) { r.GoodsPrice = 0 }, true},
		{"代币充值可不传 ProductID/GoodsPrice", func(r *PrepayRequest) {
			r.Mode = ModeShortSeriesCoin
			r.ProductID = ""
			r.GoodsPrice = 0
		}, false},
		{"优惠价合法（恰好 40%）", func(r *PrepayRequest) { r.ActivitySellingPrice = 40 }, false},
		{"优惠价过低（39%）", func(r *PrepayRequest) { r.ActivitySellingPrice = 39 }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			tc.mutate(&req)
			_, err := c.BuildPaymentParams(req)
			if tc.wantErr && err == nil {
				t.Fatal("期望报错，实际通过")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望通过，实际报错: %v", err)
			}
		})
	}
}

func TestNewClientValidation(t *testing.T) {
	base := Config{AppID: "a", OfferID: "b", AppKey: "k", Env: EnvProduction, Tokens: stubTokens{}}
	if _, err := NewClient(base); err != nil {
		t.Fatalf("完整配置应构造成功: %v", err)
	}
	cases := map[string]Config{
		"缺 AppID":        {OfferID: "b", AppKey: "k", Env: EnvProduction, Tokens: stubTokens{}},
		"缺 OfferID":      {AppID: "a", AppKey: "k", Env: EnvProduction, Tokens: stubTokens{}},
		"现网缺 AppKey":     {AppID: "a", OfferID: "b", Env: EnvProduction, Tokens: stubTokens{}},
		"沙箱缺 SandboxKey": {AppID: "a", OfferID: "b", Env: EnvSandbox, Tokens: stubTokens{}},
		"缺 Tokens":       {AppID: "a", OfferID: "b", AppKey: "k", Env: EnvProduction},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewClient(cfg); err == nil {
				t.Fatal("期望报错，实际通过")
			}
		})
	}
}
