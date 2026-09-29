package wechat_virtualpay_go

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// 投诉类的字段表，照 TestOrderFieldsCoverDoc 那一套核：名字、类型、**行序**三项都要与
// 官方字段表一致，且不多不少。
func TestComplaintFieldsCoverDoc(t *testing.T) {
	tStr := reflect.TypeOf("")
	tInt := reflect.TypeOf(int(0))
	tI64 := reflect.TypeOf(int64(0))
	tBool := reflect.TypeOf(false)
	tState := reflect.TypeOf(ComplaintState(""))
	tProblem := reflect.TypeOf(ProblemType(""))
	tMediaType := reflect.TypeOf(ComplaintMediaType(""))
	tServiceState := reflect.TypeOf(ServiceOrderState(""))
	tOperate := reflect.TypeOf(OperateType(""))
	tOrderInfoList := reflect.TypeOf([]ComplaintOrderInfo(nil))
	tMediaList := reflect.TypeOf([]ComplaintMedia(nil))
	tServiceOrderList := reflect.TypeOf([]ServiceOrderInfo(nil))
	tUserTagList := reflect.TypeOf([]UserTag(nil))
	tComplaintList := reflect.TypeOf([]Complaint(nil))
	tComplaintPtr := reflect.TypeOf((*Complaint)(nil))
	tNegotiationList := reflect.TypeOf([]NegotiationEntry(nil))
	tStringList := reflect.TypeOf([]string(nil))
	tRecoveryList := reflect.TypeOf([]RecoverySpecification(nil))

	cases := []struct {
		what string
		val  any
		doc  []fieldSpec
	}{
		{"ComplaintOrderInfo", ComplaintOrderInfo{}, []fieldSpec{
			{"transaction_id", tStr}, {"out_trade_no", tStr}, {"amount", tI64},
			{"wxa_out_trade_no", tStr}, {"wx_order_id", tStr},
		}},
		{"ComplaintMedia", ComplaintMedia{}, []fieldSpec{
			{"media_type", tMediaType}, {"media_url", tStringList},
		}},
		{"ServiceOrderInfo", ServiceOrderInfo{}, []fieldSpec{
			{"order_id", tStr}, {"out_order_no", tStr}, {"state", tServiceState},
		}},
		{"Complaint", Complaint{}, []fieldSpec{
			{"complaint_id", tStr}, {"complaint_time", tStr}, {"complaint_detail", tStr},
			{"complaint_state", tState}, {"payer_phone", tStr}, {"payer_openid", tStr},
			{"complaint_order_info", tOrderInfoList}, {"complaint_full_refunded", tBool},
			{"incoming_user_response", tBool}, {"user_complaint_times", tInt},
			{"complaint_media_list", tMediaList}, {"problem_description", tStr},
			{"problem_type", tProblem}, {"apply_refund_amount", tI64},
			{"user_tag_list", tUserTagList}, {"service_order_info", tServiceOrderList},
		}},

		{"GetComplaintListRequest", GetComplaintListRequest{}, []fieldSpec{
			{"begin_date", tStr}, {"end_date", tStr}, {"offset", tInt},
			{"limit", tInt}, {"env", tInt},
		}},
		{"GetComplaintListResponse", GetComplaintListResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"total", tInt}, {"complaints", tComplaintList},
		}},

		{"GetComplaintDetailRequest", GetComplaintDetailRequest{}, []fieldSpec{
			{"complaint_id", tStr}, {"env", tInt},
		}},
		// complaint 是**指针**：errcode=0 但查不到内容时它是 nil（见 TestComplaintResponseParsing）。
		{"GetComplaintDetailResponse", GetComplaintDetailResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"complaint", tComplaintPtr},
		}},

		{"NegotiationEntry", NegotiationEntry{}, []fieldSpec{
			{"log_id", tStr}, {"operator", tStr}, {"operate_time", tStr},
			{"operate_type", tOperate}, {"operate_details", tStr},
			{"complaint_media_list", tMediaList},
		}},
		{"GetNegotiationHistoryRequest", GetNegotiationHistoryRequest{}, []fieldSpec{
			{"complaint_id", tStr}, {"offset", tInt}, {"limit", tInt}, {"env", tInt},
		}},
		{"GetNegotiationHistoryResponse", GetNegotiationHistoryResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"total", tInt},
			{"history", tNegotiationList},
		}},

		{"ResponseComplaintRequest", ResponseComplaintRequest{}, []fieldSpec{
			{"complaint_id", tStr}, {"response_content", tStr},
			{"response_images", tStringList}, {"env", tInt},
		}},
		// 只有公共头（老实现这里是裸 error）。
		{"ResponseComplaintResponse", ResponseComplaintResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
		}},

		{"CompleteComplaintRequest", CompleteComplaintRequest{}, []fieldSpec{
			{"complaint_id", tStr}, {"env", tInt},
		}},
		{"CompleteComplaintResponse", CompleteComplaintResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
		}},

		{"UploadVPFileRequest", UploadVPFileRequest{}, []fieldSpec{
			{"base64_img", tStr}, {"img_url", tStr}, {"file_name", tStr}, {"env", tInt},
		}},
		{"UploadVPFileResponse", UploadVPFileResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"file_id", tStr},
		}},

		{"GetUploadFileSignRequest", GetUploadFileSignRequest{}, []fieldSpec{
			{"wxpay_url", tStr}, {"convert_cos", tBool}, {"complaint_id", tStr}, {"env", tInt},
		}},
		{"GetUploadFileSignResponse", GetUploadFileSignResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"sign", tStr}, {"cos_url", tStr},
		}},

		{"RecoverySpecification", RecoverySpecification{}, []fieldSpec{
			{"limitation_case_id", tStr}, {"limitation_reason_type", tStr},
			{"limitation_reason", tStr}, {"limitation_reason_describe", tStr},
			{"relate_limitations", tStr}, {"other_relate_limitations", tStr},
			{"recover_way", tStr}, {"recover_way_param", tStr}, {"recover_help_url", tStr},
			{"limitation_action_type", tStr}, {"limitation_start_date", tStr},
			{"limitation_date", tStr},
		}},
		{"QueryPunishmentReasonsResponse", QueryPunishmentReasonsResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr}, {"appid", tStr}, {"nickname", tStr},
			{"merchant_code", tStr}, {"limited_functions", tStringList},
			{"other_limited_functions", tStr}, {"recovery_specifications", tRecoveryList},
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
	// 5+2+3+16 + 5+4 + 2+3 + 6+4+4 + 4+2 + 2+2 + 4+3 + 4+4 + 12 + 8。
	if total != 99 {
		t.Fatalf("上面 %d 个结构体的文档字段总数应为 99，实际 %d（期望集合可能抄漏）", len(cases), total)
	}
}

