package wechat_virtualpay

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// 资金类的字段表，照 TestOrderFieldsCoverDoc 那一套核：名字、类型、**行序**三项都要与
// 官方字段表一致，且不多不少。
//
// 这里最要紧的是**类型**那一列：提现金额与余额都是 string（元），不是 int64（分）——
// 抄成 int64 一样能让别的测试全绿，直到真机反序列化才发现收不到值。
func TestFundsFieldsCoverDoc(t *testing.T) {
	tStr := reflect.TypeOf("")
	tInt := reflect.TypeOf(int(0))
	tWithdrawStatus := reflect.TypeOf(WithdrawStatus(0))
	tBizBalance := reflect.TypeOf(BizBalance{})

	cases := []struct {
		what string
		val  any
		doc  []fieldSpec
	}{
		{"CreateWithdrawOrderRequest", CreateWithdrawOrderRequest{}, []fieldSpec{
			{"withdraw_no", tStr}, {"withdraw_amount", tStr}, {"env", tInt},
		}},
		{"CreateWithdrawOrderResponse", CreateWithdrawOrderResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
			{"withdraw_no", tStr}, {"wx_withdraw_no", tStr},
		}},
		{"QueryWithdrawOrderRequest", QueryWithdrawOrderRequest{}, []fieldSpec{
			{"withdraw_no", tStr}, {"env", tInt},
		}},
		// 金额与时间都是 string —— 本接口是「微信把数字当字符串返回」的那一类。
		{"QueryWithdrawOrderResponse", QueryWithdrawOrderResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
			{"withdraw_no", tStr}, {"status", tWithdrawStatus},
			{"withdraw_amount", tStr}, {"wx_withdraw_no", tStr},
			{"withdraw_success_timestamp", tStr}, {"create_time", tStr},
			{"fail_reason", tStr},
		}},
		{"BizBalance", BizBalance{}, []fieldSpec{
			{"amount", tStr}, {"currency_code", tStr},
		}},
		{"QueryBizBalanceRequest", QueryBizBalanceRequest{}, []fieldSpec{
			{"env", tInt},
		}},
		{"QueryBizBalanceResponse", QueryBizBalanceResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"balance_available", tBizBalance},
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
	// 3 + 4 + 2 + 9 + 2 + 1 + 3。
	if total != 24 {
		t.Fatalf("上面 %d 个结构体的文档字段总数应为 24，实际 %d（期望集合可能抄漏）", len(cases), total)
	}
}

// env 在这三个请求结构体上都是裸 int（与订单类同一条规矩，理由见 checkEnv）。
func TestFundsRequestsExposeEnv(t *testing.T) {
	want := reflect.TypeOf(int(0))
	requests := map[string]any{
		"CreateWithdrawOrderRequest": CreateWithdrawOrderRequest{},
		"QueryWithdrawOrderRequest":  QueryWithdrawOrderRequest{},
		"QueryBizBalanceRequest":     QueryBizBalanceRequest{},
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

// 提现单状态是枚举，抄错数字不会报错，只会把状态映射到另一个含义上。
func TestWithdrawStatusValues(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"WithdrawStatusCreated", int(WithdrawStatusCreated), 1},
		{"WithdrawStatusSuccess", int(WithdrawStatusSuccess), 2},
		{"WithdrawStatusFailed", int(WithdrawStatusFailed), 3},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d，文档是 %d", c.name, c.got, c.want)
		}
	}
}

