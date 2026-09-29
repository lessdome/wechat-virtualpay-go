package wechat_virtualpay

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// 账单类的字段表，照 xpay_order_test.go 的 TestOrderFieldsCoverDoc 那一套核：名字、类型、
// **行序**三项都要与官方字段表一致，且不多不少。
//
// 本类最要紧的一行是 DownloadBillRequest / DownloadIOSBillRequest **没有 env** —— 这是
// 全包仅有的两个不出 env 的请求体，正是它们逼出了 xpayNoEnvRequest 那个标记。所以这里的
// 期望里没有 env 那行，是**有意的**，不是抄漏。
func TestBillFieldsCoverDoc(t *testing.T) {
	tStr := reflect.TypeOf("")
	tI64 := reflect.TypeOf(int64(0))
	tInt := reflect.TypeOf(int(0))
	tIOSBillList := reflect.TypeOf([]IOSBill(nil))

	cases := []struct {
		what string
		val  any
		doc  []fieldSpec
	}{
		// 官方字段表里只有 begin_ds / end_ds 两行，没有 env。
		{"DownloadBillRequest", DownloadBillRequest{}, []fieldSpec{
			{"begin_ds", tI64}, {"end_ds", tI64},
		}},
		{"DownloadBillResponse", DownloadBillResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"url", tStr},
		}},
		// 月份是**字符串**（YYYYMM），不是数字——别按 DownloadBill 那两个 int64 去写。
		{"DownloadIOSBillRequest", DownloadIOSBillRequest{}, []fieldSpec{
			{"start_month", tStr}, {"end_month", tStr},
		}},
		{"IOSBill", IOSBill{}, []fieldSpec{
			{"month", tStr}, {"bill_url", tStr},
		}},
		{"DownloadIOSBillResponse", DownloadIOSBillResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"bill_list", tIOSBillList},
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
	}
	// 2 + 3 + 2 + 2 + 3。这个数不能并进别的类的表里——那边各自钉各的。
	if total != 12 {
		t.Fatalf("上面 %d 个结构体的文档字段总数应为 12，实际 %d（期望集合可能抄漏）", len(cases), total)
	}
}

