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

// fieldSpec 是文档字段表里的一项：字段名 + 它在结构体里应有的类型。
type fieldSpec struct {
	name string
	typ  reflect.Type
}

// 机械比对：文档字段表里的字段名与**类型**，都必须与对应结构体一致。
//
// 期望集合是从官方 2.4 的 6 张字段表里抽出来的（共 100 个字段）。只比字段名是不够的
// ——把 int64 抄成 string 同样能全绿，直到真机推送反序列化失败才暴露（错误会被包成
// 「解析 xxx 事件失败」，微信重试 15 次全部落空）。所以这里连类型一起钉住。
func TestNotifyFieldsCoverDoc(t *testing.T) {
	tStr := reflect.TypeOf("")
	tI64 := reflect.TypeOf(int64(0))
	tInt := reflect.TypeOf(int(0))
	tEvent := reflect.TypeOf(NotifyEvent(""))
	tRiskType := reflect.TypeOf(WxpayCallbackEventType(""))
	tIOSStatus := reflect.TypeOf(IOSProvideStatus(""))
	// 容器字段按指针类型钉：它们必须可为 nil，否则官方那句「非微信支付渠道可能没有」
	// 就落不了地。（TeamInfo 的容器名不在文档字段表里，只有它内层的四个字段在，
	// 所以这里没有对应的类型项。）
	tPay := reflect.TypeOf((*WeChatPayInfo)(nil))
	tGoods := reflect.TypeOf((*GoodsInfo)(nil))
	tCoin := reflect.TypeOf((*CoinInfo)(nil))

	doc := map[string][]fieldSpec{
		"xpay_goods_deliver_notify": {
			{"ToUserName", tStr}, {"FromUserName", tStr}, {"CreateTime", tI64}, {"MsgType", tStr}, {"Event", tEvent},
			{"OpenId", tStr}, {"OutTradeNo", tStr}, {"Env", tInt},
			{"WeChatPayInfo", tPay}, {"GoodsInfo", tGoods},
			{"MchOrderNo", tStr}, {"TransactionId", tStr}, {"PaidTime", tI64},
			{"ProductId", tStr}, {"Quantity", tI64}, {"OrigPrice", tI64}, {"ActualPrice", tI64}, {"Attach", tStr},
			{"ActivityId", tStr}, {"TeamId", tStr}, {"TeamType", tInt}, {"TeamAction", tInt},
		},
		"xpay_coin_pay_notify": {
			{"ToUserName", tStr}, {"FromUserName", tStr}, {"CreateTime", tI64}, {"MsgType", tStr}, {"Event", tEvent},
			{"OpenId", tStr}, {"OutTradeNo", tStr}, {"Env", tInt},
			{"WeChatPayInfo", tPay}, {"CoinInfo", tCoin},
			{"MchOrderNo", tStr}, {"TransactionId", tStr}, {"PaidTime", tI64},
			{"Quantity", tI64}, {"OrigPrice", tI64}, {"ActualPrice", tI64}, {"Attach", tStr},
		},
		"xpay_refund_notify": {
			{"ToUserName", tStr}, {"FromUserName", tStr}, {"CreateTime", tI64}, {"MsgType", tStr}, {"Event", tEvent},
			{"OpenId", tStr}, {"WxRefundId", tStr}, {"MchRefundId", tStr}, {"WxOrderId", tStr}, {"MchOrderId", tStr},
			{"RefundFee", tI64}, {"RetCode", tInt}, {"RetMsg", tStr},
			{"RefundStartTimestamp", tI64}, {"RefundSuccTimestamp", tI64}, {"WxpayRefundTransactionId", tStr},
			{"RetryTimes", tInt}, {"Attach", tStr}, {"WxTransactionId", tStr},
			{"ActivityId", tStr}, {"TeamId", tStr}, {"TeamType", tInt}, {"TeamAction", tInt},
		},
		"xpay_complaint_notify": {
			{"ToUserName", tStr}, {"FromUserName", tStr}, {"CreateTime", tI64}, {"MsgType", tStr}, {"Event", tEvent},
			{"OpenId", tStr}, {"WxOrderId", tStr}, {"MchOrderId", tStr}, {"TransactionId", tStr},
			{"ComplaintId", tStr}, {"ComplaintDetail", tStr}, {"ComplaintTime", tI64},
			{"RetryTimes", tInt}, {"RequestId", tStr},
		},
		"xpay_wxpay_callback_notify": {
			{"ToUserName", tStr}, {"FromUserName", tStr}, {"CreateTime", tI64}, {"MsgType", tStr}, {"Event", tEvent},
			{"AppId", tStr}, {"NickName", tStr}, {"MerchantCode", tStr}, {"MerchantCompanyName", tStr},
			{"BusinessTime", tStr}, {"BusinessCode", tStr}, {"BusinessState", tStr}, {"Remark", tStr},
			{"EventType", tRiskType}, {"RetryTimes", tInt},
		},
		// iOS 退款问询的字段表是 snake_case，且**没有 Event**
		"xpay_subscribe_ios_refund_query_notify": {
			{"refund_time", tStr}, {"order_time", tStr}, {"channel_bill", tStr}, {"bundleid", tStr},
			{"product_id", tStr}, {"p_count", tStr}, {"refund_request_reason", tStr},
			{"provide_status", tIOSStatus}, {"pay_order_id", tStr},
		},
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
		got := map[string]reflect.Type{}
		collectJSONFieldTypes(reflect.TypeOf(structs[event]), got)
		for _, f := range want {
			total++
			typ, ok := got[f.name]
			if !ok {
				t.Errorf("事件 %s 缺文档里的字段 %q", event, f.name)
				continue
			}
			if typ != f.typ {
				t.Errorf("事件 %s 的字段 %q 类型不对：结构体是 %s，文档是 %s", event, f.name, typ, f.typ)
			}
		}
		t.Logf("%s：文档 %d 个字段的名字与类型全部命中（结构体共 %d 个 json tag）", event, len(want), len(got))
	}
	if total != 100 {
		t.Fatalf("文档字段总数应为 100，实际 %d（期望集合可能抄漏）", total)
	}
}

// collectJSONFieldTypes 递归收集结构体（含嵌入与嵌套）里的 json tag 名 → 字段类型。
//
// 同名 tag 只记第一次出现的那个：同一事件里不应该有重名不同型的字段，真出现了
// 说明结构体本身有问题，由对照表那侧报出来。
func collectJSONFieldTypes(t reflect.Type, out map[string]reflect.Type) {
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
				if _, seen := out[name]; !seen {
					out[name] = f.Type
				}
			}
		}
		collectJSONFieldTypes(f.Type, out)
	}
}