// 五个字符串枚举的取值。这些是**协议字节**，抄错一个字母就静默匹配不上。
func TestComplaintEnumValues(t *testing.T) {
	strCases := []struct {
		name string
		got  string
		want string
	}{
		{"ComplaintStatePending", string(ComplaintStatePending), "PENDING"},
		{"ComplaintStateProcessing", string(ComplaintStateProcessing), "PROCESSING"},
		{"ComplaintStateProcessed", string(ComplaintStateProcessed), "PROCESSED"},

		{"ProblemTypeRefund", string(ProblemTypeRefund), "REFUND"},
		{"ProblemTypeServiceNotWork", string(ProblemTypeServiceNotWork), "SERVICE_NOT_WORK"},
		{"ProblemTypeOthers", string(ProblemTypeOthers), "OTHERS"},

		{"UserTagTrusted", string(UserTagTrusted), "TRUSTED"},
		{"UserTagHighRisk", string(UserTagHighRisk), "HIGH_RISK"},

		{"ComplaintMediaUserImage", string(ComplaintMediaUserImage), "USER_COMPLAINT_IMAGE"},
		{"ComplaintMediaOperationImage", string(ComplaintMediaOperationImage), "OPERATION_IMAGE"},

		{"ServiceOrderDoing", string(ServiceOrderDoing), "DOING"},
		{"ServiceOrderRevoked", string(ServiceOrderRevoked), "REVOKED"},
		{"ServiceOrderWaitPay", string(ServiceOrderWaitPay), "WAITPAY"},
		{"ServiceOrderDone", string(ServiceOrderDone), "DONE"},
	}
	for _, c := range strCases {
		if c.got != c.want {
			t.Errorf("%s = %q，文档是 %q", c.name, c.got, c.want)
		}
	}
}

