package wechat_virtualpay_go

import (
	"context"
	"fmt"
)

// 本文件是官方 /xpay/* 里「道具」这一类 4 个接口：
//
//	POST /xpay/start_upload_goods   启动批量上传道具任务  access_token + pay_sig
//	POST /xpay/query_upload_goods   查询上传任务结果      access_token + pay_sig
//	POST /xpay/start_publish_goods  启动批量发布道具任务  access_token + pay_sig
//	POST /xpay/query_publish_goods  查询发布任务结果      access_token + pay_sig
//
// 四个都是 pay_sig 档。上传与发布各是一对「start 触发 + query 轮询」的**异步**任务：
//
//	start_upload_goods  →  query_upload_goods   把道具传进开发环境
//	start_publish_goods →  query_publish_goods  把开发环境的道具发布到现网
//
// ⚠️ 这一对任务**没有 task_id**：两个 query 都不收参数（请求体里只有 env），查的就是本
// 商家那一个批量任务。所以「查哪个任务」不是调用方要操心的事，也因此**同时只能跑一个**
// ——上一个没查完就再 start 一次，行为没有定义。
//
// ⚠️ 一次只能提交一个道具：官方两页都写着「一次仅支持上传/发布一个道具，多个道具需分
// 多次请求」，所以两个 start 的请求体在本地就要求列表**恰好一个**（见 checkOneGoodsItem）。
//
// ⚠️ **发布之后约 10 分钟才生效**，生效前用户下单会失败。客户端侧的错误码是 -15014，
// **服务端错误码表里没有列出对应项**——那个码是小程序端返回给用户的，不是本包这几个接口
// 会回的东西，别在 resp.ErrCode 上等它。
//
// start_* 的响应只有公共头（与订单类的 NotifyProvideGoods 同一种），但仍然各有一个响应
// 结构体：**没有自己的字段不等于不会失败**，errcode 得有地方读（理由见
// NotifyProvideGoodsResponse 的注释）。这两个 start_* 服务端接口的响应体确实没有任何
// 自己的字段，所以那两个结构体只内嵌公共头。
//
// 其余约定与订单类**逐字同义**，不在这里重抄（见 xpay_order.go 文件头）：凭据显式传参、
// env 是请求结构体上的一个裸 int（0=现网 / 1=沙箱）、字段顺序照官方字段表逐行抄、
// 响应**原值返回**、`err == nil` 不等于成功（成败看 resp.ErrCode）、失败时响应为 nil。
//
// 本类**故意不做**的本地校验：
//
//   - **不查 ID / Name / Remark 的长度**：官方给了长度区间（(0,20] / (0,20] / (0,1024]），
//     但计长规则只在一处写了半句「中文算一个字符」，而那一处的上下文字符集又只列了字母
//     数字——按字节数还是按字符数、中文到底收不收，都没有可依据的定论。本包不发明规则：
//     长度写超了微信会回参数错误，那比本地按一个猜来的规则把合法输入挡住强。
//
// 文件按接口分段，每段是「请求结构体 → 本地校验 → 响应结构体 → 调用函数」，
// 读一个接口只需要看一段。

// GoodsBatchStatus 是道具「上传/发布」**批量任务**的状态（query_upload_goods 与
// query_publish_goods 响应里的 status）。
//
// ⚠️ 与 GoodsItemStatus 区分：本类型描述的是**整批任务**，GoodsItemStatus 描述的是
// **单个道具**。
//
// ⚠️ 两个容易看反的地方：GoodsBatchPartialFail(2) 的含义是「**已经结束了**，且有道具没成功」
// ——它不是「还在跑」，要接着看每个道具的 ErrMsg；而零值 GoodsBatchNone(0) 是「无任务在
// 运行」，不是「成功」。判断任务跑完没有，认 GoodsBatchPartialFail 与 GoodsBatchSuccess
// 这两个终态。
type GoodsBatchStatus int

const (
	GoodsBatchNone        GoodsBatchStatus = 0 // 无任务在运行
	GoodsBatchRunning     GoodsBatchStatus = 1 // 任务运行中
	GoodsBatchPartialFail GoodsBatchStatus = 2 // 上传/发布失败或部分失败（任务已完成）
	GoodsBatchSuccess     GoodsBatchStatus = 3 // 上传/发布成功
)

