package wechat_virtualpay_go

// 本文件提供推送应答的**数据模型**，不含任何生成函数——应答的字节由使用者自己
// json.Marshal 写出去，Content-Type 用 application/json; charset=utf-8。
//
// 官方 2.4「推送响应格式说明」给了三种应答方式，本包覆盖其中推荐的 ErrCode 方式
// （JSON 形态），以及 iOS 退款问询那一种：
//
//	普通事件      → Ack
//	iOS 退款问询  → IOSRefundQueryResponse（**不是** ErrCode 形态）
//
// 回包格式不对、或回了失败，微信会重新推送：重试间隔 2 4 8 16…，最多 15 次
// （这句出自推送字段表的 RetryTimes 一栏）。
//
// ⚠️ 官方还要求**应答格式与推送格式一致**（XML 推送回 XML、JSON 推送回 JSON）。
// 本包只支持 JSON 报文，所以这条自然满足。

// Ack 是普通推送事件的应答（ErrCode 方式）。
//
//	body, _ := json.Marshal(wechat_virtualpay_go.Ack{ErrCode: 0, ErrMsg: "success"})
//	w.Header().Set("Content-Type", "application/json; charset=utf-8")
//	w.Write(body)
//
// 两种取值的含义：
//
//	ErrCode = 0      微信不再重推。**务必确认发货真的落地了再回成功**——回了成功但
//	                 没发货，微信不会重试，这笔单就永久丢了。
//	ErrCode 非 0     微信按 2 4 8 16… 的间隔重试，最多 15 次。
//
// ErrMsg 只用于调试，别塞敏感信息。
//
// ⚠️ **零值就是成功应答**（ErrCode 的零值是 0）。协议如此，没法让零值变成失败，所以
// `var ack Ack` 未经赋值 marshal 出去就是「已处理完，别再推了」——回成功是承诺，
// 不是默认值。
type Ack struct {
	// ErrCode 应答状态。0 表示成功，其他值微信会重试。
	ErrCode int `json:"ErrCode"`
	// ErrMsg 错误信息，用于调试。成功时官方示例给的是 "success"。
	ErrMsg string `json:"ErrMsg"`
}

// IOSRefundQueryResponse 是 **iOS 退款问询**（xpay_subscribe_ios_refund_query_notify）
// 的应答。
//
// 这条问询的应答**不是** Ack 那种 ErrCode 形态，只能用它——回错了微信当无效应答，
// 而 Apple 只问询三次、每次 3 秒，错过等于把判定权交出去。
//
//	body, _ := json.Marshal(wechat_virtualpay_go.IOSRefundQueryResponse{
//		ResultCode: 0,
//		ResultInfo: "已发货，不予退款",
//		Evidence:   "该订单已于 2026-01-01 发放并被用户领取",
//	})
//
// ⚠️ 必须在 **3 秒内**返回；这条路径上不要查库、不要调外部接口，否则会被判为「不确定」。
type IOSRefundQueryResponse struct {
	// ResultCode 结果码：0-放过，建议退款；1-拦截，拒绝退款。
	ResultCode int32 `json:"result_code"`
	// ResultInfo 结果描述。
	ResultInfo string `json:"result_info"`
	// Evidence 决策凭据（**必填**），业务需给出建议退款/拒绝退款的依据，用于退款审计。
	Evidence string `json:"evidence"`
}
