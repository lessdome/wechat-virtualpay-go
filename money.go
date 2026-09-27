package wechat_virtualpay_go

// Yuan 是**单位为「元」**的金额。
//
// 本 SDK 里绝大多数金额的单位是**分**（`GoodsPrice`、`OrderFee`、`PaidFee`、
// `RefundFee`、代币 `Amount`、广告金各项…），只有提现相关的少数几个字段用「元」。
//
// 单独给一个具名类型，是为了让单位**在类型上就能看见**——字段名 `Amount` 本身
// 不携带单位信息，而把「元」的金额按「分」的直觉去用，会差 100 倍。
//
// 值用字符串表示（微信如此返回/接收），例如 `Yuan("0.01")` 表示 1 分钱。
type Yuan string
