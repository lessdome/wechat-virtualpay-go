package wechat_virtualpay_go

import (
	"context"
	"fmt"
	"time"
)

// 本文件是官方 /xpay/* 里「账单」这一类 2 个接口：
//
//	POST /xpay/download_bill                 下载虚拟支付日账单    access_token + pay_sig
//	POST /xpay/download_ios_settlement_bill  下载苹果 IAP 月账单   access_token + pay_sig
//
// 两个都是**轮询式**的：调用只负责触发生成，返回的链接可能还没生成（见各自的注释），
// 隔一会儿再调一次即可。
//
// ⚠️ 本类是全包**唯一**请求体里没有 env 字段的一类：这两个接口的官方字段表里根本没有
// env 这一行。所以本类的请求结构体上没有 Env 字段，也不走 xpay.go 的兜底补 env ——它们
// 实现了 xpayNoEnvRequest，标的就是这件事。别顺手给它们补一个 Env 字段「与别处保持一致」：
// 那会让发出去的请求体多出一个文档里没有的字段，而签的正是这份多出来的字节。
//
// 没有 env 可传，也就没有「现网/沙箱」这层选择：用哪套环境的 AppKey，就查那套环境的
// 账。这是本类与其它类在凭据上的唯一差别，所以本类只校验 appKey 非空
// （checkAppKeyNoEnv），不像别处那样还要说清「env=N 该配哪把」。
//
// 其余约定与订单类**逐字同义**，不在这里重抄（见 xpay_order.go 文件头）：凭据显式传参、
// 字段顺序照官方字段表逐行抄、响应**原值返回**、`err == nil` 不等于成功（成败看
// resp.ErrCode）、失败时响应为 nil——`resp == nil` 与 `resp.ErrCode != 0` 是两件事。
//
// 本类**故意不做**的本地校验：
//
//   - **不查时间跨度上限**：两个接口的官方页都没给「最多查多少天/多少个月」。订单类的
//     start_download_order 那个 31 天上限是那一页自己写的，不适用于这里，别顺手挪过来。
//
// 文件按接口分段，每段是「请求结构体 → 本地校验 → 响应结构体 → 调用函数」，
// 读一个接口只需要看一段。

// checkMonthRange 校验一对 YYYYMM 月份（如 202601 到 202603）：两个都得是合法月份，且
// 结束月不早于开始月。
//
// 与日期那一对（xpay_common.go 的 checkDayRange）同理，走 time.Parse 而不是自己数位数：
// 20261、202613 这种「位数不对或月份不存在」的都被拦下来。它留在本文件而不是
// xpay_common.go，因为只有本类的接口用 YYYYMM。
func checkMonthRange(start, end string) error {
	s, err := time.Parse("200601", start)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: StartMonth %q 不是合法的 YYYYMM 月份", start)
	}
	e, err := time.Parse("200601", end)
	if err != nil {
		return fmt.Errorf("wechat_virtualpay_go: EndMonth %q 不是合法的 YYYYMM 月份", end)
	}
	if e.Before(s) {
		return fmt.Errorf("wechat_virtualpay_go: EndMonth(%s) 早于 StartMonth(%s)", end, start)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 1/2  download_bill —— 下载虚拟支付日账单
//
//	POST /xpay/download_bill  access_token + pay_sig
// ---------------------------------------------------------------------------

// DownloadBillRequest 是下载虚拟支付日账单的请求体。
//
// ⚠️ 它**没有 env 字段**，而且不是漏了：官方字段表里就没有这一行（理由见文件头）。
type DownloadBillRequest struct {
	// BeginDs 起始时间，格式 YYYYMMDD，如 20230801。
	BeginDs int64 `json:"begin_ds"`
	// EndDs 截止时间，格式 YYYYMMDD，如 20230810。
	EndDs int64 `json:"end_ds"`
}

// xpayNoEnv 把「本请求体不带 env」这件事告诉 xpay.go 的 requestBody：见文件头。
func (DownloadBillRequest) xpayNoEnv() {}

func (r DownloadBillRequest) validate() error {
	// 「两个都是合法 YYYYMMDD + 止不早于起」与下载订单任务共用一份实现
	// （xpay_common.go 的 checkDayRange），本接口没有它那个 31 天上限。
	return checkDayRange(r.BeginDs, r.EndDs)
}

// DownloadBillResponse 是下载日账单的响应体。
type DownloadBillResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// URL 账单下载地址，有效期为半小时，失效后需重新获取。
	//
	// **为空不是错误**：说明账单还在生成，隔一会儿再调一次本接口就是了（见
	// DownloadBill 的注释）。「拿到链接没有」看 resp.URL == ""，「这一趟调用成没成」
	// 看 resp.ErrCode，两件事分开。
	URL string `json:"url"`
}

