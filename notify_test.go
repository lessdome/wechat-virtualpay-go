package wechat_virtualpay_go

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// sign 独立算一遍明文模式的验签（不调用生产代码，免得自证）。
func sign(token, ts, nonce string) string {
	parts := []string{token, ts, nonce}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(sum[:])
}

// notifyBody 造一条最小可解析的推送
func notifyBody(event string) string {
	return `{"ToUserName":"gh_x","FromUserName":"oUser","CreateTime":1700000000,"MsgType":"event","Event":"` + event + `"}`
}

// 官方《消息推送》页给的两条样例（Token 是 "AAAAA"）——用它当验签的期望值。
func TestParseWithOfficialSampleSignature(t *testing.T) {
	const body = `{"ToUserName":"gh_x","MsgType":"event","Event":"xpay_goods_deliver_notify",` +
		`"WeChatPayInfo":{"MchOrderNo":"ORDER1"},"GoodsInfo":{"ProductId":"p1"}}`

	// 样例一：timestamp=1714036504 nonce=1514711492
	q := url.Values{
		"timestamp": {"1714036504"}, "nonce": {"1514711492"},
		"signature": {"f464b24fc39322e44b38aa78f5edd27bd1441696"},
	}
	notif, err := ParseNotification("AAAAA", httptest.NewRequest("POST", "/n?"+q.Encode(), strings.NewReader(body)))
	if err != nil {
		t.Fatalf("官方样例应当验过: %v", err)
	}
	if notif.Event != EventGoodsDeliver || notif.GoodsDeliver == nil {
		t.Fatalf("事件或载荷不对: %+v", notif)
	}
	if notif.GoodsDeliver.WeChatPayInfo == nil || notif.GoodsDeliver.WeChatPayInfo.MchOrderNo != "ORDER1" {
		t.Fatalf("嵌套字段没填上: %+v", notif.GoodsDeliver)
	}
	if string(notif.Plain) != body {
		t.Fatal("Plain 应当是原始报文")
	}

	// 换个 token 就不该过
	if _, err := ParseNotification("BBBBB", httptest.NewRequest("POST", "/n?"+q.Encode(), strings.NewReader(body))); err != ErrInvalidSignature {
		t.Fatalf("token 不匹配应为 ErrInvalidSignature，实际: %v", err)
	}
}

// 验签不过：返回 ErrInvalidSignature 且 Notification 为 nil（绝不能让调用方拿到半个结果）
func TestParseRejectsBadSignature(t *testing.T) {
	q := url.Values{"timestamp": {"1"}, "nonce": {"n"}, "signature": {"deadbeef"}}
	notif, err := ParseNotification("tok", httptest.NewRequest("POST", "/n?"+q.Encode(), strings.NewReader(notifyBody("xpay_refund_notify"))))
	if err != ErrInvalidSignature {
		t.Fatalf("期望 ErrInvalidSignature，实际: %v", err)
	}
	if notif != nil {
		t.Fatalf("验签失败时 Notification 必须是 nil，实际: %+v", notif)
	}
}

// 6 类事件都能识别，且载荷落到对应字段
func TestParseAllKnownEvents(t *testing.T) {
	const tok, ts, nonce = "tok", "1700000000", "n1"
	q := url.Values{"timestamp": {ts}, "nonce": {nonce}, "signature": {sign(tok, ts, nonce)}}

	cases := []struct {
		event string
		check func(*Notification) bool
	}{
		{"xpay_goods_deliver_notify", func(n *Notification) bool { return n.GoodsDeliver != nil }},
		{"xpay_coin_pay_notify", func(n *Notification) bool { return n.CoinPay != nil }},
		{"xpay_refund_notify", func(n *Notification) bool { return n.Refund != nil }},
		{"xpay_complaint_notify", func(n *Notification) bool { return n.Complaint != nil }},
		{"xpay_wxpay_callback_notify", func(n *Notification) bool { return n.WxpayCallback != nil }},
		{"xpay_subscribe_ios_refund_query_notify", func(n *Notification) bool { return n.IOSRefundQuery != nil }},
	}
	for _, c := range cases {
		t.Run(c.event, func(t *testing.T) {
			notif, err := ParseNotification(tok, httptest.NewRequest("POST", "/n?"+q.Encode(), strings.NewReader(notifyBody(c.event))))
			if err != nil {
				t.Fatal(err)
			}
			if string(notif.Event) != c.event {
				t.Fatalf("事件不对: %s", notif.Event)
			}
			if !c.check(notif) {
				t.Fatalf("载荷没落到对应字段: %+v", notif)
			}
		})
	}
}

