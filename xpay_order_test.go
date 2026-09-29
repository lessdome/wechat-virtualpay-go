package wechat_virtualpay

import (
	"reflect"
	"strings"
	"testing"
)

// 文档字段表的机械比对：字段名与**类型**都要与结构体一致，且不多不少。
//
// 只比字段名不够——把 int64 抄成 int、把具名枚举抄成裸 int 一样能全绿，直到真机
// 反序列化才发现。反过来，结构体里多出一个文档没有的字段也要报：那说明抄字段表时
// 混进了想象出来的字段。
// orderDocFields 按**声明顺序**取出「结构体直接可见」的 json 字段与类型。
//
// 「直接可见」= 自己的字段 + 匿名字段**提升**上来的字段（内嵌的 ResponseHeader 就是后
// 者：调用方写 resp.ErrCode 就读得到，所以 errcode/errmsg 是这张文档表里的两行，不是
// 外来的东西）。
//
// 但**不**递归进具名嵌套的结构体：order 那样的子对象有自己的文档字段表，各钉各的，
// 混在一张表里核就对不上了。
//
// 不能用 notify_test.go 的 collectJSONFieldTypes：那个是**递归**的（notify 那边正靠它
// 穿透内嵌的 CommonNotifyFields 与嵌套的容器），而且走 map、顺序丢了。这里的语义是
// 「结构体字段表 = 文档表的一行行」，顺序本身就是要核的东西。
func orderDocFields(t reflect.Type) []fieldSpec {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []fieldSpec
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if tag, ok := f.Tag.Lookup("json"); ok {
			if name := strings.Split(tag, ",")[0]; name != "" && name != "-" {
				out = append(out, fieldSpec{name, f.Type})
			}
			continue
		}
		// 没有 json 标签的匿名字段：嵌入。提升上来的字段也算直接可见。
		if f.Anonymous {
			out = append(out, orderDocFields(f.Type)...)
		}
	}
	return out
}

