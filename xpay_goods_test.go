package wechat_virtualpay_go

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// 道具类的字段表，照 TestOrderFieldsCoverDoc 那一套核：名字、类型、**行序**三项都要与
// 官方字段表一致，且不多不少。
//
// 两个 start_* 的响应只有公共头，所以那两张表就只有 errcode/errmsg 两行——「响应没有
// 自己的字段」这件事在**类型层面**也被钉住：谁要是给它们加了字段，这张表会红。
func TestGoodsFieldsCoverDoc(t *testing.T) {
	tStr := reflect.TypeOf("")
	tI64 := reflect.TypeOf(int64(0))
	tInt := reflect.TypeOf(int(0))
	tItemStatus := reflect.TypeOf(GoodsItemStatus(0))
	tBatchStatus := reflect.TypeOf(GoodsBatchStatus(0))
	tUploadItemList := reflect.TypeOf([]UploadGoodsItem(nil))
	tUploadedList := reflect.TypeOf([]UploadedGoodsItem(nil))
	tPublishItemList := reflect.TypeOf([]PublishGoodsItem(nil))
	tPublishedList := reflect.TypeOf([]PublishedGoodsItem(nil))

	cases := []struct {
		what string
		val  any
		doc  []fieldSpec
	}{
		{"UploadGoodsItem", UploadGoodsItem{}, []fieldSpec{
			{"id", tStr}, {"name", tStr}, {"price", tI64}, {"remark", tStr}, {"item_url", tStr},
		}},
		{"StartUploadGoodsRequest", StartUploadGoodsRequest{}, []fieldSpec{
			{"upload_item", tUploadItemList}, {"env", tInt},
		}},
		// 只有公共头。这两行不是「抄漏了别的东西」，是官方响应体里确实只有它们。
		{"StartUploadGoodsResponse", StartUploadGoodsResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
		}},
		{"UploadedGoodsItem", UploadedGoodsItem{}, []fieldSpec{
			{"id", tStr}, {"name", tStr}, {"price", tI64}, {"remark", tStr}, {"item_url", tStr},
			{"upload_status", tItemStatus}, {"errmsg", tStr},
		}},
		{"QueryUploadGoodsRequest", QueryUploadGoodsRequest{}, []fieldSpec{
			{"env", tInt},
		}},
		{"QueryUploadGoodsResponse", QueryUploadGoodsResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
			{"upload_item", tUploadedList}, {"status", tBatchStatus},
		}},
		{"PublishGoodsItem", PublishGoodsItem{}, []fieldSpec{
			{"id", tStr},
		}},
		{"StartPublishGoodsRequest", StartPublishGoodsRequest{}, []fieldSpec{
			{"publish_item", tPublishItemList}, {"env", tInt},
		}},
		{"StartPublishGoodsResponse", StartPublishGoodsResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
		}},
		{"PublishedGoodsItem", PublishedGoodsItem{}, []fieldSpec{
			{"id", tStr}, {"publish_status", tItemStatus}, {"errmsg", tStr},
		}},
		{"QueryPublishGoodsRequest", QueryPublishGoodsRequest{}, []fieldSpec{
			{"env", tInt},
		}},
		{"QueryPublishGoodsResponse", QueryPublishGoodsResponse{}, []fieldSpec{
			{"errcode", tInt}, {"errmsg", tStr},
			{"publish_item", tPublishedList}, {"status", tBatchStatus},
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
	// 5 + 2 + 2 + 7 + 1 + 4 + 1 + 2 + 2 + 3 + 1 + 4。
	if total != 34 {
		t.Fatalf("上面 %d 个结构体的文档字段总数应为 34，实际 %d（期望集合可能抄漏）", len(cases), total)
	}
}

// 两套状态枚举的取值：抄错数字不会报错，只会把「任务已成」读成「还在跑」。
func TestGoodsEnumValues(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"GoodsBatchNone", int(GoodsBatchNone), 0},
		{"GoodsBatchRunning", int(GoodsBatchRunning), 1},
		{"GoodsBatchPartialFail", int(GoodsBatchPartialFail), 2},
		{"GoodsBatchSuccess", int(GoodsBatchSuccess), 3},

		{"GoodsItemPending", int(GoodsItemPending), 0},
		{"GoodsItemExists", int(GoodsItemExists), 1},
		{"GoodsItemOK", int(GoodsItemOK), 2},
		{"GoodsItemFailed", int(GoodsItemFailed), 3},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d，文档是 %d", c.name, c.got, c.want)
		}
	}
}