// iOS 退款问询的报文**不带 Event**（官方字段表里没有它），靠 channel_bill + bundleid 认出来
func TestParseIOSRefundQueryWithoutEvent(t *testing.T) {
	const tok, ts, nonce = "tok", "1700000000", "n1"
	q := url.Values{"timestamp": {ts}, "nonce": {nonce}, "signature": {sign(tok, ts, nonce)}}
	body := `{"refund_time":"1","channel_bill":"BILL1","bundleid":"com.x.y","provide_status":"1","pay_order_id":"O1"}`

	notif, err := ParseNotification(tok, httptest.NewRequest("POST", "/n?"+q.Encode(), strings.NewReader(body)))
	if err != nil {
		t.Fatalf("应当识别为 iOS 退款问询: %v", err)
	}
	if notif.Event != EventIOSRefundQuery || notif.IOSRefundQuery == nil {
		t.Fatalf("事件不对: %+v", notif)
	}
	if notif.IOSRefundQuery.ChannelBill != "BILL1" || notif.IOSRefundQuery.ProvideStatus != IOSProvideDone {
		t.Fatalf("字段不对: %+v", notif.IOSRefundQuery)
	}
}

// 既没有 Event、也不是 iOS 退款问询 —— 必须硬失败。
// 放行的话调用方会当成收到了真推送、照常回成功应答，这条推送就永久丢了。
func TestParseRejectsPayloadWithoutEvent(t *testing.T) {
	const tok, ts, nonce = "tok", "1700000000", "n1"
	q := url.Values{"timestamp": {ts}, "nonce": {nonce}, "signature": {sign(tok, ts, nonce)}}

	_, err := ParseNotification(tok, httptest.NewRequest("POST", "/n?"+q.Encode(), strings.NewReader(`{"foo":"bar"}`)))
	if err == nil || !strings.Contains(err.Error(), "Event") {
		t.Fatalf("没有 Event 应当报错，实际: %v", err)
	}
}

// 未知事件放行：Event 有值、载荷全 nil（微信将来可能新增事件）
func TestParseUnknownEventPassesThrough(t *testing.T) {
	const tok, ts, nonce = "tok", "1700000000", "n1"
	q := url.Values{"timestamp": {ts}, "nonce": {nonce}, "signature": {sign(tok, ts, nonce)}}

	notif, err := ParseNotification(tok, httptest.NewRequest("POST", "/n?"+q.Encode(), strings.NewReader(notifyBody("xpay_future_notify"))))
	if err != nil {
		t.Fatalf("未知事件不该报错: %v", err)
	}
	if string(notif.Event) != "xpay_future_notify" || notif.GoodsDeliver != nil {
		t.Fatalf("结果不对: %+v", notif)
	}
}

// 安全模式（encrypt_type=aes）本包不支持，要给明确报错而不是含糊的解析失败
func TestParseRejectsEncryptedMode(t *testing.T) {
	const tok, ts, nonce = "tok", "1700000000", "n1"
	q := url.Values{"timestamp": {ts}, "nonce": {nonce}, "encrypt_type": {"aes"}, "msg_signature": {"x"}}

	_, err := ParseNotification(tok, httptest.NewRequest("POST", "/n?"+q.Encode(), strings.NewReader(`{"Encrypt":"abc"}`)))
	if err == nil || !strings.Contains(err.Error(), "明文模式") {
		t.Fatalf("安全模式应当被明确拒绝，实际: %v", err)
	}
}

// 入参与报文的基本校验
func TestParseInputValidation(t *testing.T) {
	t.Run("token 为空", func(t *testing.T) {
		if _, err := ParseNotification("", httptest.NewRequest("POST", "/n", strings.NewReader("{}"))); err == nil || !strings.Contains(err.Error(), "token") {
			t.Fatalf("期望 token 报错，实际: %v", err)
		}
	})
	t.Run("body 为空", func(t *testing.T) {
		_, err := ParseNotification("tok", httptest.NewRequest("POST", "/n", strings.NewReader("   ")))
		if err == nil || !strings.Contains(err.Error(), "请求体为空") {
			t.Fatalf("期望请求体为空的报错，实际: %v", err)
		}
	})
	t.Run("缺 timestamp/nonce", func(t *testing.T) {
		_, err := ParseNotification("tok", httptest.NewRequest("POST", "/n", strings.NewReader(notifyBody("xpay_refund_notify"))))
		if err == nil || !strings.Contains(err.Error(), "timestamp") {
			t.Fatalf("期望缺 timestamp/nonce 的报错，实际: %v", err)
		}
	})
}