func TestOrderFieldsCoverDoc(t *testing.T) {
	tStr := reflect.TypeOf("")
	tI64 := reflect.TypeOf(int64(0))
	tInt := reflect.TypeOf(int(0))
	tOrderStatus := reflect.TypeOf(OrderStatus(0))
	tOrderType := reflect.TypeOf(OrderType(0))
	tSettleState := reflect.TypeOf(SettleState(0))
	tEnvType := reflect.TypeOf(OrderEnvType(0))
	tRefundReason := reflect.TypeOf(RefundReason(""))
	tRefundFrom := reflect.TypeOf(RefundFrom(""))
	tDownloadOrderType := reflect.TypeOf(DownloadOrderType(0))
	tPayChannel := reflect.TypeOf(PayChannel(0))
	tRefundStatusFilter := reflect.TypeOf(RefundStatusFilter(0))
	tDownloadTaskStatus := reflect.TypeOf(DownloadTaskStatus(0))
	tBoolPtr := reflect.TypeOf((*bool)(nil))
	tOrderPtr := reflect.TypeOf((*Order)(nil))

	cases := []struct {
		what string
		val  any
		doc  []fieldSpec
	}{
		// query_order 响应的 order 对象：官方字段表 23 项，行序照抄（order_id 就是
		// 表里的第一项）。
		{"Order", Order{}, []fieldSpec{
			{"order_id", tStr}, {"create_time", tI64}, {"update_time", tI64},
			{"status", tOrderStatus}, {"biz_type", tInt},
			{"order_fee", tI64}, {"coupon_fee", tI64}, {"paid_fee", tI64},
			{"order_type", tOrderType}, {"refund_fee", tI64},
			{"paid_time", tI64}, {"provide_time", tI64},
			{"biz_meta", tStr}, {"env_type", tEnvType}, {"token", tStr},
			{"left_fee", tI64}, {"wx_order_id", tStr},
			{"channel_order_id", tStr}, {"wxpay_order_id", tStr},
			{"sett_time", tI64}, {"sett_state", tSettleState},
			{"platform_fee_fen", tI64}, {"cps_fee_fen", tI64},
		}},
		{"QueryOrderRequest", QueryOrderRequest{}, []fieldSpec{
			{"openid", tStr}, {"env", tInt}, {"order_id", tStr}, {"wx_order_id", tStr},
		}},
		// 哪怕响应体只有「公共头 + order」两层，也要有这一层结构体——errmsg/errcode 就在
		// 这里，没有它调用方看不到失败原因。
		{"QueryOrderResponse", QueryOrderResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"order", tOrderPtr},
		}},
		{"RefundOrderRequest", RefundOrderRequest{}, []fieldSpec{
			{"openid", tStr}, {"order_id", tStr}, {"wx_order_id", tStr},
			{"refund_order_id", tStr}, {"left_fee", tI64}, {"refund_fee", tI64},
			{"biz_meta", tStr}, {"refund_reason", tRefundReason}, {"req_from", tRefundFrom},
			{"env", tInt},
		}},
		{"RefundOrderResponse", RefundOrderResponse{}, []fieldSpec{
			// 公共头。它在结构体里是**内嵌**的匿名字段，提升后就是直接可见的字段，所以
			// 也进这张表（每个响应表都有这两行，不是只有某一个有）。
			//
			// ⚠️ 放在**最前**是照官方响应体样例（errcode/errmsg 打头）取的，没有逐页核对
			// 过字段表的行序。若某页的响应表把这两行放在表尾，就把对应结构体里的
			// ResponseHeader 也挪到最后——顺序是照文档抄的，这条不算例外。
			{"errcode", tInt}, {"errmsg", tStr},
			{"refund_order_id", tStr}, {"refund_wx_order_id", tStr},
			{"pay_order_id", tStr}, {"pay_wx_order_id", tStr},
		}},
		{"NotifyProvideGoodsRequest", NotifyProvideGoodsRequest{}, []fieldSpec{
			{"order_id", tStr}, {"wx_order_id", tStr}, {"env", tInt},
		}},
		// 只回公共头的响应也要有个结构体：没有自己的字段 ≠ 不会失败。
		{"NotifyProvideGoodsResponse", NotifyProvideGoodsResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
		}},
		{"StartDownloadOrderRequest", StartDownloadOrderRequest{}, []fieldSpec{
			{"begin_ds", tI64}, {"end_ds", tI64}, {"order_type", tDownloadOrderType},
			{"order_info", tStr}, {"is_provided", tBoolPtr},
			{"refund_status", tRefundStatusFilter}, {"env", tInt}, {"pay_channel", tPayChannel},
		}},
		{"StartDownloadOrderResponse", StartDownloadOrderResponse{}, []fieldSpec{
			// 公共头。它在结构体里是**内嵌**的匿名字段，提升后就是直接可见的字段，所以
			// 也进这张表（每个响应表都有这两行，不是只有某一个有）。
			//
			// ⚠️ 放在**最前**是照官方响应体样例（errcode/errmsg 打头）取的，没有逐页核对
			// 过字段表的行序。若某页的响应表把这两行放在表尾，就把对应结构体里的
			// ResponseHeader 也挪到最后——顺序是照文档抄的，这条不算例外。
			{"errcode", tInt}, {"errmsg", tStr},
			{"task_id", tStr},
		}},
		{"QueryDownloadOrderRequest", QueryDownloadOrderRequest{}, []fieldSpec{
			{"task_id", tStr}, {"env", tInt},
		}},
		{"QueryDownloadOrderResponse", QueryDownloadOrderResponse{}, []fieldSpec{
			// 公共头。它在结构体里是**内嵌**的匿名字段，提升后就是直接可见的字段，所以
			// 也进这张表（每个响应表都有这两行，不是只有某一个有）。
			//
			// ⚠️ 放在**最前**是照官方响应体样例（errcode/errmsg 打头）取的，没有逐页核对
			// 过字段表的行序。若某页的响应表把这两行放在表尾，就把对应结构体里的
			// ResponseHeader 也挪到最后——顺序是照文档抄的，这条不算例外。
			{"errcode", tInt}, {"errmsg", tStr},
			{"task_id", tStr}, {"status", tDownloadTaskStatus},
			{"download_url", tStr}, {"expire_at", tI64},
		}},
	}

	total := 0
	for _, c := range cases {
		got := orderDocFields(reflect.TypeOf(c.val))
		total += len(c.doc)

		byName := make(map[string]reflect.Type, len(got))
		for _, f := range got {
			byName[f.name] = f.typ
		}
		for _, want := range c.doc {
			typ, ok := byName[want.name]
			if !ok {
				t.Errorf("%s 缺文档里的字段 %q", c.what, want.name)
				continue
			}
			if typ != want.typ {
				t.Errorf("%s 的字段 %q 类型不对：结构体是 %s，文档是 %s", c.what, want.name, typ, want.typ)
			}
		}
		inDoc := make(map[string]bool, len(c.doc))
		for _, f := range c.doc {
			inDoc[f.name] = true
		}
		for _, f := range got {
			if !inDoc[f.name] {
				t.Errorf("%s 多出文档里没有的字段 %q（%s）", c.what, f.name, f.typ)
			}
		}

		// **行序**也要对：官方字段表是有序的，结构体照表逐行抄下来，读代码时才能一眼
		// 对上文档。spec 本身就是按文档行序写的，直接拿它当期望。
		wantOrder := make([]string, 0, len(c.doc))
		for _, f := range c.doc {
			wantOrder = append(wantOrder, f.name)
		}
		gotOrder := make([]string, 0, len(got))
		for _, f := range got {
			gotOrder = append(gotOrder, f.name)
		}
		if !reflect.DeepEqual(gotOrder, wantOrder) {
			t.Errorf("%s 的字段顺序与文档字段表不一致\n文档: %v\n结构体: %v", c.what, wantOrder, gotOrder)
		}
		t.Logf("%s：文档 %d 个字段的名字与类型全部命中", c.what, len(c.doc))
	}
	if total != 70 {
		t.Fatalf("上面 %d 个结构体的文档字段总数应为 70，实际 %d（期望集合可能抄漏）", len(cases), total)
	}
}

