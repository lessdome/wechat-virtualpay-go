package virtualpay

import (
	"encoding/json"
	"strings"
	"testing"
)

// prepayBodyForTest 与真实下单请求体的字段顺序保持一致（顺序决定签名字符串）。
type prepayBodyForTest struct {
	OfferID      string `json:"offerId"`
	BuyQuantity  int    `json:"buyQuantity"`
	Env          int    `json:"env"`
	CurrencyType string `json:"currencyType"`
	ProductID    string `json:"productId"`
	GoodsPrice   int64  `json:"goodsPrice"`
	OutTradeNo   string `json:"outTradeNo"`
	Attach       string `json:"attach"`
}

// TestMarshalNoHTMLEscape 是本包最重要的一条回归测试：
// 它锁死「序列化 → 签名」整条链路，防止 Go 的 HTML 转义把签名搞坏。
func TestMarshalNoHTMLEscape(t *testing.T) {
	const want = `{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY","productId":"a<b>&c","goodsPrice":100,"outTradeNo":"ORDER20260101002","attach":"x&y"}`

	body := prepayBodyForTest{
		OfferID:      "1234567890",
		BuyQuantity:  1,
		Env:          0,
		CurrencyType: "CNY",
		ProductID:    "a<b>&c",
		GoodsPrice:   100,
		OutTradeNo:   "ORDER20260101002",
		Attach:       "x&y",
	}

	// 1) 我们的序列化器必须与原文逐字节一致（保留 < > &，不转义、无尾换行）。
	got, err := marshalNoHTMLEscape(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("marshalNoHTMLEscape() = %s\nwant                  = %s", got, want)
	}
	if strings.HasSuffix(string(got), "\n") {
		t.Fatal("不应有结尾换行（json.Encoder 默认会追加）")
	}

	// 2) 对照：标准库的 json.Marshal 会对同一结构做 HTML 转义。
	//    两者必须不同 —— 这正是本函数存在的意义。
	std, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("标准库输出: %s", std)
	if string(std) == string(got) {
		t.Fatal("标准库默认会转义 < > &，与本包输出意外相同，说明转义防护未生效")
	}

	// 3) 集成断言：用这份序列化结果算出的签名，必须等于 Python 交叉验证的期望值。
	sig := CalcPaySig("test_app_key_1234567890", "requestVirtualPayment", string(got))
	if sig != "540b7ce535b85c51f5419207c1a833f10765d1050254f21a1cfa23a2bcf4e271" {
		t.Fatalf("由序列化结果算出的签名不符: %s", sig)
	}
}