// DownloadBill 下载普通虚拟支付按日汇总的结算账单。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证。自研小程序传 GetStableAccessToken 换来的 access_token，第三方
//	             平台代商家调用传 authorizer_access_token——两者在这里是同一种东西。
//	appKey       商家密钥，用来算 pay_sig。本接口的请求体没有 env 可配，所以这里**无从**
//	             判断它是哪套环境的 key——用哪套就查哪套的账，由调用方自己配对。
//	req          时间区间：BeginDs / EndDs（YYYYMMDD），止不早于起。
//
// 本接口是**轮询式**的：首次调用触发生成下载 URL，若返回的 URL 为空说明还在生成中，
// 间隔一段时间重试即可。拿到 URL 后请尽快下载——有效期只有半小时。
//
// ⚠️ 本接口的请求体里**没有 env**（官方字段表里就没有这一行），所以本方法没有 Env 参数，
// 也不像别处那样校验 key 与环境配套。见文件头。
//
// 官方文档：POST /xpay/download_bill
func DownloadBill(ctx context.Context, accessToken, appKey string, req DownloadBillRequest) (*DownloadBillResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkAppKeyNoEnv(appKey); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp DownloadBillResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/download_bill", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 2/2  download_ios_settlement_bill —— 下载苹果 IAP 月账单
//
//	POST /xpay/download_ios_settlement_bill  access_token + pay_sig
// ---------------------------------------------------------------------------

// DownloadIOSBillRequest 是下载苹果 IAP 支付月账单的请求体。
//
// ⚠️ 它**没有 env 字段**，同 DownloadBillRequest（理由见文件头）。
type DownloadIOSBillRequest struct {
	// StartMonth 开始月份，格式 YYYYMM，如 202601。
	StartMonth string `json:"start_month"`
	// EndMonth 结束月份，格式 YYYYMM。
	EndMonth string `json:"end_month"`
}

// xpayNoEnv 同 DownloadBillRequest。
func (DownloadIOSBillRequest) xpayNoEnv() {}

func (r DownloadIOSBillRequest) validate() error {
	return checkMonthRange(r.StartMonth, r.EndMonth)
}

// IOSBill 是一张苹果 IAP 结算单。
type IOSBill struct {
	// Month 月份，格式 YYYYMM。
	Month string `json:"month"`
	// BillURL 账单下载链接，请及时使用，一定时间后失效。
	//
	// 官方只写了「一定时间后失效」，没给具体时长——别照 DownloadBill 那半小时去推算它。
	BillURL string `json:"bill_url"`
}

// DownloadIOSBillResponse 是下载苹果 IAP 月账单的响应体。
type DownloadIOSBillResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// BillList 结算单列表：请求里给的每个月一张。
	BillList []IOSBill `json:"bill_list"`
}

// DownloadIOSBill 下载指定月份的苹果 IAP 支付月账单及其下载链接。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 DownloadBill。
//	appKey       商家密钥，用来算 pay_sig；同 DownloadBill，本接口也没有 env 可配。
//	req          月份区间：StartMonth / EndMonth（YYYYMM），止不早于起。
//
// ⚠️ 本接口的请求体里**没有 env**（官方字段表里就没有这一行），同 DownloadBill。
//
// ⚠️ 函数名与路径对不上是有意的：接口在官方文档里叫「下载 IOS 结算账单」，路径是
// download_ios_settlement_bill，而响应里的字段是 bill_list/bill_url。改名字或改路径都会
// 404，两边都照官方原样。
//
// 官方文档：POST /xpay/download_ios_settlement_bill
func DownloadIOSBill(ctx context.Context, accessToken, appKey string, req DownloadIOSBillRequest) (*DownloadIOSBillResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkAppKeyNoEnv(appKey); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp DownloadIOSBillResponse
	if err := PostWithPaySig(ctx, accessToken, appKey, "/xpay/download_ios_settlement_bill", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