// 机械比对：文档字段表里的字段名，必须都能在对应结构体的 json tag 里找到。
// 期望集合是从官方 2.4 的 6 张字段表里抽出来的（共 100 个字段名）。
func TestNotifyFieldsCoverDoc(t *testing.T) {
	doc := map[string][]string{
		"xpay_goods_deliver_notify":  {"ToUserName", "FromUserName", "CreateTime", "MsgType", "Event", "OpenId", "OutTradeNo", "Env", "WeChatPayInfo", "GoodsInfo", "MchOrderNo", "TransactionId", "PaidTime", "ProductId", "Quantity", "OrigPrice", "ActualPrice", "Attach", "ActivityId", "TeamId", "TeamType", "TeamAction"},
		"xpay_coin_pay_notify":       {"ToUserName", "FromUserName", "CreateTime", "MsgType", "Event", "OpenId", "OutTradeNo", "Env", "WeChatPayInfo", "CoinInfo", "MchOrderNo", "TransactionId", "PaidTime", "Quantity", "OrigPrice", "ActualPrice", "Attach"},
		"xpay_refund_notify":         {"ToUserName", "FromUserName", "CreateTime", "MsgType", "Event", "OpenId", "WxRefundId", "MchRefundId", "WxOrderId", "MchOrderId", "RefundFee", "RetCode", "RetMsg", "RefundStartTimestamp", "RefundSuccTimestamp", "WxpayRefundTransactionId", "RetryTimes", "Attach", "WxTransactionId", "ActivityId", "TeamId", "TeamType", "TeamAction"},
		"xpay_complaint_notify":      {"ToUserName", "FromUserName", "CreateTime", "MsgType", "Event", "OpenId", "WxOrderId", "MchOrderId", "TransactionId", "ComplaintId", "ComplaintDetail", "ComplaintTime", "RetryTimes", "RequestId"},
		"xpay_wxpay_callback_notify": {"ToUserName", "FromUserName", "CreateTime", "MsgType", "Event", "AppId", "NickName", "MerchantCode", "MerchantCompanyName", "BusinessTime", "BusinessCode", "BusinessState", "Remark", "EventType", "RetryTimes"},
		// iOS 退款问询的字段表是 snake_case，且**没有 Event**
		"xpay_subscribe_ios_refund_query_notify": {"refund_time", "order_time", "channel_bill", "bundleid", "product_id", "p_count", "refund_request_reason", "provide_status", "pay_order_id"},
	}
	structs := map[string]any{
		"xpay_goods_deliver_notify":              GoodsDeliverNotify{},
		"xpay_coin_pay_notify":                   CoinPayNotify{},
		"xpay_refund_notify":                     RefundNotify{},
		"xpay_complaint_notify":                  ComplaintNotify{},
		"xpay_wxpay_callback_notify":             WxpayCallbackNotify{},
		"xpay_subscribe_ios_refund_query_notify": IOSRefundQueryNotify{},
	}

	total := 0
	for event, want := range doc {
		got := map[string]bool{}
		collectJSONTags(reflect.TypeOf(structs[event]), got)
		for _, f := range want {
			total++
			if !got[f] {
				t.Errorf("事件 %s 缺文档里的字段 %q", event, f)
			}
		}
		t.Logf("%s：文档 %d 个字段全部命中（结构体共 %d 个 json tag）", event, len(want), len(got))
	}
	if total != 100 {
		t.Fatalf("文档字段总数应为 100，实际 %d（期望集合可能抄漏）", total)
	}
}

// collectJSONTags 递归收集结构体（含嵌入与嵌套）里的 json tag 名。
func collectJSONTags(t reflect.Type, out map[string]bool) {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if tag, ok := f.Tag.Lookup("json"); ok {
			name := strings.Split(tag, ",")[0]
			if name != "" && name != "-" {
				out[name] = true
			}
		}
		collectJSONTags(f.Type, out)
	}
}
