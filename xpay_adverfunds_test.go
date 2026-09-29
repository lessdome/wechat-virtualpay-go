package wechat_virtualpay_go

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// 广告金类的字段表，照 TestOrderFieldsCoverDoc 那一套核：名字、类型、**行序**三项都要与
// 官方字段表一致，且不多不少。
//
// 这里最要紧的类型是 AdFundFilter.FundType 的**指针**：换成值类型一样能让别的测试全绿，
// 而「不筛类型」与「筛通用赠送」会挤在同一个零值上（见 TestAdverFundsFundTypePointer）。
func TestAdverFundsFieldsCoverDoc(t *testing.T) {
	tStr := reflect.TypeOf("")
	tInt := reflect.TypeOf(int(0))
	tI64 := reflect.TypeOf(int64(0))
	tState := reflect.TypeOf(TransferAccountState(0))
	tBindResult := reflect.TypeOf(TransferAccountBindResult(0))
	tFundType := reflect.TypeOf(AdFundType(0))
	tFundTypePtr := reflect.TypeOf((*AdFundType)(nil))
	tBillStatus := reflect.TypeOf(FundsBillStatus(0))
	tAdFundFilter := reflect.TypeOf((*AdFundFilter)(nil))
	tFundsBillFilter := reflect.TypeOf(FundsBillFilter{})
	tRecoverBillFilter := reflect.TypeOf(RecoverBillFilter{})
	tAcctList := reflect.TypeOf([]TransferAccount(nil))
	tAdverFundList := reflect.TypeOf([]AdverFund(nil))
	tFundsBillList := reflect.TypeOf([]FundsBill(nil))
	tRecoverBillList := reflect.TypeOf([]RecoverBill(nil))
	tStringList := reflect.TypeOf([]string(nil))

	cases := []struct {
		what string
		val  any
		doc  []fieldSpec
	}{
		{"TransferAccount", TransferAccount{}, []fieldSpec{
			{"transfer_account_name", tStr}, {"transfer_account_uid", tI64},
			{"transfer_account_agency_id", tI64}, {"transfer_account_agency_name", tStr},
			{"state", tState}, {"bind_result", tBindResult}, {"error_msg", tStr},
		}},
		{"QueryTransferAccountRequest", QueryTransferAccountRequest{}, []fieldSpec{
			{"env", tInt},
		}},
		{"QueryTransferAccountResponse", QueryTransferAccountResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"acct_list", tAcctList},
		}},

		{"AdFundFilter", AdFundFilter{}, []fieldSpec{
			{"settle_begin", tI64}, {"settle_end", tI64}, {"fund_type", tFundTypePtr},
		}},
		{"QueryAdverFundsRequest", QueryAdverFundsRequest{}, []fieldSpec{
			{"page", tInt}, {"page_size", tInt}, {"filter", tAdFundFilter}, {"env", tInt},
		}},
		{"AdverFund", AdverFund{}, []fieldSpec{
			{"settle_begin", tI64}, {"settle_end", tI64}, {"total_amount", tI64},
			{"remain_amount", tI64}, {"expire_time", tI64}, {"fund_type", tFundType},
			{"fund_id", tStr},
		}},
		{"QueryAdverFundsResponse", QueryAdverFundsResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"adver_funds_list", tAdverFundList},
			{"total_page", tInt},
		}},

		{"CreateFundsBillRequest", CreateFundsBillRequest{}, []fieldSpec{
			{"transfer_amount", tI64}, {"transfer_account_uid", tI64},
			{"transfer_account_name", tStr}, {"transfer_account_agency_id", tI64},
			{"request_id", tStr}, {"settle_begin", tI64}, {"settle_end", tI64},
			{"authorize_advertise", tInt}, {"fund_type", tFundType}, {"env", tInt},
		}},
		{"CreateFundsBillResponse", CreateFundsBillResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"bill_id", tStr},
		}},

		{"BindTransferAccountRequest", BindTransferAccountRequest{}, []fieldSpec{
			{"transfer_account_uid", tI64}, {"transfer_account_org_name", tStr}, {"env", tInt},
		}},
		// 只有公共头：绑定成没成只能从 errcode 读。
		{"BindTransferAccountResponse", BindTransferAccountResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
		}},

		{"FundsBillFilter", FundsBillFilter{}, []fieldSpec{
			{"oper_time_begin", tI64}, {"oper_time_end", tI64},
			{"bill_id", tStr}, {"request_id", tStr},
		}},
		{"QueryFundsBillRequest", QueryFundsBillRequest{}, []fieldSpec{
			{"page", tInt}, {"page_size", tInt}, {"filter", tFundsBillFilter}, {"env", tInt},
		}},
		{"FundsBill", FundsBill{}, []fieldSpec{
			{"bill_id", tStr}, {"oper_time", tI64}, {"settle_begin", tI64}, {"settle_end", tI64},
			{"fund_id", tStr}, {"transfer_account_name", tStr}, {"transfer_account_uid", tI64},
			{"transfer_amount", tI64}, {"status", tBillStatus}, {"request_id", tStr},
		}},
		{"QueryFundsBillResponse", QueryFundsBillResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"bill_list", tFundsBillList},
			{"total_page", tInt},
		}},

		{"RecoverBillFilter", RecoverBillFilter{}, []fieldSpec{
			{"recover_time_begin", tI64}, {"recover_time_end", tI64}, {"bill_id", tStr},
		}},
		{"QueryRecoverBillRequest", QueryRecoverBillRequest{}, []fieldSpec{
			{"page", tInt}, {"page_size", tInt}, {"filter", tRecoverBillFilter}, {"env", tInt},
		}},
		{"RecoverBill", RecoverBill{}, []fieldSpec{
			{"bill_id", tStr}, {"recover_time", tI64}, {"settle_begin", tI64}, {"settle_end", tI64},
			{"fund_id", tStr}, {"recover_account_name", tStr}, {"recover_amount", tI64},
			{"refund_order_list", tStringList},
		}},
		{"QueryRecoverBillResponse", QueryRecoverBillResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"bill_list", tRecoverBillList},
			{"total_page", tInt},
		}},

		{"DownloadAdverFundsOrderRequest", DownloadAdverFundsOrderRequest{}, []fieldSpec{
			{"fund_id", tStr}, {"env", tInt},
		}},
		{"DownloadAdverFundsOrderResponse", DownloadAdverFundsOrderResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"url", tStr},
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
	// 7+1+3 + 3+4+7+4 + 10+3 + 3+2 + 4+4+10+4 + 3+4+8+4 + 2+3
	// （每段是按接口分的：查询账户 / 发放记录 / 充值 / 绑定 / 充值记录 / 回收记录 / 下载）。
	if total != 93 {
		t.Fatalf("上面 %d 个结构体的文档字段总数应为 93，实际 %d（期望集合可能抄漏）", len(cases), total)
	}
}