// 协商记录的 20 个操作类型，逐个对官方给的字符串。
//
// ⚠️ 其中两个是**微信官方拼错的**（comfirm / rerund），本包照抄。这条测试的用处正是：
// 谁要是「顺手把拼写改对」，这里立刻红——改对了在协议上就是改错了，比对永远不成立。
func TestOperateTypeValues(t *testing.T) {
	cases := []struct {
		name string
		got  OperateType
		want string
	}{
		{"OpUserCreateComplaint", OpUserCreateComplaint, "USER_CREATE_COMPLAINT"},
		{"OpUserContinueComplaint", OpUserContinueComplaint, "USER_CONTINUE_COMPLAINT"},
		{"OpUserResponse", OpUserResponse, "USER_RESPONSE"},
		{"OpPlatformResponse", OpPlatformResponse, "PLATFORM_RESPONSE"},
		{"OpMerchantResponse", OpMerchantResponse, "MERCHANT_RESPONSE"},
		{"OpMerchantConfirmComplete", OpMerchantConfirmComplete, "MERCHANT_CONFIRM_COMPLETE"},
		{"OpUserCreateComplaintSystem", OpUserCreateComplaintSystem, "USER_CREATE_COMPLAINT_SYSTEM_MESSAGE"},
		{"OpFullRefundedSystem", OpFullRefundedSystem, "COMPLAINT_FULL_REFUNDED_SYSTEM_MESSAGE"},
		{"OpPartialRefundedSystem", OpPartialRefundedSystem, "COMPLAINT_PARTIAL_REFUNDED_SYSTEM_MESSAGE"},
		{"OpRefundReceivedSystem", OpRefundReceivedSystem, "COMPLAINT_REFUND_RECEIVED_SYSTEM_MESSAGE"},
		{"OpUserContinueSystem", OpUserContinueSystem, "USER_CONTINUE_COMPLAINT_SYSTEM_MESSAGE"},
		{"OpUserRevokeComplaint", OpUserRevokeComplaint, "USER_REVOKE_COMPLAINT"},
		{"OpUserConfirmComplaint", OpUserConfirmComplaint, "USER_COMFIRM_COMPLAINT"},
		{"OpPlatformHelpApplication", OpPlatformHelpApplication, "PLATFORM_HELP_APPLICATION"},
		{"OpUserApplyPlatformHelp", OpUserApplyPlatformHelp, "USER_APPLY_PLATFORM_HELP"},
		{"OpMerchantApproveRefund", OpMerchantApproveRefund, "MERCHANT_APPROVE_REFUND"},
		{"OpMerchantRefuseRefund", OpMerchantRefuseRefund, "MERCHANT_REFUSE_RERUND"},
		{"OpUserSubmitSatisfaction", OpUserSubmitSatisfaction, "USER_SUBMIT_SATISFACTION"},
		{"OpServiceOrderCancel", OpServiceOrderCancel, "SERVICE_ORDER_CANCEL"},
		{"OpServiceOrderComplete", OpServiceOrderComplete, "SERVICE_ORDER_COMPLETE"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("%s = %q，文档是 %q", c.name, c.got, c.want)
		}
	}

	// 把「官方拼错了」这件事单独钉一次：照正确拼写写出来的常量必须**不等于**官方的那个值。
	if string(OpUserConfirmComplaint) == "USER_CONFIRM_COMPLAINT" {
		t.Error("OpUserConfirmComplaint 被「修好」了——官方原文是 USER_COMFIRM_COMPLAINT（少一个 R），改对即改错")
	}
	if string(OpMerchantRefuseRefund) == "MERCHANT_REFUSE_REFUND" {
		t.Error("OpMerchantRefuseRefund 被「修好」了——官方原文是 MERCHANT_REFUSE_RERUND（少一个 F），改对即改错")
	}
}