// env 在这四个请求结构体上都是裸 int。
func TestGoodsRequestsExposeEnv(t *testing.T) {
	want := reflect.TypeOf(int(0))
	requests := map[string]any{
		"StartUploadGoodsRequest":  StartUploadGoodsRequest{},
		"QueryUploadGoodsRequest":  QueryUploadGoodsRequest{},
		"StartPublishGoodsRequest": StartPublishGoodsRequest{},
		"QueryPublishGoodsRequest": QueryPublishGoodsRequest{},
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

// 四个接口发出去的请求体**逐字节**对：键序即字段声明顺序，pay_sig 盖住同一份字节。
//
// 上传那个刻意把道具的五个字段都填上并逐个核对：UploadGoodsItem 的字段**没有
// omitempty**，空值也要发出去（官方字段表里它们都在，空的也照样是一行）。
func TestGoodsRequestBodies(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		call func(ctx context.Context) error
		want string
	}{
		{
			"start_upload_goods",
			"/xpay/start_upload_goods",
			func(ctx context.Context) error {
				_, err := StartUploadGoods(ctx, "T", "K", StartUploadGoodsRequest{
					UploadItem: []UploadGoodsItem{{ID: "A", Name: "n", Price: 100, Remark: "r", ItemURL: "u"}},
				})
				return err
			},
			`{"upload_item":[{"id":"A","name":"n","price":100,"remark":"r","item_url":"u"}],"env":0}`,
		},
		{
			"query_upload_goods",
			"/xpay/query_upload_goods",
			func(ctx context.Context) error {
				_, err := QueryUploadGoods(ctx, "T", "K", QueryUploadGoodsRequest{})
				return err
			},
			`{"env":0}`,
		},
		{
			"start_publish_goods",
			"/xpay/start_publish_goods",
			func(ctx context.Context) error {
				_, err := StartPublishGoods(ctx, "T", "K", StartPublishGoodsRequest{
					PublishItem: []PublishGoodsItem{{ID: "A"}},
				})
				return err
			},
			`{"publish_item":[{"id":"A"}],"env":0}`,
		},
		{
			"query_publish_goods",
			"/xpay/query_publish_goods",
			func(ctx context.Context) error {
				_, err := QueryPublishGoods(ctx, "T", "K", QueryPublishGoodsRequest{})
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
			if got, want := rt.query.Get("pay_sig"), testPaySig("K", c.uri, string(rt.body)); got != want {
				t.Errorf("pay_sig 与发出去的请求体对不上\n实际: %s\n期望: %s", got, want)
			}
			if got := rt.query.Get("signature"); got != "" {
				t.Errorf("这一档不该有 signature，实际 %s", got)
			}
		})
	}
}

// 「一次仅支持一个道具」是官方写在两个 start_* 页上的限制，本包在本地拦：0 个是空提交，
// 2 个官方只保证其中一个（文档没说是哪个）——两种都不发出去。
func TestGoodsStartRejectsNotExactlyOneItem(t *testing.T) {
	one := func() UploadGoodsItem { return UploadGoodsItem{ID: "A", Name: "n", Price: 1} }

	cases := []struct {
		name     string
		call     func(ctx context.Context) error
		wantPass bool
	}{
		{"上传：0 个道具", func(ctx context.Context) error {
			_, err := StartUploadGoods(ctx, "T", "K", StartUploadGoodsRequest{})
			return err
		}, false},
		{"上传：2 个道具", func(ctx context.Context) error {
			_, err := StartUploadGoods(ctx, "T", "K", StartUploadGoodsRequest{
				UploadItem: []UploadGoodsItem{one(), one()},
			})
			return err
		}, false},
		{"上传：正好 1 个", func(ctx context.Context) error {
			_, err := StartUploadGoods(ctx, "T", "K", StartUploadGoodsRequest{UploadItem: []UploadGoodsItem{one()}})
			return err
		}, true},
		{"发布：0 个道具", func(ctx context.Context) error {
			_, err := StartPublishGoods(ctx, "T", "K", StartPublishGoodsRequest{})
			return err
		}, false},
		{"发布：2 个道具", func(ctx context.Context) error {
			_, err := StartPublishGoods(ctx, "T", "K", StartPublishGoodsRequest{
				PublishItem: []PublishGoodsItem{{ID: "A"}, {ID: "B"}},
			})
			return err
		}, false},
		{"发布：正好 1 个", func(ctx context.Context) error {
			_, err := StartPublishGoods(ctx, "T", "K", StartPublishGoodsRequest{
				PublishItem: []PublishGoodsItem{{ID: "A"}},
			})
			return err
		}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			err := c.call(context.Background())
			if c.wantPass && err != nil {
				t.Fatalf("应当放行，实际: %v", err)
			}
			if !c.wantPass {
				if err == nil {
					t.Fatal("应当被拦下来")
				}
				if !strings.Contains(err.Error(), "恰好一个") {
					t.Errorf("文案里应当说清是「恰好一个」这条规则，实际: %v", err)
				}
				if rt.calls != 0 {
					t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
				}
			}
		})
	}
}