// GoodsItemStatus 是单个道具的上传/发布状态。
//
// 上传与发布共用同一套取值（upload_status / publish_status）。
//
// ⚠️ 零值是 GoodsItemPending(0)。微信没有单独的「未知」取值，所以响应里没回这个字段时，
// 读到的就是「上传中/发布中」——那不是默认值，是这个枚举真有一个 0。要判断某个道具成没成，
// 认 GoodsItemOK(2) / GoodsItemFailed(3)，别拿零值当「还没轮到」。
type GoodsItemStatus int

const (
	GoodsItemPending GoodsItemStatus = 0 // 上传中/发布中
	GoodsItemExists  GoodsItemStatus = 1 // id 已经存在
	GoodsItemOK      GoodsItemStatus = 2 // 上传/发布成功
	GoodsItemFailed  GoodsItemStatus = 3 // 上传/发布失败
)

// checkOneGoodsItem 校验「一次只提交一个道具」。
//
// 官方两个 start_* 页都写明「一次仅支持上传/发布一个道具，多个道具需分多次请求」，所以
// 这里要求**恰好一个**：0 个是空提交，多于 1 个官方只处理/只保证其中一个（文档没说是哪个），
// 两种都拦。这条是**文档写着的**，不是推出来的——与代币类那条「金额必须大于 0」不同。
func checkOneGoodsItem(n int, what string) error {
	if n != 1 {
		return fmt.Errorf("wechat_virtualpay_go: %s 必须恰好一个道具，当前 %d 个（官方：一次仅支持一个，多个需分多次请求）", what, n)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 1/4  start_upload_goods —— 启动批量上传道具任务
//
//	POST /xpay/start_upload_goods  access_token + pay_sig
// ---------------------------------------------------------------------------

// UploadGoodsItem 是要上传的道具。
type UploadGoodsItem struct {
	// ID 道具 ID，长度 (0,20]，只允许字母、数字、'_'、'-'（中文算一个字符）。
	//
	// ⚠️ 官方这句自相矛盾：括号前只列了字母数字下划线横线，括号里却说「中文算一个字符」
	// ——像是从别处的说明里抄来的（Name 那栏才像收中文）。本包因此**不查**它的字符集与
	// 长度，只查非空（见文件头的「故意不做」）。
	ID string `json:"id"`
	// Name 道具名称，长度 (0,20]。
	Name string `json:"name"`
	// Price 道具单价，**单位分**，需大于 0。
	//
	// 官方明写「需大于 0」（不像代币那几页对 amount 一个字不提），所以本包在本地拦：
	// 0 或负数的道具价格没有含义。
	Price int64 `json:"price"`
	// Remark 道具备注，长度 (0,1024]。
	Remark string `json:"remark"`
	// ItemURL 道具图片 URL，当前仅支持 jpg、png 等格式。
	ItemURL string `json:"item_url"`
}

func (i UploadGoodsItem) validate() error {
	if i.ID == "" {
		return fmt.Errorf("wechat_virtualpay_go: 道具 ID 不能为空")
	}
	if i.Price <= 0 {
		return fmt.Errorf("wechat_virtualpay_go: 道具 Price 必须大于 0（单位分，官方明写「需大于 0」）")
	}
	// Name / Remark / ItemURL 不查：官方没标它们必填，长度与格式也没有可依据的规则。
	return nil
}

// StartUploadGoodsRequest 是启动批量上传道具任务的请求体。
type StartUploadGoodsRequest struct {
	// UploadItem 上传的道具列表。**必须恰好一个**（官方：一次仅支持上传一个道具，
	// 多个道具需分多次请求），本包在本地拦。
	UploadItem []UploadGoodsItem `json:"upload_item"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

func (r StartUploadGoodsRequest) validate() error {
	if err := checkOneGoodsItem(len(r.UploadItem), "UploadItem"); err != nil {
		return err
	}
	return r.UploadItem[0].validate()
}

// StartUploadGoodsResponse 是启动批量上传道具任务的响应体。
//
// 它没有自己的字段：微信只回公共头。仍然留着这个类型，因为**没有自己的字段不等于不会
// 失败**——调用方要判断成败，得有一个地方能读到 errcode。
type StartUploadGoodsResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
}

// StartUploadGoods 启动批量上传道具任务。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证。自研小程序传 GetStableAccessToken 换来的 access_token，第三方
//	             平台代商家调用传 authorizer_access_token——两者在这里是同一种东西。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          一个道具（**恰好一个**）与 Env。
//
// 任务是**异步**的：本方法只表示「任务已受理」，要接着调 QueryUploadGoods 看结果。
//
// 官方文档：POST /xpay/start_upload_goods
func StartUploadGoods(ctx context.Context, accessToken, appKey string, req StartUploadGoodsRequest) (*StartUploadGoodsResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp StartUploadGoodsResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/start_upload_goods", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 2/4  query_upload_goods —— 查询批量上传道具任务的结果
//
//	POST /xpay/query_upload_goods  access_token + pay_sig
// ---------------------------------------------------------------------------

// UploadedGoodsItem 是查询上传任务结果里的单个道具。
type UploadedGoodsItem struct {
	// ID 道具 ID。
	ID string `json:"id"`
	// Name 道具名称。
	Name string `json:"name"`
	// Price 道具单价，**单位分**。
	Price int64 `json:"price"`
	// Remark 道具备注。
	Remark string `json:"remark"`
	// ItemURL 道具图片 URL（微信转存后的地址）。
	ItemURL string `json:"item_url"`
	// UploadStatus 该道具的上传状态。取值见 GoodsItemStatus。
	//
	// ⚠️ 零值是 GoodsItemPending(0)——要判断上传成没成，认 GoodsItemOK/GoodsItemFailed。
	UploadStatus GoodsItemStatus `json:"upload_status"`
	// ErrMsg 上传失败的原因。字段名与公共头的 errmsg 同名（都是 errmsg），但这一层是
	// **道具自己**的失败原因——整体成功后仍可能有道具失败（见 GoodsBatchPartialFail）。
	ErrMsg string `json:"errmsg"`
}

// QueryUploadGoodsRequest 是查询批量上传道具任务的请求体。
type QueryUploadGoodsRequest struct {
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

// validate 没有可查的字段：本请求体只有 env，而 env 由 checkEnv 查（0/1 之外都在本地
// 拦下）。留着这个方法是为了让本类四个接口的调用形状一样。
//
// ⚠️ 也正因为**没有 task_id 可传**，本接口查的就是本商家那一个批量上传任务——别指望用
// 它查历史任务。
func (r QueryUploadGoodsRequest) validate() error { return nil }

// QueryUploadGoodsResponse 是查询批量上传道具任务的响应体。
type QueryUploadGoodsResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// UploadItem 上传的道具列表。
	UploadItem []UploadedGoodsItem `json:"upload_item"`
	// Status 整体任务状态。取值见 GoodsBatchStatus。
	//
	// ⚠️ 零值是 GoodsBatchNone(0)「无任务在运行」，不是「成功」；GoodsBatchPartialFail(2)
	// 是**已结束**但有道具没成功。认 GoodsBatchSuccess/GoodsBatchPartialFail 两个终态，
	// 并且**只看 Status 是不够的**：部分失败时哪些道具失败了，要看 UploadItem 里各自的
	// UploadStatus 与 ErrMsg。
	Status GoodsBatchStatus `json:"status"`
}

// QueryUploadGoods 查询批量上传道具任务的结果。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 StartUploadGoods。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          只有 Env——没有 task_id，查的就是本商家那一个批量上传任务。
//
// 轮询到 Status 变成终态（GoodsBatchSuccess / GoodsBatchPartialFail）为止；到了
// GoodsBatchPartialFail 还要逐个道具看 ErrMsg 才知道是哪个没成。
//
// 官方文档：POST /xpay/query_upload_goods
func QueryUploadGoods(ctx context.Context, accessToken, appKey string, req QueryUploadGoodsRequest) (*QueryUploadGoodsResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp QueryUploadGoodsResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/query_upload_goods", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 3/4  start_publish_goods —— 启动批量发布道具任务
//
//	POST /xpay/start_publish_goods  access_token + pay_sig
// ---------------------------------------------------------------------------

// PublishGoodsItem 是要发布的道具。
type PublishGoodsItem struct {
	// ID 道具 ID，即添加到开发环境时传的道具 ID（UploadGoodsItem.ID 用的那个）。
	ID string `json:"id"`
}

func (i PublishGoodsItem) validate() error {
	if i.ID == "" {
		return fmt.Errorf("wechat_virtualpay_go: 道具 ID 不能为空")
	}
	return nil
}

// StartPublishGoodsRequest 是启动批量发布道具任务的请求体。
type StartPublishGoodsRequest struct {
	// PublishItem 发布的道具列表。**必须恰好一个**（官方：一次仅支持发布一个道具，
	// 多个道具需分多次请求），本包在本地拦。
	PublishItem []PublishGoodsItem `json:"publish_item"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

func (r StartPublishGoodsRequest) validate() error {
	if err := checkOneGoodsItem(len(r.PublishItem), "PublishItem"); err != nil {
		return err
	}
	return r.PublishItem[0].validate()
}

// StartPublishGoodsResponse 是启动批量发布道具任务的响应体。
//
// 同 StartUploadGoodsResponse：只有公共头，但仍要有这个类型来读 errcode。
type StartPublishGoodsResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
}

// StartPublishGoods 启动批量发布道具任务（把开发环境的道具发布到现网）。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 StartUploadGoods。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          一个道具的 ID（**恰好一个**）与 Env。
//
// 任务是**异步**的：本方法只表示「任务已受理」，要接着调 QueryPublishGoods 看结果。
//
// ⚠️ 发布之后约 **10 分钟**才生效，生效前用户下单会失败（客户端侧的错误码是 -15014；
// 服务端错误码表里没有列出对应项）。所以「发布成功」不等于「马上能卖」。
//
// 官方文档：POST /xpay/start_publish_goods
func StartPublishGoods(ctx context.Context, accessToken, appKey string, req StartPublishGoodsRequest) (*StartPublishGoodsResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp StartPublishGoodsResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/start_publish_goods", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 4/4  query_publish_goods —— 查询批量发布道具任务的结果
//
//	POST /xpay/query_publish_goods  access_token + pay_sig
// ---------------------------------------------------------------------------

// PublishedGoodsItem 是查询发布任务结果里的单个道具。
type PublishedGoodsItem struct {
	// ID 道具 ID。
	ID string `json:"id"`
	// PublishStatus 该道具的发布状态。取值见 GoodsItemStatus。
	//
	// ⚠️ 零值是 GoodsItemPending(0)——判断发布成没成，认 GoodsItemOK/GoodsItemFailed。
	PublishStatus GoodsItemStatus `json:"publish_status"`
	// ErrMsg 发布失败的原因。与 UploadedGoodsItem.ErrMsg 同名（都叫 errmsg），含义都是
	// 「这一条道具为什么没成」。
	ErrMsg string `json:"errmsg"`
}

// QueryPublishGoodsRequest 是查询批量发布道具任务的请求体。
type QueryPublishGoodsRequest struct {
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

// validate 没有可查的字段：同 QueryUploadGoodsRequest（也**没有 task_id**，查的就是本
// 商家那一个批量发布任务）。
func (r QueryPublishGoodsRequest) validate() error { return nil }

// QueryPublishGoodsResponse 是查询批量发布道具任务的响应体。
type QueryPublishGoodsResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// PublishItem 发布的道具列表。
	PublishItem []PublishedGoodsItem `json:"publish_item"`
	// Status 整体任务状态。取值见 GoodsBatchStatus——两个坑同 QueryUploadGoodsResponse
	// （零值是「无任务在运行」；GoodsBatchPartialFail 是已结束但有道具没成功）。
	Status GoodsBatchStatus `json:"status"`
}

// QueryPublishGoods 查询批量发布道具任务的结果。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 StartUploadGoods。
//	appKey       商家密钥，用来算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	req          只有 Env——没有 task_id，查的就是本商家那一个批量发布任务。
//
// 与上传不同的是：**任务状态到终态也不等于道具已经能卖了**，发布还有约 10 分钟的生效
// 延迟（见 StartPublishGoods）。
//
// 官方文档：POST /xpay/query_publish_goods
func QueryPublishGoods(ctx context.Context, accessToken, appKey string, req QueryPublishGoodsRequest) (*QueryPublishGoodsResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp QueryPublishGoodsResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/query_publish_goods", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