// 八个接口的路径与签名档位：全部是 access_token + pay_sig，且 pay_sig 盖住的正是发出去的
// 那份字节。
//
// ⚠️ 顺带钉住文档的一处毛病：get_complaint_list 的「注意事项」写「使用用户态签名与支付
// 签名」，但参数表只列 pay_sig——本包按参数表实现，所以这里**断言没有 signature**。
func TestAllComplaintEndpointsSignWithPaySig(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0}`}
	swapXpay(t, rt)

	calls := map[string]struct {
		uri  string
		call func(ctx context.Context) error
	}{
		"get_complaint_list": {"/xpay/get_complaint_list", func(ctx context.Context) error {
			_, err := GetComplaintList(ctx, "T", "K", GetComplaintListRequest{
				BeginDate: "2023-01-01", EndDate: "2023-01-31", Limit: 20})
			return err
		}},
		"get_complaint_detail": {"/xpay/get_complaint_detail", func(ctx context.Context) error {
			_, err := GetComplaintDetail(ctx, "T", "K", GetComplaintDetailRequest{ComplaintID: "C1"})
			return err
		}},
		"get_negotiation_history": {"/xpay/get_negotiation_history", func(ctx context.Context) error {
			_, err := GetNegotiationHistory(ctx, "T", "K",
				GetNegotiationHistoryRequest{ComplaintID: "C1", Limit: 10})
			return err
		}},
		"response_complaint": {"/xpay/response_complaint", func(ctx context.Context) error {
			_, err := ResponseComplaint(ctx, "T", "K", ResponseComplaintRequest{
				ComplaintID: "C1", ResponseContent: "已处理"})
			return err
		}},
		"complete_complaint": {"/xpay/complete_complaint", func(ctx context.Context) error {
			_, err := CompleteComplaint(ctx, "T", "K", CompleteComplaintRequest{ComplaintID: "C1"})
			return err
		}},
		"upload_vp_file": {"/xpay/upload_vp_file", func(ctx context.Context) error {
			_, err := UploadVPFile(ctx, "T", "K", UploadVPFileRequest{
				ImgURL: "https://x/a.png", FileName: "a.png"})
			return err
		}},
		"get_upload_file_sign": {"/xpay/get_upload_file_sign", func(ctx context.Context) error {
			_, err := GetUploadFileSign(ctx, "T", "K", GetUploadFileSignRequest{
				WxpayURL:    "https://api.mch.weixin.qq.com/v3/merchant-service/images/x",
				ComplaintID: "C1"})
			return err
		}},
		"query_punishment_reasons": {"/xpay/query_punishment_reasons", func(ctx context.Context) error {
			_, err := QueryPunishmentReasons(ctx, "T", "K")
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
			if got, want := rt.query.Get("pay_sig"), testPaySig("K", c.uri, string(rt.body)); got != want {
				t.Errorf("pay_sig 与发出去的请求体对不上\n实际: %s\n期望: %s", got, want)
			}
			if got := rt.query.Get("signature"); got != "" {
				t.Errorf("本类按参数表是 pay_sig 档，不该有 signature，实际 %q", got)
			}
			if got := rt.query.Get("access_token"); got != "T" {
				t.Errorf("access_token 应当是 T，实际 %q", got)
			}
		})
	}
}

// 请求体**逐字节**对：键序即字段声明顺序（结构体自己带了 env）。
func TestComplaintRequestBodies(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		call func(ctx context.Context) error
		want string
	}{
		{
			"get_complaint_list",
			"/xpay/get_complaint_list",
			func(ctx context.Context) error {
				_, err := GetComplaintList(ctx, "T", "K", GetComplaintListRequest{
					BeginDate: "2023-01-01", EndDate: "2023-01-31", Limit: 20})
				return err
			},
			`{"begin_date":"2023-01-01","end_date":"2023-01-31","offset":0,"limit":20,"env":0}`,
		},
		{
			"get_complaint_detail",
			"/xpay/get_complaint_detail",
			func(ctx context.Context) error {
				_, err := GetComplaintDetail(ctx, "T", "K", GetComplaintDetailRequest{ComplaintID: "C1"})
				return err
			},
			`{"complaint_id":"C1","env":0}`,
		},
		{
			"get_negotiation_history",
			"/xpay/get_negotiation_history",
			func(ctx context.Context) error {
				_, err := GetNegotiationHistory(ctx, "T", "K",
					GetNegotiationHistoryRequest{ComplaintID: "C1", Limit: 10})
				return err
			},
			`{"complaint_id":"C1","offset":0,"limit":10,"env":0}`,
		},
		{
			// 只回文字：response_images 带 omitempty，**整行不出现**（不是 null）。
			"response_complaint 只回文字",
			"/xpay/response_complaint",
			func(ctx context.Context) error {
				_, err := ResponseComplaint(ctx, "T", "K", ResponseComplaintRequest{
					ComplaintID: "C1", ResponseContent: "已处理，退款已原路返回"})
				return err
			},
			`{"complaint_id":"C1","response_content":"已处理，退款已原路返回","env":0}`,
		},
		{
			// 只回图片：response_content 没有 omitempty，空串照样发（它在字段表里）。
			"response_complaint 只回图片",
			"/xpay/response_complaint",
			func(ctx context.Context) error {
				_, err := ResponseComplaint(ctx, "T", "K", ResponseComplaintRequest{
					ComplaintID: "C1", ResponseImages: []string{"F1", "F2"}})
				return err
			},
			`{"complaint_id":"C1","response_content":"","response_images":["F1","F2"],"env":0}`,
		},
		{
			"complete_complaint",
			"/xpay/complete_complaint",
			func(ctx context.Context) error {
				_, err := CompleteComplaint(ctx, "T", "K", CompleteComplaintRequest{ComplaintID: "C1"})
				return err
			},
			`{"complaint_id":"C1","env":0}`,
		},
		{
			"upload_vp_file 传 URL（base64_img 不出现）",
			"/xpay/upload_vp_file",
			func(ctx context.Context) error {
				_, err := UploadVPFile(ctx, "T", "K", UploadVPFileRequest{
					ImgURL: "https://x/a.png", FileName: "a.png"})
				return err
			},
			`{"img_url":"https://x/a.png","file_name":"a.png","env":0}`,
		},
		{
			"upload_vp_file 传 base64（img_url 不出现）",
			"/xpay/upload_vp_file",
			func(ctx context.Context) error {
				_, err := UploadVPFile(ctx, "T", "K", UploadVPFileRequest{
					Base64Img: "AAAA", FileName: "a.png"})
				return err
			},
			`{"base64_img":"AAAA","file_name":"a.png","env":0}`,
		},
		{
			"get_upload_file_sign",
			"/xpay/get_upload_file_sign",
			func(ctx context.Context) error {
				_, err := GetUploadFileSign(ctx, "T", "K", GetUploadFileSignRequest{
					WxpayURL:    "https://api.mch.weixin.qq.com/v3/merchant-service/images/x",
					ConvertCOS:  true,
					ComplaintID: "C1"})
				return err
			},
			`{"wxpay_url":"https://api.mch.weixin.qq.com/v3/merchant-service/images/x",` +
				`"convert_cos":true,"complaint_id":"C1","env":0}`,
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

// query_punishment_reasons 的请求体是全包唯一的例外：**官方写「请求体：无」**，本包发一份
// 空的 `{}` 出去（签名要签它），并且**不补 env**——官方那一页连请求体都没有，自然没有 env
// 这一行（同账单类那两个「字段表里没 env」的接口，是同一道理的更轻版本）。
//
// 谁要是把这个 marker 去掉，requestBody 会补出 {"env":0}，这条立刻红。
func TestQueryPunishmentReasonsSendsEmptyBody(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0}`}
	swapXpay(t, rt)

	if _, err := QueryPunishmentReasons(context.Background(), "T", "K"); err != nil {
		t.Fatal(err)
	}
	if rt.path != "/xpay/query_punishment_reasons" {
		t.Errorf("路径不对: %s", rt.path)
	}
	if got := string(rt.body); got != "{}" {
		t.Fatalf("请求体应当是 {}，实际: %s", got)
	}
	if _, ok := bodyEnv(t, rt.body)["env"]; ok {
		t.Fatal("本接口没有请求体，不该补出 env")
	}
	// 签名盖住的必须就是那份 {}。
	if got, want := rt.query.Get("pay_sig"), testPaySig("K", "/xpay/query_punishment_reasons", "{}"); got != want {
		t.Errorf("pay_sig 与 {} 对不上\n实际: %s\n期望: %s", got, want)
	}
	// marker 与空结构体必须配套：类型断言过不去就说明实现被换掉了。
	if _, ok := any(punishmentReasonsBody{}).(xpayNoEnvRequest); !ok {
		t.Fatal("punishmentReasonsBody 应当实现 xpayNoEnvRequest")
	}
}