// env 是请求体里官方标为**必填**的字段，5 个请求结构体上都必须有，而且是**裸 int**
// ——调用方直接写 0/1，含义写在字段注释里。
//
// 这里原先是钉「必须是具名的 Env 类型」的，后来把这个类型整个删了：值域只有两个数，
// 包一层类型只是让调用方多认一个名字，挡不住任何东西（写 Env: 2 照样编得过）。
//
// 那「两套环境编码不能互赋值」的保险呢？它**不在这个字段上**，在响应侧：env_type 是
// 1=现网 / 2=沙箱 的另一套码，仍然是具名的 OrderEnvType。两边的类型不同名，所以
// req.Env 与 resp.Order.EnvType 互相赋值**两个方向都编不过**——裸 int 也是具名类型
// （预声明类型），一样挡得住。删掉的是名字，不是这道保险。
func TestOrderRequestsExposeEnv(t *testing.T) {
	want := reflect.TypeOf(int(0))
	requests := map[string]any{
		"QueryOrderRequest":         QueryOrderRequest{},
		"RefundOrderRequest":        RefundOrderRequest{},
		"NotifyProvideGoodsRequest": NotifyProvideGoodsRequest{},
		"StartDownloadOrderRequest": StartDownloadOrderRequest{},
		"QueryDownloadOrderRequest": QueryDownloadOrderRequest{},
	}
	for name, req := range requests {
		got := map[string]reflect.Type{}
		collectJSONFieldTypes(reflect.TypeOf(req), got)
		typ, ok := got["env"]
		if !ok {
			t.Errorf("%s 缺 env 字段（官方标必填，零值即现网，必须显式带上）", name)
			continue
		}
		if typ != want {
			t.Errorf("%s 的 env 应当是 %s，实际 %s", name, want, typ)
		}
	}
}