// 三个接口的请求体**逐字节**对：键序即字段声明顺序（`withdraw_amount` 的 omitempty 也在
// 这里钉住——空串是「全额提现」，不是「金额为空」）。
func TestFundsRequestBodies(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		call func(ctx context.Context) error
		want string
	}{
		{
			"create_withdraw_order",
			"/xpay/create_withdraw_order",
			func(ctx context.Context) error {
				_, err := CreateWithdrawOrder(ctx, "T", "K", CreateWithdrawOrderRequest{
					WithdrawNo: "W1234567", WithdrawAmount: "0.01"})
				return err
			},
			`{"withdraw_no":"W1234567","withdraw_amount":"0.01","env":0}`,
		},
		{
			"create_withdraw_order 不填金额=全额提现",
			"/xpay/create_withdraw_order",
			func(ctx context.Context) error {
				_, err := CreateWithdrawOrder(ctx, "T", "K", CreateWithdrawOrderRequest{
					WithdrawNo: "W1234567"})
				return err
			},
			`{"withdraw_no":"W1234567","env":0}`,
		},
		{
			"query_withdraw_order",
			"/xpay/query_withdraw_order",
			func(ctx context.Context) error {
				_, err := QueryWithdrawOrder(ctx, "T", "K", QueryWithdrawOrderRequest{WithdrawNo: "W1234567"})
				return err
			},
			`{"withdraw_no":"W1234567","env":0}`,
		},
		{
			// 本接口的请求体里只有 env，别的什么都没有。
			"query_biz_balance",
			"/xpay/query_biz_balance",
			func(ctx context.Context) error {
				_, err := QueryBizBalance(ctx, "T", "K", QueryBizBalanceRequest{})
				return err
			},
			`{"env":0}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			if err := c.call(context.Background()); err != nil {
				t.Fatal(err)
			}
			if rt.path != c.uri {
				t.Errorf("路径不对: %s", rt.path)
			}
			if got := string(rt.body); got != c.want {
				t.Fatalf("请求体不对\n实际: %s\n期望: %s", got, c.want)
			}
			// 签名与请求体是同一份字节。
			if got, want := rt.query.Get("pay_sig"), testPaySig("K", c.uri, string(rt.body)); got != want {
				t.Errorf("pay_sig 与发出去的请求体对不上\n实际: %s\n期望: %s", got, want)
			}
		})
	}
}

// 三个接口都发 env，且填了沙箱就是 1（这一档官方标必填，与订单类同）。
func TestAllFundsEndpointsSendEnv(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0}`}
	swapXpay(t, rt)

	calls := map[string]struct {
		uri  string
		call func(ctx context.Context, env int) error
	}{
		"create_withdraw_order": {"/xpay/create_withdraw_order", func(ctx context.Context, env int) error {
			_, err := CreateWithdrawOrder(ctx, "T", "K", CreateWithdrawOrderRequest{
				WithdrawNo: "W1234567", WithdrawAmount: "0.01", Env: env})
			return err
		}},
		"query_withdraw_order": {"/xpay/query_withdraw_order", func(ctx context.Context, env int) error {
			_, err := QueryWithdrawOrder(ctx, "T", "K", QueryWithdrawOrderRequest{WithdrawNo: "W1234567", Env: env})
			return err
		}},
		"query_biz_balance": {"/xpay/query_biz_balance", func(ctx context.Context, env int) error {
			_, err := QueryBizBalance(ctx, "T", "K", QueryBizBalanceRequest{Env: env})
			return err
		}},
	}

	for name, c := range calls {
		for _, env := range []int{0, 1} {
			t.Run(name+"/env="+strconv.Itoa(env), func(t *testing.T) {
				rt.calls = 0
				if err := c.call(context.Background(), env); err != nil {
					t.Fatal(err)
				}
				if rt.calls != 1 {
					t.Fatalf("应当只发一次请求，实际 %d", rt.calls)
				}
				got, ok := bodyEnv(t, rt.body)["env"]
				if !ok {
					t.Fatalf("请求体里没有 env: %s", rt.body)
				}
				if got != float64(env) {
					t.Fatalf("env 应当是 %d，实际 %v：%s", env, got, rt.body)
				}
				// 沙箱那一路：pay_sig 必须盖住这份带 env=1 的字节。
				if want := testPaySig("K", c.uri, string(rt.body)); rt.query.Get("pay_sig") != want {
					t.Fatalf("pay_sig 与带 env=%d 的请求体对不上", env)
				}
			})
		}
	}
}

// 提现单号按官方规范（[8,32]、字母/数字/_/-）在本地拦下来；顺带确认「合法边界放行」——
// 否则上面这张表可能只是「什么都拦」。
func TestWithdrawNoValidation(t *testing.T) {
	cases := []struct {
		name     string
		no       string
		wantPass bool
	}{
		{"正好 8 位", "W1234567", true},
		{"32 位", strings.Repeat("a", 32), true},
		{"带下划线与横线", "ab_cd-ef", true},
		{"7 位，太短", "W123456", false},
		{"33 位，太长", strings.Repeat("a", 33), false},
		{"含 * 号", "W1234567*", false},
		{"含中文", "提现单号1234", false},
		{"空串", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			_, err := CreateWithdrawOrder(context.Background(), "T", "K",
				CreateWithdrawOrderRequest{WithdrawNo: c.no, WithdrawAmount: "0.01"})
			if c.wantPass && err != nil {
				t.Fatalf("应当放行，实际: %v", err)
			}
			if !c.wantPass && err == nil {
				t.Fatal("应当被拦下来")
			}
			// 拦住时一个字节都不该发出去。
			if err != nil && rt.calls != 0 {
				t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
			}
		})
	}
}