// 道具自己的两处必填/取值范围，都在本地拦（Price 那条是官方明写的「需大于 0」）。
//
// 顺带钉住**故意不查**的那几条：Name/Remark/ItemURL 空着、ID 写中文，都要能过——官方在
// 长度与字符集上自相矛盾（见文件头），本包不发明规则，谁日后「顺手」加上格式校验，这里会红。
func TestUploadGoodsItemValidation(t *testing.T) {
	ok := UploadGoodsItem{ID: "A", Name: "n", Price: 1, Remark: "r", ItemURL: "u"}

	cases := []struct {
		name     string
		mut      func(*UploadGoodsItem)
		wantPass bool
	}{
		{"ID 为空", func(i *UploadGoodsItem) { i.ID = "" }, false},
		{"Price 为 0", func(i *UploadGoodsItem) { i.Price = 0 }, false},
		{"Price 为负", func(i *UploadGoodsItem) { i.Price = -1 }, false},
		{"Price 为 1 分", func(i *UploadGoodsItem) { i.Price = 1 }, true},
		// 以下都是「不查」的那几条，必须放行。
		{"Name 为空（不查）", func(i *UploadGoodsItem) { i.Name = "" }, true},
		{"Remark 为空（不查）", func(i *UploadGoodsItem) { i.Remark = "" }, true},
		{"ItemURL 为空（不查）", func(i *UploadGoodsItem) { i.ItemURL = "" }, true},
		{"ID 写中文（文档自相矛盾，不查）", func(i *UploadGoodsItem) { i.ID = "道具一" }, true},
		{"ID 超 20 字符（长度不查）", func(i *UploadGoodsItem) { i.ID = strings.Repeat("a", 40) }, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &xpayRT{resp: `{"errcode":0}`}
			swapXpay(t, rt)

			item := ok
			c.mut(&item)
			_, err := StartUploadGoods(context.Background(), "T", "K",
				StartUploadGoodsRequest{UploadItem: []UploadGoodsItem{item}})
			if c.wantPass && err != nil {
				t.Fatalf("应当放行，实际: %v", err)
			}
			if !c.wantPass {
				if err == nil {
					t.Fatal("应当被拦下来")
				}
				if rt.calls != 0 {
					t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
				}
			}
		})
	}
}

// 发布那一路的道具必填：列表「恰好一个」之后，那一个自己还得有 ID。
//
// 这条与上传那一路的 UploadGoodsItem.validate 是**两处**判断，所以也得分两处钉：上面那张
// 表只覆盖了上传，漏掉的话，发布这一路的 ID 校验删掉也没人吭声。
func TestPublishGoodsItemRequiresID(t *testing.T) {
	rt := &xpayRT{resp: `{"errcode":0}`}
	swapXpay(t, rt)

	_, err := StartPublishGoods(context.Background(), "T", "K",
		StartPublishGoodsRequest{PublishItem: []PublishGoodsItem{{}}})
	if err == nil {
		t.Fatal("道具 ID 为空应当被拦下来")
	}
	if !strings.Contains(err.Error(), "道具 ID") {
		t.Errorf("文案里应当指明是道具 ID 的问题，实际: %v", err)
	}
	if rt.calls != 0 {
		t.Fatalf("参数不合法却发出了 %d 次请求", rt.calls)
	}
}