// 四套枚举的取值。抄错数字不会报错，只会把「审核通过」读成「已驳回」这类。
//
// ⚠️ TransferAccountBindResult **从 1 开始**（0 不在集合里）——这里把它钉住：谁要是顺手
// 加个 0 值常量，等于给「字段没回」编了一个结果。
func TestAdverFundsEnumValues(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"TransferAccountPending", int(TransferAccountPending), 0},
		{"TransferAccountApproved", int(TransferAccountApproved), 1},
		{"TransferAccountRejected", int(TransferAccountRejected), 2},

		{"TransferAccountBindOK", int(TransferAccountBindOK), 1},
		{"TransferAccountBindFail", int(TransferAccountBindFail), 2},

		{"AdFundTypeGeneral", int(AdFundTypeGeneral), 0},
		{"AdFundTypeAd", int(AdFundTypeAd), 1},
		{"AdFundTypeTarget", int(AdFundTypeTarget), 2},

		{"FundsBillProcessing", int(FundsBillProcessing), 0},
		{"FundsBillSuccess", int(FundsBillSuccess), 1},
		{"FundsBillFailed", int(FundsBillFailed), 2},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d，文档是 %d", c.name, c.got, c.want)
		}
	}
}

// 本类最要紧的一条：**七个接口一个签名都不带**。
//
// 官方这几页的请求体里写着 env「仅作为签名校验」，跟它们自己的参数表互相矛盾；本包按参数表
// 实现（理由见文件头）。所以这里逐个钉：query 里只有 access_token，既没有 pay_sig 也没有
// signature。谁要是哪天按那句模板文字把它们升到 pay_sig 档，这条会红。
func TestAllAdverFundsEndpointsAreTokenOnly(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0}`}
	swapXpay(t, rt)

	fundType := AdFundTypeGeneral
	calls := map[string]struct {
		uri  string
		call func(ctx context.Context) error
	}{
		"query_transfer_account": {"/xpay/query_transfer_account", func(ctx context.Context) error {
			_, err := QueryTransferAccount(ctx, "T", QueryTransferAccountRequest{})
			return err
		}},
		"query_adver_funds": {"/xpay/query_adver_funds", func(ctx context.Context) error {
			_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{
				Page: 1, PageSize: 10, Filter: &AdFundFilter{FundType: &fundType}})
			return err
		}},
		"create_funds_bill": {"/xpay/create_funds_bill", func(ctx context.Context) error {
			_, err := CreateFundsBill(ctx, "T", CreateFundsBillRequest{
				TransferAmount: 100, TransferAccountUID: 7, TransferAccountName: "甲",
				TransferAccountAgencyID: 9, RequestID: "r1", SettleBegin: 100, SettleEnd: 200})
			return err
		}},
		// ⚠️ 路径里的 accout 是官方拼写（少一个 n）。
		"bind_transfer_accout": {"/xpay/bind_transfer_accout", func(ctx context.Context) error {
			_, err := BindTransferAccount(ctx, "T", BindTransferAccountRequest{
				TransferAccountUID: 7, TransferAccountOrgName: "甲"})
			return err
		}},
		"query_funds_bill": {"/xpay/query_funds_bill", func(ctx context.Context) error {
			_, err := QueryFundsBill(ctx, "T", QueryFundsBillRequest{
				Page: 1, PageSize: 10, Filter: FundsBillFilter{OperTimeBegin: 100, OperTimeEnd: 200}})
			return err
		}},
		"query_recover_bill": {"/xpay/query_recover_bill", func(ctx context.Context) error {
			_, err := QueryRecoverBill(ctx, "T", QueryRecoverBillRequest{
				Page: 1, PageSize: 10,
				Filter: RecoverBillFilter{RecoverTimeBegin: 100, RecoverTimeEnd: 200, BillID: "B1"}})
			return err
		}},
		"download_adverfunds_order": {"/xpay/download_adverfunds_order", func(ctx context.Context) error {
			_, err := DownloadAdverFundsOrder(ctx, "T", DownloadAdverFundsOrderRequest{FundID: "F1"})
			return err
		}},
	}

	for name, c := range calls {
		t.Run(name, func(t *testing.T) {
			rt.calls = 0
			if err := c.call(context.Background()); err != nil {
				t.Fatal(err)
			}
			if rt.calls != 1 {
				t.Fatalf("应当只发一次请求，实际 %d", rt.calls)
			}
			if rt.path != c.uri {
				t.Errorf("路径不对: %s（期望 %s）", rt.path, c.uri)
			}
			if got := rt.query.Get("access_token"); got != "T" {
				t.Errorf("access_token 应当是 T，实际 %q", got)
			}
			if got := rt.query.Get("pay_sig"); got != "" {
				t.Errorf("这一档不该有 pay_sig（本类不签名），实际 %q", got)
			}
			if got := rt.query.Get("signature"); got != "" {
				t.Errorf("这一档不该有 signature，实际 %q", got)
			}
		})
	}
}

// 七个接口的请求体**逐字节**对：键序即字段声明顺序（结构体自己带了 env，所以 requestBody
// 原样发、不重排）。
//
// 这里钉住两处最容易改错的字节：
//   - AdFundFilter.FundType 的**指针**：&0 要发出 "fund_type":0，nil 要整个省掉（见下一条）。
//   - 各 filter 里 omitempty 的那几个字段：空串必须**不在**请求体里。
func TestAdverFundsRequestBodies(t *testing.T) {
	general := AdFundTypeGeneral

	cases := []struct {
		name string
		uri  string
		call func(ctx context.Context) error
		want string
	}{
		{
			"query_transfer_account",
			"/xpay/query_transfer_account",
			func(ctx context.Context) error {
				_, err := QueryTransferAccount(ctx, "T", QueryTransferAccountRequest{})
				return err
			},
			`{"env":0}`,
		},
		{
			"query_adver_funds 不筛（全空）",
			"/xpay/query_adver_funds",
			func(ctx context.Context) error {
				_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{})
				return err
			},
			`{"env":0}`,
		},
		{
			"query_adver_funds 全填",
			"/xpay/query_adver_funds",
			func(ctx context.Context) error {
				_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{
					Page: 1, PageSize: 10,
					Filter: &AdFundFilter{SettleBegin: 100, SettleEnd: 200, FundType: &general}})
				return err
			},
			`{"page":1,"page_size":10,"filter":{"settle_begin":100,"settle_end":200,"fund_type":0},"env":0}`,
		},
		{
			"query_adver_funds 只筛时间（FundType 为 nil，必须整条省掉）",
			"/xpay/query_adver_funds",
			func(ctx context.Context) error {
				_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{
					Filter: &AdFundFilter{SettleBegin: 100, SettleEnd: 200}})
				return err
			},
			`{"filter":{"settle_begin":100,"settle_end":200},"env":0}`,
		},
		{
			// 与上一条「不筛」放一起看：都是「三个字段全空」，线上字节却不同——nil 时
			// filter 整栏不出现，&AdFundFilter{} 会发出一个空对象。omitempty 只省 nil
			// 指针，不省指向零值的非 nil 指针。别把这两条合并。
			"query_adver_funds 传一个空 filter（≠ 不传）",
			"/xpay/query_adver_funds",
			func(ctx context.Context) error {
				_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{Filter: &AdFundFilter{}})
				return err
			},
			`{"filter":{},"env":0}`,
		},
		{
			"create_funds_bill",
			"/xpay/create_funds_bill",
			func(ctx context.Context) error {
				_, err := CreateFundsBill(ctx, "T", CreateFundsBillRequest{
					TransferAmount: 100, TransferAccountUID: 7, TransferAccountName: "甲",
					TransferAccountAgencyID: 9, RequestID: "r1",
					SettleBegin: 100, SettleEnd: 200, AuthorizeAdvertise: 1, FundType: AdFundTypeAd})
				return err
			},
			`{"transfer_amount":100,"transfer_account_uid":7,"transfer_account_name":"甲",` +
				`"transfer_account_agency_id":9,"request_id":"r1","settle_begin":100,"settle_end":200,` +
				`"authorize_advertise":1,"fund_type":1,"env":0}`,
		},
		{
			"bind_transfer_accout（路径拼写就是 accout）",
			"/xpay/bind_transfer_accout",
			func(ctx context.Context) error {
				_, err := BindTransferAccount(ctx, "T", BindTransferAccountRequest{
					TransferAccountUID: 7, TransferAccountOrgName: "甲"})
				return err
			},
			`{"transfer_account_uid":7,"transfer_account_org_name":"甲","env":0}`,
		},
		{
			"query_funds_bill 只用时间筛（bill_id/request_id 空着，必须省掉）",
			"/xpay/query_funds_bill",
			func(ctx context.Context) error {
				_, err := QueryFundsBill(ctx, "T", QueryFundsBillRequest{
					Page: 1, PageSize: 10,
					Filter: FundsBillFilter{OperTimeBegin: 100, OperTimeEnd: 200}})
				return err
			},
			`{"page":1,"page_size":10,"filter":{"oper_time_begin":100,"oper_time_end":200},"env":0}`,
		},
		{
			"query_funds_bill 用 RequestID 反查",
			"/xpay/query_funds_bill",
			func(ctx context.Context) error {
				_, err := QueryFundsBill(ctx, "T", QueryFundsBillRequest{
					Page: 1, PageSize: 10,
					Filter: FundsBillFilter{OperTimeBegin: 100, OperTimeEnd: 200, RequestID: "r1"}})
				return err
			},
			`{"page":1,"page_size":10,` +
				`"filter":{"oper_time_begin":100,"oper_time_end":200,"request_id":"r1"},"env":0}`,
		},
		{
			"query_recover_bill",
			"/xpay/query_recover_bill",
			func(ctx context.Context) error {
				_, err := QueryRecoverBill(ctx, "T", QueryRecoverBillRequest{
					Page: 1, PageSize: 10,
					Filter: RecoverBillFilter{RecoverTimeBegin: 100, RecoverTimeEnd: 200, BillID: "B1"}})
				return err
			},
			`{"page":1,"page_size":10,` +
				`"filter":{"recover_time_begin":100,"recover_time_end":200,"bill_id":"B1"},"env":0}`,
		},
		{
			"download_adverfunds_order",
			"/xpay/download_adverfunds_order",
			func(ctx context.Context) error {
				_, err := DownloadAdverFundsOrder(ctx, "T", DownloadAdverFundsOrderRequest{FundID: "F1"})
				return err
			},
			`{"fund_id":"F1","env":0}`,
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
				t.Errorf("路径不对: %s（期望 %s）", rt.path, c.uri)
			}
			if got := string(rt.body); got != c.want {
				t.Fatalf("请求体不对\n实际: %s\n期望: %s", got, c.want)
			}
		})
	}
}

// FundType 用指针的**唯一**理由：AdFundType 的零值是一个有意义的取值（通用赠送）。
//
// 这条把两个意图分开钉：&0 发 "fund_type":0（筛通用赠送），nil 整条不发（不筛）。若把它
// 改成值类型，前者会退化成后者——而后者是「什么都没筛」的查询，看起来还像成功了。
func TestAdverFundsFundTypePointer(t *testing.T) {
	general := AdFundTypeGeneral
	ad := AdFundTypeAd

	cases := []struct {
		name   string
		filter *AdFundFilter
		want   string
	}{
		{"不筛类型", &AdFundFilter{SettleBegin: 100, SettleEnd: 200},
			`{"settle_begin":100,"settle_end":200}`},
		{"筛通用赠送（零值，指针才表达得出）", &AdFundFilter{FundType: &general},
			`{"fund_type":0}`},
		{"筛广告激励", &AdFundFilter{FundType: &ad},
			`{"fund_type":1}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			if _, err := QueryAdverFunds(context.Background(), "T",
				QueryAdverFundsRequest{Filter: c.filter}); err != nil {
				t.Fatal(err)
			}
			got, ok := bodyEnv(t, rt.body)["filter"]
			if !ok {
				t.Fatalf("请求体里没有 filter: %s", rt.body)
			}
			// map[string]any 的键在 encoding/json 里是**字典序**，所以这里的期望字节
			// 按 fund_type < settle_begin < settle_end 排。
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != c.want {
				t.Fatalf("filter 不对\n实际: %s\n期望: %s", raw, c.want)
			}
		})
	}
}