// 查询提现单**只查非空**，不查创建那一页的格式规范——读路径不该被格式挡住（理由见
// QueryWithdrawOrderRequest.validate）。所以这里刻意用一个「不符合创建规范」的单号，
// 确认它**能发出去**。谁要是把 withdrawNoRe 也加到查询上，这条会红。
func TestQueryWithdrawOrderAcceptsLegacyNoFormat(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0}`}
	swapXpay(t, rt)

	_, err := QueryWithdrawOrder(context.Background(), "T", "K",
		QueryWithdrawOrderRequest{WithdrawNo: "short"}) // 5 位，创建时不可能通过
	if err != nil {
		t.Fatalf("查询路径不该校验单号格式，实际被拦: %v", err)
	}
	if rt.calls != 1 {
		t.Fatalf("应当发出去一次，实际 %d", rt.calls)
	}
	// 空串仍然要拦——那是漏传，不是历史数据。
	rt.calls = 0
	if _, err := QueryWithdrawOrder(context.Background(), "T", "K", QueryWithdrawOrderRequest{}); err == nil {
		t.Fatal("WithdrawNo 为空应当报错")
	}
	if rt.calls != 0 {
		t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
	}
}

// 凭据与 env 的校验：三样都在本地拦，且一个字节都不发出去。
func TestFundsEndpointsRejectBadInputWithoutRequest(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		want string
		call func() error
	}{
		{"缺 accessToken", "accessToken", func() error {
			_, err := CreateWithdrawOrder(ctx, "", "K", CreateWithdrawOrderRequest{WithdrawNo: "W1234567"})
			return err
		}},
		{"缺 appKey", "appKey", func() error {
			_, err := QueryBizBalance(ctx, "T", "", QueryBizBalanceRequest{})
			return err
		}},
		{"env 取值非法", "Env 2 非法", func() error {
			_, err := QueryBizBalance(ctx, "T", "K", QueryBizBalanceRequest{Env: 2})
			return err
		}},
		{"沙箱下缺 appKey，报错要指明该配沙箱 key", "沙箱 AppKey", func() error {
			_, err := QueryBizBalance(ctx, "T", "", QueryBizBalanceRequest{Env: 1})
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

// 响应解析：金额与时间都是**字符串**（微信如此返回），状态是枚举，余额是嵌套对象。
//
// 这里刻意用 "100.00" 与 "1735660800" 这种「看着像数字」的字符串：它们必须原样落进
// string 字段，落进 int64 就会解析失败——而字段类型抄错正是这类接口最容易出的事。
func TestFundsResponseParsing(t *testing.T) {
	t.Run("query_withdraw_order", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok","withdraw_no":"W1234567",` +
			`"status":2,"withdraw_amount":"100.00","wx_withdraw_no":"WX1",` +
			`"withdraw_success_timestamp":"1735660800","create_time":"20260101000000","fail_reason":""}`}
		swapXpay(t, rt)

		resp, err := QueryWithdrawOrder(context.Background(), "T", "K",
			QueryWithdrawOrderRequest{WithdrawNo: "W1234567"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != WithdrawStatusSuccess {
			t.Errorf("status 应当是 %d，实际 %d", WithdrawStatusSuccess, resp.Status)
		}
		if resp.WithdrawAmount != "100.00" {
			t.Errorf("金额是元、字符串形式，原值应当是 %q，实际 %q", "100.00", resp.WithdrawAmount)
		}
		if resp.WithdrawSuccessTimestamp != "1735660800" {
			t.Errorf("时间戳也是字符串，原值应当是 %q，实际 %q", "1735660800", resp.WithdrawSuccessTimestamp)
		}
		if resp.WxWithdrawNo != "WX1" || resp.CreateTime != "20260101000000" {
			t.Errorf("字段没填对: %+v", resp)
		}
		if resp.ErrCode != 0 || resp.ErrMsg != "ok" {
			t.Errorf("公共头也要原值带出来，实际 errcode=%d errmsg=%q", resp.ErrCode, resp.ErrMsg)
		}
	})

	t.Run("query_biz_balance", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"balance_available":{"amount":"0.01","currency_code":"CNY"}}`}
		swapXpay(t, rt)

		resp, err := QueryBizBalance(context.Background(), "T", "K", QueryBizBalanceRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if resp.BalanceAvailable.Amount != "0.01" || resp.BalanceAvailable.CurrencyCode != "CNY" {
			t.Fatalf("嵌套的余额对象没解析对: %+v", resp.BalanceAvailable)
		}
	})

	t.Run("提现失败时原因原值带出", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"status":3,"fail_reason":"账户余额不足"}`}
		swapXpay(t, rt)

		resp, err := QueryWithdrawOrder(context.Background(), "T", "K",
			QueryWithdrawOrderRequest{WithdrawNo: "W1234567"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != WithdrawStatusFailed || resp.FailReason != "账户余额不足" {
			t.Fatalf("字段没填对: %+v", resp)
		}
	})

	t.Run("业务失败时响应仍原值返回、err 为 nil", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":268490003,"errmsg":"pay sig error"}`}
		swapXpay(t, rt)

		resp, err := QueryBizBalance(context.Background(), "T", "K", QueryBizBalanceRequest{})
		if err != nil {
			t.Fatalf("业务失败不该变成 error: %v", err)
		}
		if resp.ErrCode != 268490003 || resp.ErrMsg != "pay sig error" {
			t.Fatalf("errcode/errmsg 必须是原值，实际 %d %q", resp.ErrCode, resp.ErrMsg)
		}
	})
}