// 没有 req 形参的那两个函数：appKey 为空时要报**现网**那把的错（本接口没有 env，也就没有
// 沙箱这一说）。
func TestQueryPunishmentReasonsCredentialErrors(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0}`}
	swapXpay(t, rt)

	if _, err := QueryPunishmentReasons(context.Background(), "", "K"); err == nil ||
		!strings.Contains(err.Error(), "accessToken") {
		t.Fatalf("缺 accessToken 应当报错，实际: %v", err)
	}
	_, err := QueryPunishmentReasons(context.Background(), "T", "")
	if err == nil || !strings.Contains(err.Error(), "现网") {
		t.Fatalf("缺 appKey 的文案应当指明是现网那把，实际: %v", err)
	}
	if rt.calls != 0 {
		t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
	}
}

// 本地校验：该拦的拦（且一个字节都不发），该放的放。
func TestComplaintValidation(t *testing.T) {
	ctx := context.Background()

	okList := func() GetComplaintListRequest {
		return GetComplaintListRequest{BeginDate: "2023-01-01", EndDate: "2023-01-31", Limit: 20}
	}

	cases := []struct {
		name    string
		wantErr string // 空串＝应当放行
		call    func() error
	}{
		// ---- 日期：格式（yyyy-mm-dd）与先后。
		{"日期用斜杠", "yyyy-mm-dd", func() error {
			r := okList()
			r.BeginDate = "2023/01/01"
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},
		// time.Parse 对这个 layout 是定宽取的，少位直接 parse 错——这条与下一条一起，
		// 钉住「形状」这件事（checkDay10Range 里没有回写比对，靠的就是 Parse 本身）。
		{"日期少位", "yyyy-mm-dd", func() error {
			r := okList()
			r.BeginDate = "2023-1-1"
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},
		{"日期不是日期", "yyyy-mm-dd", func() error {
			r := okList()
			r.EndDate = "2023-02-30" // 2 月没有 30 号
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},
		{"结束早于开始", "早于 BeginDate", func() error {
			r := okList()
			r.EndDate = "2022-12-31"
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},
		{"同一天（合法）", "", func() error {
			r := okList()
			r.EndDate = r.BeginDate
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},

		// ---- 分页：Offset 从 0 开始合法，Limit 至少 1。
		{"Offset 为负", "Offset -1 非法", func() error {
			r := okList()
			r.Offset = -1
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},
		{"Offset 为 0（合法，就是第一页）", "", func() error {
			r := okList()
			r.Offset = 0
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},
		{"Limit 为 0", "Limit 0 非法", func() error {
			r := okList()
			r.Limit = 0
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},
		{"Limit 为 1（合法边界）", "", func() error {
			r := okList()
			r.Limit = 1
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},

		// ---- 投诉 ID 必填（四个接口各来一条）。
		{"详情：投诉 ID 为空", "ComplaintID 不能为空", func() error {
			_, err := GetComplaintDetail(ctx, "T", "K", GetComplaintDetailRequest{})
			return err
		}},
		{"协商历史：投诉 ID 为空", "ComplaintID 不能为空", func() error {
			_, err := GetNegotiationHistory(ctx, "T", "K", GetNegotiationHistoryRequest{Limit: 10})
			return err
		}},
		{"回复：投诉 ID 为空", "ComplaintID 不能为空", func() error {
			_, err := ResponseComplaint(ctx, "T", "K", ResponseComplaintRequest{ResponseContent: "x"})
			return err
		}},
		{"结单：投诉 ID 为空", "ComplaintID 不能为空", func() error {
			_, err := CompleteComplaint(ctx, "T", "K", CompleteComplaintRequest{})
			return err
		}},

		// ---- 回复：文字与图片至少有一样。
		{"回复：两样都空", "至少要有一个", func() error {
			_, err := ResponseComplaint(ctx, "T", "K", ResponseComplaintRequest{ComplaintID: "C1"})
			return err
		}},
		{"回复：只文字", "", func() error {
			_, err := ResponseComplaint(ctx, "T", "K",
				ResponseComplaintRequest{ComplaintID: "C1", ResponseContent: "已处理"})
			return err
		}},
		{"回复：只图片", "", func() error {
			_, err := ResponseComplaint(ctx, "T", "K",
				ResponseComplaintRequest{ComplaintID: "C1", ResponseImages: []string{"F1"}})
			return err
		}},

		// ---- 上传：图片二选一 + 文件名。
		{"上传：两样都空", "至少要有一个", func() error {
			_, err := UploadVPFile(ctx, "T", "K", UploadVPFileRequest{FileName: "a.png"})
			return err
		}},
		{"上传：文件名为空", "FileName 不能为空", func() error {
			_, err := UploadVPFile(ctx, "T", "K", UploadVPFileRequest{Base64Img: "AAAA"})
			return err
		}},
		{"上传：两样都填（ImgURL 优先，不拦）", "", func() error {
			_, err := UploadVPFile(ctx, "T", "K", UploadVPFileRequest{
				Base64Img: "AAAA", ImgURL: "https://x/a.png", FileName: "a.png"})
			return err
		}},
		{"上传：base64 不是合法 base64（故意不查，交给微信）", "", func() error {
			_, err := UploadVPFile(ctx, "T", "K", UploadVPFileRequest{
				Base64Img: "这不是base64!!!", FileName: "a.png"})
			return err
		}},

		// ---- 取签名头部：地址与投诉 ID。
		{"取签名：地址为空", "WxpayURL 不能为空", func() error {
			_, err := GetUploadFileSign(ctx, "T", "K", GetUploadFileSignRequest{ComplaintID: "C1"})
			return err
		}},
		{"取签名：投诉 ID 为空", "ComplaintID 不能为空", func() error {
			_, err := GetUploadFileSign(ctx, "T", "K",
				GetUploadFileSignRequest{WxpayURL: "https://x/a.png"})
			return err
		}},
		{"取签名：地址不是官方域名（不查前缀，放行）", "", func() error {
			_, err := GetUploadFileSign(ctx, "T", "K", GetUploadFileSignRequest{
				WxpayURL: "https://example.com/whatever.png", ComplaintID: "C1"})
			return err
		}},

		// ---- env 与凭据。
		{"Env 不是 0/1", "Env 2 非法", func() error {
			r := okList()
			r.Env = 2
			_, err := GetComplaintList(ctx, "T", "K", r)
			return err
		}},
		{"缺 accessToken", "accessToken", func() error {
			_, err := GetComplaintList(ctx, "", "K", okList())
			return err
		}},
		{"缺 appKey", "appKey", func() error {
			_, err := GetComplaintList(ctx, "T", "", okList())
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

// 响应解析。重点有两处：详情的 complaint 可能是 nil，以及协商记录里的操作类型要按官方
// 拼错的那两个值来比对。
func TestComplaintResponseParsing(t *testing.T) {
	t.Run("列表：total 与嵌套的订单/服务单/图片", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok","total":1,"complaints":[{` +
			`"complaint_id":"C1","complaint_time":"2026-01-01T10:00:00+08:00",` +
			`"complaint_detail":"买了道具没到账","complaint_state":"PENDING","payer_phone":"13800000000",` +
			`"payer_openid":"o1","complaint_order_info":[{"transaction_id":"TX1","out_trade_no":"CH1",` +
			`"amount":100,"wxa_out_trade_no":"OT1","wx_order_id":"WX1"}],"complaint_full_refunded":false,` +
			`"incoming_user_response":true,"user_complaint_times":1,"complaint_media_list":[` +
			`{"media_type":"USER_COMPLAINT_IMAGE","media_url":["https://api.mch.weixin.qq.com/v3/x/1"]}],` +
			`"problem_description":"道具未到账","problem_type":"REFUND","apply_refund_amount":100,` +
			`"user_tag_list":["TRUSTED"],"service_order_info":[{"order_id":"SO1","out_order_no":"OSO1",` +
			`"state":"DOING"}]}]}`}
		swapXpay(t, rt)

		resp, err := GetComplaintList(context.Background(), "T", "K", GetComplaintListRequest{
			BeginDate: "2023-01-01", EndDate: "2023-01-31", Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Total != 1 || len(resp.Complaints) != 1 {
			t.Fatalf("total/complaints 没解析对: %+v", resp)
		}
		c := resp.Complaints[0]
		if c.ComplaintState != ComplaintStatePending || c.ProblemType != ProblemTypeRefund ||
			!c.IncomingUserResponse || c.ApplyRefundAmount != 100 {
			t.Fatalf("投诉本体没解析对: %+v", c)
		}
		if c.ComplaintOrderInfo[0].Amount != 100 || c.ComplaintOrderInfo[0].WxaOutTradeNo != "OT1" {
			t.Errorf("关联订单没解析对: %+v", c.ComplaintOrderInfo[0])
		}
		if c.UserTagList[0] != UserTagTrusted {
			t.Errorf("用户标签没解析对: %v", c.UserTagList)
		}
		if c.ServiceOrderInfo[0].State != ServiceOrderDoing {
			t.Errorf("服务单状态没解析对: %+v", c.ServiceOrderInfo[0])
		}
		if c.ComplaintMediaList[0].MediaType != ComplaintMediaUserImage ||
			len(c.ComplaintMediaList[0].MediaURL) != 1 {
			t.Errorf("图片资料没解析对: %+v", c.ComplaintMediaList[0])
		}
	})

	t.Run("详情：complaint 为空是 nil，不是错误", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok","complaint":null}`}
		swapXpay(t, rt)

		resp, err := GetComplaintDetail(context.Background(), "T", "K",
			GetComplaintDetailRequest{ComplaintID: "C1"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Complaint != nil {
			t.Fatalf("这一趟就是没有内容，应当是 nil，实际 %+v", resp.Complaint)
		}
		if resp.ErrCode != 0 {
			t.Fatalf("errcode 是 0，实际 %d", resp.ErrCode)
		}
	})

	t.Run("详情：有内容时逐字段可读", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"complaint":{"complaint_id":"C1","complaint_state":"PROCESSED"}}`}
		swapXpay(t, rt)

		resp, err := GetComplaintDetail(context.Background(), "T", "K",
			GetComplaintDetailRequest{ComplaintID: "C1"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Complaint == nil || resp.Complaint.ComplaintID != "C1" ||
			resp.Complaint.ComplaintState != ComplaintStateProcessed {
			t.Fatalf("详情没解析对: %+v", resp.Complaint)
		}
	})

	t.Run("协商历史：两个官方拼错的操作类型要能比对", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"total":2,"history":[` +
			`{"log_id":"L1","operator":"用户","operate_time":"2026-01-01T10:00:00+08:00",` +
			`"operate_type":"USER_COMFIRM_COMPLAINT","operate_details":"已解决",` +
			`"complaint_media_list":[]},` +
			`{"log_id":"L2","operator":"商户","operate_time":"2026-01-01T11:00:00+08:00",` +
			`"operate_type":"MERCHANT_REFUSE_RERUND","operate_details":"不同意退款",` +
			`"complaint_media_list":[]}]}`}
		swapXpay(t, rt)

		resp, err := GetNegotiationHistory(context.Background(), "T", "K",
			GetNegotiationHistoryRequest{ComplaintID: "C1", Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Total != 2 || len(resp.History) != 2 {
			t.Fatalf("total/history 没解析对: %+v", resp)
		}
		// 用常量比对——这正是「按官方拼写抄下来」的用处。
		if resp.History[0].OperateType != OpUserConfirmComplaint {
			t.Errorf("第一条应当是「用户确认投诉解决」，实际 %q", resp.History[0].OperateType)
		}
		if resp.History[1].OperateType != OpMerchantRefuseRefund {
			t.Errorf("第二条应当是「商户拒绝退款」，实际 %q", resp.History[1].OperateType)
		}
		if resp.History[1].OperateDetails != "不同意退款" {
			t.Errorf("留言正文没解析对: %q", resp.History[1].OperateDetails)
		}
	})

	t.Run("上传图片只回一个 file_id", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"file_id":"F1"}`}
		swapXpay(t, rt)

		resp, err := UploadVPFile(context.Background(), "T", "K", UploadVPFileRequest{
			Base64Img: "AAAA", FileName: "a.png"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.FileID != "F1" {
			t.Fatalf("FileID 应当是 F1，实际 %q", resp.FileID)
		}
	})

	t.Run("取签名头部", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"sign":"WECHATPAY2-SHA256-RSA2048 mchid=\"1\",nonce_str=\"n\"","cos_url":"https://cos/x"}`}
		swapXpay(t, rt)

		resp, err := GetUploadFileSign(context.Background(), "T", "K", GetUploadFileSignRequest{
			WxpayURL:   "https://api.mch.weixin.qq.com/v3/merchant-service/images/x",
			ConvertCOS: true, ComplaintID: "C1"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(resp.Sign, "WECHATPAY2-SHA256-RSA2048") || resp.CosURL != "https://cos/x" {
			t.Fatalf("签名头部没解析对: %+v", resp)
		}
	})

	t.Run("管控原因：relate_limitations 按 string 收（官方类型列如此）", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"appid":"wx1","nickname":"小程序","merchant_code":"1900000109",` +
			`"limited_functions":["虚拟支付"],"other_limited_functions":"",` +
			`"recovery_specifications":[{"limitation_case_id":"LC1",` +
			`"limitation_reason_type":"RISK","limitation_reason":"涉嫌违规",` +
			`"limitation_reason_describe":"","relate_limitations":"虚拟支付",` +
			`"other_relate_limitations":"","recover_way":"提交申诉",` +
			`"recover_way_param":"AP1","recover_help_url":"https://x/help",` +
			`"limitation_action_type":"IMMEDIATE","limitation_start_date":"",` +
			`"limitation_date":"2026-01-01"}]}`}
		swapXpay(t, rt)

		resp, err := QueryPunishmentReasons(context.Background(), "T", "K")
		if err != nil {
			t.Fatal(err)
		}
		if resp.AppID != "wx1" || resp.MerchantCode != "1900000109" ||
			len(resp.LimitedFunctions) != 1 {
			t.Fatalf("商户信息没解析对: %+v", resp)
		}
		spec := resp.RecoverySpecifications[0]
		if spec.RelateLimitations != "虚拟支付" || spec.RecoverWay != "提交申诉" ||
			spec.LimitationDate != "2026-01-01" {
			t.Fatalf("管控原因没解析对: %+v", spec)
		}
	})

	t.Run("回复/结单：只有公共头，失败也要读得到", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":268490003,"errmsg":"pay sig error"}`}
		swapXpay(t, rt)

		resp, err := ResponseComplaint(context.Background(), "T", "K",
			ResponseComplaintRequest{ComplaintID: "C1", ResponseContent: "x"})
		if err != nil {
			t.Fatalf("业务失败不该变成 error: %v", err)
		}
		if resp == nil || resp.ErrCode != 268490003 || resp.ErrMsg != "pay sig error" {
			t.Fatalf("errcode/errmsg 必须是原值，实际 %+v", resp)
		}

		done, err := CompleteComplaint(context.Background(), "T", "K",
			CompleteComplaintRequest{ComplaintID: "C1"})
		if err != nil {
			t.Fatal(err)
		}
		if done.ErrCode != 268490003 {
			t.Fatalf("结单那条也应原值带出 errcode，实际 %d", done.ErrCode)
		}
	})
}