// env 在这一类也是请求体上的裸 int；填 1 就是 1。（本类不签名、也不切换数据源，但字段
// 该带还是得带——官方字段表有这一行。）
func TestAllAdverFundsEndpointsSendEnv(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0}`}
	swapXpay(t, rt)

	calls := map[string]func(ctx context.Context, env int) error{
		"query_transfer_account": func(ctx context.Context, env int) error {
			_, err := QueryTransferAccount(ctx, "T", QueryTransferAccountRequest{Env: env})
			return err
		},
		"query_adver_funds": func(ctx context.Context, env int) error {
			_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{Env: env})
			return err
		},
		"create_funds_bill": func(ctx context.Context, env int) error {
			_, err := CreateFundsBill(ctx, "T", CreateFundsBillRequest{
				TransferAmount: 100, TransferAccountUID: 7, TransferAccountName: "甲",
				TransferAccountAgencyID: 9, RequestID: "r1", SettleBegin: 100, SettleEnd: 200, Env: env})
			return err
		},
		"bind_transfer_accout": func(ctx context.Context, env int) error {
			_, err := BindTransferAccount(ctx, "T", BindTransferAccountRequest{
				TransferAccountUID: 7, TransferAccountOrgName: "甲", Env: env})
			return err
		},
		"query_funds_bill": func(ctx context.Context, env int) error {
			_, err := QueryFundsBill(ctx, "T", QueryFundsBillRequest{
				Page: 1, PageSize: 10, Filter: FundsBillFilter{OperTimeBegin: 100, OperTimeEnd: 200}, Env: env})
			return err
		},
		"query_recover_bill": func(ctx context.Context, env int) error {
			_, err := QueryRecoverBill(ctx, "T", QueryRecoverBillRequest{
				Page: 1, PageSize: 10,
				Filter: RecoverBillFilter{RecoverTimeBegin: 100, RecoverTimeEnd: 200, BillID: "B1"}, Env: env})
			return err
		},
		"download_adverfunds_order": func(ctx context.Context, env int) error {
			_, err := DownloadAdverFundsOrder(ctx, "T",
				DownloadAdverFundsOrderRequest{FundID: "F1", Env: env})
			return err
		},
	}

	for name, call := range calls {
		for _, env := range []int{0, 1} {
			t.Run(name+"/env="+strconv.Itoa(env), func(t *testing.T) {
				rt.calls = 0
				if err := call(context.Background(), env); err != nil {
					t.Fatal(err)
				}
				got, ok := bodyEnv(t, rt.body)["env"]
				if !ok {
					t.Fatalf("请求体里没有 env: %s", rt.body)
				}
				if got != float64(env) {
					t.Fatalf("env 应当是 %d，实际 %v：%s", env, got, rt.body)
				}
			})
		}
	}
}

// 本地校验：该拦的拦（且一个字节都不发），该放的放。
//
// 「该放的放」那一半是防止日后有人「顺手」把约束加严——比如给可选的时间戳加上必填。
func TestAdverFundsValidation(t *testing.T) {
	ctx := context.Background()

	okCreate := func() CreateFundsBillRequest {
		return CreateFundsBillRequest{
			TransferAmount: 100, TransferAccountUID: 7, TransferAccountName: "甲",
			TransferAccountAgencyID: 9, RequestID: "r1", SettleBegin: 100, SettleEnd: 200}
	}
	okFundsBill := func() QueryFundsBillRequest {
		return QueryFundsBillRequest{
			Page: 1, PageSize: 10, Filter: FundsBillFilter{OperTimeBegin: 100, OperTimeEnd: 200}}
	}
	okRecover := func() QueryRecoverBillRequest {
		return QueryRecoverBillRequest{
			Page: 1, PageSize: 10,
			Filter: RecoverBillFilter{RecoverTimeBegin: 100, RecoverTimeEnd: 200, BillID: "B1"}}
	}

	cases := []struct {
		name    string
		wantErr string // 空串＝应当放行
		call    func() error
	}{
		// ---- query_transfer_account：只有 env 可查。
		{"Env 不是 0/1", "Env 2 非法", func() error {
			_, err := QueryTransferAccount(ctx, "T", QueryTransferAccountRequest{Env: 2})
			return err
		}},

		// ---- query_adver_funds：分页可省，筛可选，但写反了要拦。
		{"Page 为负", "Page -1 非法", func() error {
			_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{Page: -1})
			return err
		}},
		{"Page 留 0（＝不传这一项）", "", func() error {
			_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{})
			return err
		}},
		{"PageSize 为负", "PageSize -1 非法", func() error {
			_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{PageSize: -1})
			return err
		}},
		{"结算周期写反了", "早于开始时间", func() error {
			_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{
				Filter: &AdFundFilter{SettleBegin: 200, SettleEnd: 100}})
			return err
		}},
		{"只填结算周期的一头（合法）", "", func() error {
			_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{
				Filter: &AdFundFilter{SettleBegin: 100}})
			return err
		}},
		{"结算周期两端相等（退化的闭区间，放行）", "", func() error {
			_, err := QueryAdverFunds(ctx, "T", QueryAdverFundsRequest{
				Filter: &AdFundFilter{SettleBegin: 100, SettleEnd: 100}})
			return err
		}},

		// ---- create_funds_bill：钱、账户三件套、幂等键。
		{"充值金额为 0", "TransferAmount 必须大于 0", func() error {
			r := okCreate()
			r.TransferAmount = 0
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"充值金额为负", "TransferAmount 必须大于 0", func() error {
			r := okCreate()
			r.TransferAmount = -1
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"账户 uid 为 0", "TransferAccountUID 不能为 0", func() error {
			r := okCreate()
			r.TransferAccountUID = 0
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"账户名为空", "TransferAccountName 不能为空", func() error {
			r := okCreate()
			r.TransferAccountName = ""
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"服务商 id 为 0", "TransferAccountAgencyID 不能为 0", func() error {
			r := okCreate()
			r.TransferAccountAgencyID = 0
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"幂等键为空", "RequestID 不能为空", func() error {
			r := okCreate()
			r.RequestID = ""
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"幂等键 1024 字符（边界，放行）", "", func() error {
			r := okCreate()
			r.RequestID = strings.Repeat("a", 1024)
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"幂等键 1025 字符", "1024", func() error {
			r := okCreate()
			r.RequestID = strings.Repeat("a", 1025)
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		// 上面两条是 ASCII——一个字符一字节，字符数、字节数、消息里的数字三者相同，所以
		// 它们钉不住「按字符还是按字节」。官方那句话的原话是「不超过 1024 字符」，下面这两条
		// 才真的钉住：1024 个汉字是 3072 字节，按字节算会被本地错杀，而报错里那个数也必须是
		// 字符数（1025）而不是字节数（3075）。
		{"幂等键 1024 个汉字（3072 字节，按字符算要放行）", "", func() error {
			r := okCreate()
			r.RequestID = strings.Repeat("字", 1024)
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"幂等键 1025 个汉字", "1025", func() error {
			r := okCreate()
			r.RequestID = strings.Repeat("字", 1025)
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"结算周期没填（0＝漏填）", "1970", func() error {
			r := okCreate()
			r.SettleBegin, r.SettleEnd = 0, 0
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"结算周期写反了", "早于开始时间", func() error {
			r := okCreate()
			r.SettleBegin, r.SettleEnd = 200, 100
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"AuthorizeAdvertise 取 7（枚举不校验，交微信）", "", func() error {
			r := okCreate()
			r.AuthorizeAdvertise = 7
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},
		{"FundType 取 9（枚举不校验，交微信）", "", func() error {
			r := okCreate()
			r.FundType = AdFundType(9)
			_, err := CreateFundsBill(ctx, "T", r)
			return err
		}},

		// ---- bind_transfer_accout：两栏都按必填。
		{"绑定：uid 为 0", "TransferAccountUID 不能为 0", func() error {
			_, err := BindTransferAccount(ctx, "T", BindTransferAccountRequest{TransferAccountOrgName: "甲"})
			return err
		}},
		{"绑定：主体名为空", "TransferAccountOrgName 不能为空", func() error {
			_, err := BindTransferAccount(ctx, "T", BindTransferAccountRequest{TransferAccountUID: 7})
			return err
		}},

		// ---- query_funds_bill：分页与时间区间都必填。
		{"充值记录：Page 为 0", "Page 0 非法", func() error {
			r := okFundsBill()
			r.Page = 0
			_, err := QueryFundsBill(ctx, "T", r)
			return err
		}},
		{"充值记录：PageSize 为 0", "PageSize 0 非法", func() error {
			r := okFundsBill()
			r.PageSize = 0
			_, err := QueryFundsBill(ctx, "T", r)
			return err
		}},
		{"充值记录：时间区间没填", "1970", func() error {
			r := okFundsBill()
			r.Filter.OperTimeBegin, r.Filter.OperTimeEnd = 0, 0
			_, err := QueryFundsBill(ctx, "T", r)
			return err
		}},
		// BillID/RequestID 是**叠在时间区间之上**的精确条件，不是它的替代品——单号填了、
		// 时间区间没填，仍然在本地拦（官方字段表把两个时间戳标为必填，见 FundsBillFilter）。
		{"充值记录：只填单号、不给时间区间（仍要拦）", "1970", func() error {
			r := okFundsBill()
			r.Filter.OperTimeBegin, r.Filter.OperTimeEnd = 0, 0
			r.Filter.BillID = "B1"
			_, err := QueryFundsBill(ctx, "T", r)
			return err
		}},
		{"充值记录：时间区间写反了", "早于开始时间", func() error {
			r := okFundsBill()
			r.Filter.OperTimeBegin, r.Filter.OperTimeEnd = 200, 100
			_, err := QueryFundsBill(ctx, "T", r)
			return err
		}},
		{"充值记录：时间区间两端相等（放行）", "", func() error {
			r := okFundsBill()
			r.Filter.OperTimeBegin, r.Filter.OperTimeEnd = 100, 100
			_, err := QueryFundsBill(ctx, "T", r)
			return err
		}},
		{"充值记录：BillID 与 RequestID 都不填（合法，「只按时间筛」）", "", func() error {
			_, err := QueryFundsBill(ctx, "T", okFundsBill())
			return err
		}},

		// ---- query_recover_bill：BillID 按必填处理（官方字段表与说明文字打架的那一栏）。
		{"回收记录：BillID 为空", "BillID 不能为空", func() error {
			r := okRecover()
			r.Filter.BillID = ""
			_, err := QueryRecoverBill(ctx, "T", r)
			return err
		}},
		{"回收记录：时间区间没填", "1970", func() error {
			r := okRecover()
			r.Filter.RecoverTimeBegin, r.Filter.RecoverTimeEnd = 0, 0
			_, err := QueryRecoverBill(ctx, "T", r)
			return err
		}},
		{"回收记录：时间区间写反了", "早于开始时间", func() error {
			r := okRecover()
			r.Filter.RecoverTimeBegin, r.Filter.RecoverTimeEnd = 200, 100
			_, err := QueryRecoverBill(ctx, "T", r)
			return err
		}},

		// ---- download_adverfunds_order：FundID 必填。
		{"下载：FundID 为空", "FundID 不能为空", func() error {
			_, err := DownloadAdverFundsOrder(ctx, "T", DownloadAdverFundsOrderRequest{})
			return err
		}},

		// ---- 凭据那一层：本类七个都只收 accessToken。
		{"缺 accessToken", "accessToken", func() error {
			_, err := QueryAdverFunds(ctx, "", QueryAdverFundsRequest{})
			return err
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			err := c.call()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("应当放行，实际: %v", err)
				}
				if rt.calls != 1 {
					t.Fatalf("应当发出去一次，实际 %d", rt.calls)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("期望报错含 %q，实际: %v", c.wantErr, err)
			}
			if rt.calls != 0 {
				t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
			}
		})
	}
}

// 响应解析：七类响应各来一次，重点在几个「看着像成功」的地方。
//
// 最要紧的一条是 download_adverfunds_order 的**空 URL**：errcode=0、url 是空串，这是
// 「下载链接还在生成」的正常中间态，不是错误——所以本包不把它当失败，调用方要自己轮询
// （判据是 URL 非空，不是 errcode）。
func TestAdverFundsResponseParsing(t *testing.T) {
	t.Run("query_transfer_account 的账户列表与 error_msg", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok","acct_list":[` +
			`{"transfer_account_name":"甲","transfer_account_uid":7,"transfer_account_agency_id":9,` +
			`"transfer_account_agency_name":"服务商","state":1,"bind_result":1,"error_msg":""},` +
			`{"transfer_account_name":"乙","transfer_account_uid":8,"transfer_account_agency_id":9,` +
			`"transfer_account_agency_name":"服务商","state":2,"bind_result":2,"error_msg":"主体名称不符"}` +
			`]}`}
		swapXpay(t, rt)

		resp, err := QueryTransferAccount(context.Background(), "T", QueryTransferAccountRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.AcctList) != 2 {
			t.Fatalf("应当有两个账户，实际 %d", len(resp.AcctList))
		}
		if resp.AcctList[0].State != TransferAccountApproved ||
			resp.AcctList[0].BindResult != TransferAccountBindOK {
			t.Errorf("第一个账户没解析对: %+v", resp.AcctList[0])
		}
		if resp.AcctList[1].State != TransferAccountRejected ||
			resp.AcctList[1].ErrorMsg != "主体名称不符" {
			t.Errorf("第二个账户没解析对: %+v", resp.AcctList[1])
		}
		// 单个账户的 error_msg 与公共头的 errmsg 是两件事，别互相顶掉。
		if resp.ErrMsg != "ok" {
			t.Errorf("公共头的 errmsg 不该被账户的 error_msg 顶掉，实际 %q", resp.ErrMsg)
		}
	})

	t.Run("query_adver_funds 的金额与 fund_id", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"adver_funds_list":[` +
			`{"settle_begin":1700000000,"settle_end":1702592000,"total_amount":12345,` +
			`"remain_amount":1234,"expire_time":1730000000,"fund_type":1,"fund_id":"F1"}` +
			`],"total_page":3}`}
		swapXpay(t, rt)

		resp, err := QueryAdverFunds(context.Background(), "T", QueryAdverFundsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if resp.TotalPage != 3 {
			t.Errorf("TotalPage 应当是 3，实际 %d", resp.TotalPage)
		}
		f := resp.AdverFundsList[0]
		if f.TotalAmount != 12345 || f.RemainAmount != 1234 || f.FundType != AdFundTypeAd ||
			f.FundID != "F1" {
			t.Fatalf("字段没填对: %+v", f)
		}
	})

	t.Run("create_funds_bill 只回一个 bill_id", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"bill_id":"BILL1"}`}
		swapXpay(t, rt)

		resp, err := CreateFundsBill(context.Background(), "T", CreateFundsBillRequest{
			TransferAmount: 100, TransferAccountUID: 7, TransferAccountName: "甲",
			TransferAccountAgencyID: 9, RequestID: "r1", SettleBegin: 100, SettleEnd: 200})
		if err != nil {
			t.Fatal(err)
		}
		if resp.BillID != "BILL1" {
			t.Fatalf("BillID 应当是 BILL1，实际 %q", resp.BillID)
		}
	})

	t.Run("query_funds_bill 的充值状态", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"bill_list":[` +
			`{"bill_id":"BILL1","oper_time":1700000001,"settle_begin":100,"settle_end":200,` +
			`"fund_id":"F1","transfer_account_name":"甲","transfer_account_uid":7,` +
			`"transfer_amount":100,"status":1,"request_id":"r1"}` +
			`],"total_page":1}`}
		swapXpay(t, rt)

		resp, err := QueryFundsBill(context.Background(), "T", QueryFundsBillRequest{
			Page: 1, PageSize: 10, Filter: FundsBillFilter{OperTimeBegin: 100, OperTimeEnd: 200}})
		if err != nil {
			t.Fatal(err)
		}
		b := resp.BillList[0]
		if b.Status != FundsBillSuccess || b.TransferAmount != 100 || b.RequestID != "r1" {
			t.Fatalf("字段没填对: %+v", b)
		}
	})

	t.Run("query_recover_bill 的退款单号是列表", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"bill_list":[` +
			`{"bill_id":"REC1","recover_time":1700000002,"settle_begin":100,"settle_end":200,` +
			`"fund_id":"F1","recover_account_name":"甲","recover_amount":50,` +
			`"refund_order_list":["R1","R2"]}` +
			`],"total_page":1}`}
		swapXpay(t, rt)

		resp, err := QueryRecoverBill(context.Background(), "T", QueryRecoverBillRequest{
			Page: 1, PageSize: 10,
			Filter: RecoverBillFilter{RecoverTimeBegin: 100, RecoverTimeEnd: 200, BillID: "REC1"}})
		if err != nil {
			t.Fatal(err)
		}
		if got := resp.BillList[0].RefundOrderList; !reflect.DeepEqual(got, []string{"R1", "R2"}) {
			t.Fatalf("退款单号列表没解析对: %v", got)
		}
	})

	t.Run("download_adverfunds_order 空 URL 是中间态，不是错误", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok","url":""}`}
		swapXpay(t, rt)

		resp, err := DownloadAdverFundsOrder(context.Background(), "T",
			DownloadAdverFundsOrderRequest{FundID: "F1"})
		if err != nil {
			t.Fatalf("空 URL 不是失败——链接还在生成: %v", err)
		}
		if resp.URL != "" {
			t.Fatalf("这一趟就是空串，实际 %q", resp.URL)
		}
		if resp.ErrCode != 0 {
			t.Fatalf("空 URL 时 errcode 是 0，实际 %d", resp.ErrCode)
		}
	})

	t.Run("bind_transfer_accout 只有公共头，失败也要读得到", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":268490003,"errmsg":"pay sig error"}`}
		swapXpay(t, rt)

		resp, err := BindTransferAccount(context.Background(), "T", BindTransferAccountRequest{
			TransferAccountUID: 7, TransferAccountOrgName: "甲"})
		if err != nil {
			t.Fatalf("业务失败不该变成 error: %v", err)
		}
		if resp == nil {
			t.Fatal("走通了就该有响应")
		}
		if resp.ErrCode != 268490003 || resp.ErrMsg != "pay sig error" {
			t.Fatalf("errcode/errmsg 必须是原值，实际 %d %q", resp.ErrCode, resp.ErrMsg)
		}
	})
}