// 响应解析：批量任务的**部分失败**是重点——Status 说「结束了且有失败」，具体哪个道具
// 没成要看每条自己的 upload_status/errmsg。
//
// 另外钉住两个 start_* 的公共头：它们的响应结构体只有内嵌的 ResponseHeader，失败时
// errcode/errmsg 照样要能读出来（这正是给它们留一个响应类型的原因）。
func TestGoodsResponseParsing(t *testing.T) {
	t.Run("上传任务部分失败", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok","status":2,"upload_item":[` +
			`{"id":"A","name":"甲","price":100,"remark":"","item_url":"http://x/a.jpg","upload_status":2},` +
			`{"id":"B","name":"乙","price":200,"remark":"","item_url":"http://x/b.jpg","upload_status":3,"errmsg":"图片下载失败"}` +
			`]}`}
		swapXpay(t, rt)

		resp, err := QueryUploadGoods(context.Background(), "T", "K", QueryUploadGoodsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != GoodsBatchPartialFail {
			t.Errorf("status 应当是 %d（已结束且有失败），实际 %d", GoodsBatchPartialFail, resp.Status)
		}
		if len(resp.UploadItem) != 2 {
			t.Fatalf("应当有两个道具，实际 %d", len(resp.UploadItem))
		}
		if resp.UploadItem[0].UploadStatus != GoodsItemOK {
			t.Errorf("第一个道具应当上传成功，实际 %d", resp.UploadItem[0].UploadStatus)
		}
		if resp.UploadItem[1].UploadStatus != GoodsItemFailed || resp.UploadItem[1].ErrMsg != "图片下载失败" {
			t.Errorf("第二个道具应当带着失败原因，实际 %+v", resp.UploadItem[1])
		}
		// 字段名与公共头的 errmsg 同名，但这一层是「道具自己」的失败原因——两处都要在。
		if resp.ErrMsg != "ok" {
			t.Errorf("公共头的 errmsg 不该被道具的 errmsg 顶掉，实际 %q", resp.ErrMsg)
		}
	})

	t.Run("发布任务成功", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"status":3,"publish_item":[{"id":"A","publish_status":2}]}`}
		swapXpay(t, rt)

		resp, err := QueryPublishGoods(context.Background(), "T", "K", QueryPublishGoodsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != GoodsBatchSuccess || resp.PublishItem[0].PublishStatus != GoodsItemOK {
			t.Fatalf("字段没填对: %+v", resp)
		}
	})

	t.Run("start_upload_goods 只有公共头，失败也要读得到", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":268490003,"errmsg":"pay sig error"}`}
		swapXpay(t, rt)

		resp, err := StartUploadGoods(context.Background(), "T", "K", StartUploadGoodsRequest{
			UploadItem: []UploadGoodsItem{{ID: "A", Price: 1}},
		})
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

	t.Run("start_publish_goods 同样", func(t *testing.T) {
		rt := &xpayRT{resp: `{"errcode":0,"errmsg":"ok"}`}
		swapXpay(t, rt)

		resp, err := StartPublishGoods(context.Background(), "T", "K", StartPublishGoodsRequest{
			PublishItem: []PublishGoodsItem{{ID: "A"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if resp.ErrCode != 0 || resp.ErrMsg != "ok" {
			t.Fatalf("公共头没解析对: %+v", resp)
		}
	})
}

// 凭据与 env 的校验：本地拦，一个字节不发。
func TestGoodsEndpointsRejectBadInputWithoutRequest(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		want string
		call func() error
	}{
		{"缺 accessToken", "accessToken", func() error {
			_, err := QueryUploadGoods(ctx, "", "K", QueryUploadGoodsRequest{})
			return err
		}},
		{"缺 appKey", "appKey", func() error {
			_, err := QueryPublishGoods(ctx, "T", "", QueryPublishGoodsRequest{})
			return err
		}},
		{"env 取值非法", "Env 2 非法", func() error {
			_, err := QueryUploadGoods(ctx, "T", "K", QueryUploadGoodsRequest{Env: 2})
			return err
		}},
		{"沙箱下缺 appKey，报错要指明该配沙箱 key", "沙箱 AppKey", func() error {
			_, err := QueryUploadGoods(ctx, "T", "", QueryUploadGoodsRequest{Env: 1})
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