// 枚举取值本身也要钉：抄错一个数字不会报错，只会把状态映射到另一个含义上，比抄错
// 字段名更隐蔽。
func TestOrderEnumValues(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"OrderStatusInit", int(OrderStatusInit), 0},
		{"OrderStatusCreated", int(OrderStatusCreated), 1},
		{"OrderStatusPaid", int(OrderStatusPaid), 2},
		{"OrderStatusProviding", int(OrderStatusProviding), 3},
		{"OrderStatusProvided", int(OrderStatusProvided), 4},
		{"OrderStatusRefunded", int(OrderStatusRefunded), 5},
		{"OrderStatusClosed", int(OrderStatusClosed), 6},
		{"OrderStatusRefundFailed", int(OrderStatusRefundFailed), 7},
		{"OrderStatusRefundCompleted", int(OrderStatusRefundCompleted), 8},
		{"OrderStatusAdFundsRecovered", int(OrderStatusAdFundsRecovered), 9},
		{"OrderStatusSettleRollback", int(OrderStatusSettleRollback), 10},

		{"OrderTypeNormal", int(OrderTypeNormal), 0},
		{"OrderTypeRefund", int(OrderTypeRefund), 1},
		{"OrderTypeIOS", int(OrderTypeIOS), 7},
		{"OrderTypeIOSRefund", int(OrderTypeIOSRefund), 8},

		{"SettleStatePending", int(SettleStatePending), 0},
		{"SettleStateRunning", int(SettleStateRunning), 1},
		{"SettleStateSuccess", int(SettleStateSuccess), 2},
		{"SettleStateWaiting", int(SettleStateWaiting), 3},

		// 响应里的 env_type 是 1/2，与请求体的 env(0/1) 不是一套编码，所以单独钉。
		{"OrderEnvTypeProduction", int(OrderEnvTypeProduction), 1},
		{"OrderEnvTypeSandbox", int(OrderEnvTypeSandbox), 2},

		{"DownloadOrderCoin", int(DownloadOrderCoin), 1},
		{"DownloadOrderGoods", int(DownloadOrderGoods), 2},
		{"DownloadOrderSubscription", int(DownloadOrderSubscription), 3},
		{"DownloadOrderRefund", int(DownloadOrderRefund), 4},

		{"PayChannelNormal", int(PayChannelNormal), 1},
		{"PayChannelIAP", int(PayChannelIAP), 2},

		{"RefundStatusFilterAll", int(RefundStatusFilterAll), 0},
		{"RefundStatusFilterRefunded", int(RefundStatusFilterRefunded), 2},
		{"RefundStatusFilterOngoing", int(RefundStatusFilterOngoing), 4},
		{"RefundStatusFilterFailed", int(RefundStatusFilterFailed), 5},

		{"DownloadTaskInit", int(DownloadTaskInit), 0},
		{"DownloadTaskRunning", int(DownloadTaskRunning), 1},
		{"DownloadTaskSuccess", int(DownloadTaskSuccess), 2},
		{"DownloadTaskFailed", int(DownloadTaskFailed), 3},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d，文档是 %d", c.name, c.got, c.want)
		}
	}
}

// 字符串枚举同理：refund_reason / req_from 是官方限定的字面量，抄错了微信直接拒。
func TestOrderStringEnumValues(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"RefundReasonNone", string(RefundReasonNone), "0"},
		{"RefundReasonProduct", string(RefundReasonProduct), "1"},
		{"RefundReasonAfterSale", string(RefundReasonAfterSale), "2"},
		{"RefundReasonUserWill", string(RefundReasonUserWill), "3"},
		{"RefundReasonPrice", string(RefundReasonPrice), "4"},
		{"RefundReasonOther", string(RefundReasonOther), "5"},

		{"RefundFromCustomerService", string(RefundFromCustomerService), "1"},
		{"RefundFromUser", string(RefundFromUser), "2"},
		{"RefundFromOther", string(RefundFromOther), "3"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q，文档是 %q", c.name, c.got, c.want)
		}
	}
}