// 本类的两个接口发出去的请求体：**逐字节**对，且里面**没有 env**。
//
// 这条是 xpayNoEnvRequest 那个标记的唯一效果所在，也是它的护栏：兜底补 env 那条路一旦
// 盖到这两个接口上（标记被删、或者有人给结构体加了 Env 字段），下面第一处的字节比对立刻
// 报红。字节里还顺带钉住了键序——不补 env 的那条路不做 map 往返，所以键序就是字段声明
// 顺序，能拿着官方字段表一行行对下来。
//
// pay_sig 用发出去的那份字节独立复算（不复用 CalcPaySig）：签名与请求体是同一份，这条在
// 本类同样成立，不因为少了 env 而变成两件事。
func TestBillRequestsSendNoEnvAndSignTheSentBytes(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		call func(ctx context.Context) error
		want string
		resp string
	}{
		{
			"download_bill",
			"/xpay/download_bill",
			func(ctx context.Context) error {
				_, err := DownloadBill(ctx, "T", "K", DownloadBillRequest{BeginDs: 20230801, EndDs: 20230810})
				return err
			},
			`{"begin_ds":20230801,"end_ds":20230810}`,
			`{"errcode":0,"url":"https://example.com/bill"}`,
		},
		{
			"download_ios_settlement_bill",
			"/xpay/download_ios_settlement_bill",
			func(ctx context.Context) error {
				_, err := DownloadIOSBill(ctx, "T", "K", DownloadIOSBillRequest{StartMonth: "202601", EndMonth: "202603"})
				return err
			},
			`{"start_month":"202601","end_month":"202603"}`,
			`{"errcode":0,"bill_list":[{"month":"202601","bill_url":"https://example.com/ios"}]}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: c.resp}
			swapXpay(t, rt)

			if err := c.call(context.Background()); err != nil {
				t.Fatal(err)
			}
			if rt.calls != 1 {
				t.Fatalf("应当只发一次请求，实际 %d", rt.calls)
			}
			if rt.path != c.uri {
				t.Errorf("路径不对: %s", rt.path)
			}
			if got := string(rt.body); got != c.want {
				t.Fatalf("请求体不对\n实际: %s\n期望: %s", got, c.want)
			}

			// 这一条是本文件存在的理由：请求体里不许有 env。整个 /xpay/* 里只有本类
			// 如此，多出来一个字节都是「本包替微信发明了字段」。
			if _, ok := bodyEnv(t, rt.body)["env"]; ok {
				t.Errorf("本接口的请求体里不该有 env: %s", rt.body)
			}

			if got, want := rt.query.Get("pay_sig"), testPaySig("K", c.uri, string(rt.body)); got != want {
				t.Errorf("pay_sig 与发出去的请求体对不上\n实际: %s\n期望: %s\n请求体: %s", got, want, rt.body)
			}
			if got := rt.query.Get("access_token"); got != "T" {
				t.Errorf("access_token 不对: %s", got)
			}
			if got := rt.query.Get("signature"); got != "" {
				t.Errorf("这一档不该有 signature，实际 %s", got)
			}
		})
	}
}

// 本类的请求结构体上不该有 env 字段——这是**类型层面**的另一半：上面那条测的是发出去的
// 字节，这条测的是结构体本身。两处都要在，因为「结构体有 env 字段但因为别的原因没发出去」
// 与「结构体没有 env 字段」在字节上分不出来，而在调用方那里是两件事（前者说明有人在
// 瞎折腾，后者是文档的实情）。
func TestBillRequestsHaveNoEnvField(t *testing.T) {
	// 反射走的是 fieldSpec 那套取法：本类的请求体没有内嵌字段，直接看自己的字段即可。
	for name, req := range map[string]any{
		"DownloadBillRequest":    DownloadBillRequest{},
		"DownloadIOSBillRequest": DownloadIOSBillRequest{},
	} {
		if _, ok := reflect.TypeOf(req).FieldByName("Env"); ok {
			t.Errorf("%s 不该有 Env 字段：官方字段表里没有 env（见 xpay_bill.go 文件头）", name)
		}
		// 声明了「不带 env」才允许没有 env 字段，标记与字段必须成对——否则
		// requestBody 会替它补一个文档里没有的 env。
		if _, ok := req.(xpayNoEnvRequest); !ok {
			t.Errorf("%s 没有 env 字段却没实现 xpayNoEnvRequest：requestBody 会替它补一个 env", name)
		}
	}
}

// 校验不合法时，一个字节都不该发出去。
func TestBillEndpointsRejectBadInputWithoutRequest(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		want string
		call func() error
	}{
		{"缺 accessToken", "accessToken", func() error {
			_, err := DownloadBill(ctx, "", "K", DownloadBillRequest{BeginDs: 20230801, EndDs: 20230810})
			return err
		}},
		{"缺 appKey", "appKey", func() error {
			_, err := DownloadBill(ctx, "T", "", DownloadBillRequest{BeginDs: 20230801, EndDs: 20230810})
			return err
		}},
		{"BeginDs 不是日期", "BeginDs", func() error {
			_, err := DownloadBill(ctx, "T", "K", DownloadBillRequest{BeginDs: 20261320, EndDs: 20230810})
			return err
		}},
		{"EndDs 位数不足", "EndDs", func() error {
			_, err := DownloadBill(ctx, "T", "K", DownloadBillRequest{BeginDs: 20230801, EndDs: 2023081})
			return err
		}},
		{"EndDs 早于 BeginDs", "早于", func() error {
			_, err := DownloadBill(ctx, "T", "K", DownloadBillRequest{BeginDs: 20230810, EndDs: 20230801})
			return err
		}},
		{"StartMonth 位数不足", "StartMonth", func() error {
			_, err := DownloadIOSBill(ctx, "T", "K", DownloadIOSBillRequest{StartMonth: "20261", EndMonth: "202603"})
			return err
		}},
		{"月份带了分隔符", "StartMonth", func() error {
			_, err := DownloadIOSBill(ctx, "T", "K", DownloadIOSBillRequest{StartMonth: "2026-01", EndMonth: "202603"})
			return err
		}},
		{"月份 13 月", "EndMonth", func() error {
			_, err := DownloadIOSBill(ctx, "T", "K", DownloadIOSBillRequest{StartMonth: "202601", EndMonth: "202613"})
			return err
		}},
		{"EndMonth 早于 StartMonth", "早于", func() error {
			_, err := DownloadIOSBill(ctx, "T", "K", DownloadIOSBillRequest{StartMonth: "202603", EndMonth: "202601"})
			return err
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			err := c.call()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("期望报错含 %q，实际: %v", c.want, err)
			}
			if rt.calls != 0 {
				t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
			}
		})
	}
}

// 合法输入的边界必须放行——否则上面那张表可能只是「什么都拦」。
//
// 这里刻意把**跨度**放开测：订单类的 start_download_order 限 31 天，本类没有那条上限
// （官方两页都没写），所以一个 90 天的区间、一个跨 12 个月的区间都必须过。谁要是「顺手」
// 把 checkDateRange 换成本类的校验（连带把 31 天带进来），这条会红。
func TestBillAcceptsWholeRanges(t *testing.T) {
	cases := []struct {
		name string
		call func(ctx context.Context) error
	}{
		{"同一天的日账单", func(ctx context.Context) error {
			_, err := DownloadBill(ctx, "T", "K", DownloadBillRequest{BeginDs: 20230801, EndDs: 20230801})
			return err
		}},
		{"90 天的账单区间（没有 31 天上限）", func(ctx context.Context) error {
			_, err := DownloadBill(ctx, "T", "K", DownloadBillRequest{BeginDs: 20260101, EndDs: 20260401})
			return err
		}},
		{"单月", func(ctx context.Context) error {
			_, err := DownloadIOSBill(ctx, "T", "K", DownloadIOSBillRequest{StartMonth: "202601", EndMonth: "202601"})
			return err
		}},
		{"跨年 12 个月", func(ctx context.Context) error {
			_, err := DownloadIOSBill(ctx, "T", "K", DownloadIOSBillRequest{StartMonth: "202601", EndMonth: "202612"})
			return err
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			if err := c.call(context.Background()); err != nil {
				t.Fatalf("应当放行，实际: %v", err)
			}
			if rt.calls != 1 {
				t.Fatalf("合法输入应当发出去一次，实际 %d", rt.calls)
			}
		})
	}
}

// 本类没有 env，「该配哪把 key」这句话就无从说起：报错文案里不许出现 Env。否则调用方会去
// 找一个本类结构体上根本不存在的字段。
func TestBillAppKeyErrorDoesNotMentionEnv(t *testing.T) {
	_, err := DownloadBill(context.Background(), "T", "", DownloadBillRequest{BeginDs: 20230801, EndDs: 20230810})
	if err == nil {
		t.Fatal("appKey 为空应当报错")
	}
	if !strings.Contains(err.Error(), "appKey") {
		t.Errorf("文案里应当有 appKey，实际: %v", err)
	}
	if strings.Contains(err.Error(), "Env") {
		t.Errorf("本接口没有 env 字段，文案不该提它（会把人支到一个不存在的字段上）: %v", err)
	}
}

// 响应解析：链接为空不是错误（账单还在生成），是轮询该继续的信号。
func TestDownloadBillEmptyURLIsNotAnError(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok"}`}
	swapXpay(t, rt)

	resp, err := DownloadBill(context.Background(), "T", "K", DownloadBillRequest{BeginDs: 20230801, EndDs: 20230810})
	if err != nil {
		t.Fatalf("链接还没生成不该报错: %v", err)
	}
	if resp.URL != "" {
		t.Fatalf("URL 应当是空串，实际 %q", resp.URL)
	}
	// 别把 resp 本身与 resp.URL 混了：resp 非 nil 说明这一趟走通了。
	if resp == nil {
		t.Fatal("走通了就该有响应")
	}
	if resp.ErrCode != 0 {
		t.Errorf("errcode 应当是 0，实际 %d", resp.ErrCode)
	}
}
