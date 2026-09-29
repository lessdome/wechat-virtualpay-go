package wechat_virtualpay

import (
	"context"
	"encoding/json"
	"fmt"
)

// 本文件是官方 /xpay/* 里「代币」这一类 4 个接口：
//
//	POST /xpay/query_user_balance   查询用户代币余额  access_token + pay_sig + signature
//	POST /xpay/currency_pay         扣减代币          access_token + pay_sig + signature
//	POST /xpay/cancel_currency_pay  代币支付退款      access_token + pay_sig + signature
//	POST /xpay/present_currency     代币赠送          access_token
//
// 前三个是全包仅有的三个**用户态**接口：它们动的是某个用户的代币账户，所以除商家的
// pay_sig 外还要一把用**那个用户的** session_key 签的 signature。本类的三个方法因此比
// 订单类的多一个 sessionKey 参数——两把钥匙不能互换，算法也不同（见 PostWithUserSig）。
// 第四个一个签名都不带，与订单类的 NotifyProvideGoods 同档。
//
// 其余约定与订单类**逐字同义**，不在这里重抄（见 xpay_order.go 文件头）：凭据显式传参、
// env 是请求结构体上的一个裸 int（0=现网 / 1=沙箱）、字段顺序照官方字段表逐行抄、
// 响应**原值返回**、`err == nil` 不等于成功（成败看 resp.ErrCode）、失败时响应为 nil
// ——`resp == nil` 与 `resp.ErrCode != 0` 是两件事。
//
// 文件按接口分段，每段是「请求结构体 → 本地校验 → 响应结构体 → 调用函数」，
// 读一个接口只需要看一段。
//
// 本类**故意不做**的本地校验（写在这里，免得下一个人当成漏了、顺手补上）：
//
//   - **不查 UserIP 的格式**：官方只写了一句「用户 IP，形如 1.1.1.1」，那是说明文字的
//     举例，不是约束——反代 / NAT / IPv6 下真实取到的 IP 未必长这样。只拦空串。
//   - **不查单号的格式**：与下单那页不同，这几页没有 outTradeNo 那种「长度 8–32、限制
//     字符集」的规范。不发明约束，只查非空。
//   - **不查 PayItem 的内容**：它是调用方给的一段字符串（一个 JSON 数组），官方只给了
//     示例、没给字段表。要校验就得先定义「合法的道具明细长什么样」，那是本包无权发明的
//     规矩；MarshalPayItems 只是省手写，不是必经之路。
//   - **不查 Quantity / UnitPrice 的取值范围**：官方只把它们记进流水，没说能不能为 0。

// checkCoinAmount 校验代币数量为正。三个要动代币的接口共用这一条。
//
// ⚠️ 这条是**从语义推出来的，不是文档写的**：官方这几页对 amount 只说「代币数量」，
// 没有一个字讲取值范围。但 0 个代币的扣减/退款/赠送在任何读法下都是空操作，负数更是
// 没有定义——把语义未定义的金额发到钱路上，是本包最不该做的事。若真机上 0 有特殊含义
// （例如表示全额），把这里放开就是了。
//
// what 是「这一趟在做什么」（扣减/退款/赠送），让报错能指回是哪个接口。
func checkCoinAmount(amount int64, what string) error {
	if amount <= 0 {
		return fmt.Errorf("wechat_virtualpay: Amount 必须大于 0（%s的代币数量，0 与负数都没有含义）", what)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 1/4  query_user_balance —— 查询用户代币余额
//
//	POST /xpay/query_user_balance  access_token + pay_sig + signature
// ---------------------------------------------------------------------------

// QueryUserBalanceRequest 是查询用户代币余额的请求体。
type QueryUserBalanceRequest struct {
	// OpenID 用户的 openid。必填。
	OpenID string `json:"openid"`
	// UserIP 用户 IP，官方写「形如 1.1.1.1」。必填（本包只查非空，不查格式，理由见文件头）。
	UserIP string `json:"user_ip"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

func (r QueryUserBalanceRequest) validate() error {
	if r.OpenID == "" {
		return fmt.Errorf("wechat_virtualpay: OpenID 不能为空")
	}
	if r.UserIP == "" {
		return fmt.Errorf("wechat_virtualpay: UserIP 不能为空（官方标必填，形如 1.1.1.1）")
	}
	return nil
}

// QueryUserBalanceResponse 是查询用户代币余额的响应体。
//
// 余额分两本账：有价的（Balance 里不含赠送）与赠送的（PresentBalance）。Balance 是两者
// 之和，所以「能不能付得起这一单」看 Balance，而「有多少是送的」看 PresentBalance
// ——CurrencyPay 扣减时优先扣赠送部分，扣了多少回在 UsedPresentAmount 里。
type QueryUserBalanceResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// Balance 代币总余额，包括有价和赠送部分。
	Balance int64 `json:"balance"`
	// PresentBalance 赠送账户的代币余额。
	PresentBalance int64 `json:"present_balance"`
	// SumSave 累计有价货币充值数量。
	SumSave int64 `json:"sum_save"`
	// SumPresent 累计赠送无价货币数量。
	SumPresent int64 `json:"sum_present"`
	// SumBalance 历史总增加的代币金额。
	SumBalance int64 `json:"sum_balance"`
	// SumCost 历史总消耗代币金额。
	SumCost int64 `json:"sum_cost"`
	// FirstSaveFlag 是否满足首充活动。
	//
	// ⚠️ 文档在这一个字段上有**三种互相矛盾**的表示：类型列写 boolean，说明列写
	// 「0:不满足。1:满足」，示例写 false。本包按**类型列**取 bool——本仓库的规矩是
	// 「按文档的类型列忠实映射，不给它造防御类型」（判据见 xpay_common.go 里 checkEnv
	// 那段：有没有配套常量块）。
	//
	// 代价要说清：类型错了，**整个响应**会反序列化失败，不是少一个字段——报错会带着原始
	// 报文（见 xpaySend），所以不会静默，但这一趟也就读不到 errcode 了。真机若证实微信
	// 回的是 0/1，改法是给这个字段换一个能同时吃 true/false 与 0/1 的自定义反序列化类型，
	// 并同步改 xpay_coin_test.go 的 TestCoinFieldsCoverDoc（那里逐字段钉类型）。
	//
	// 「`pre-rewrite` 分支上那份被删掉的实现」也是同样取 bool、同样记了这处三矛盾——两次
	// 照同一张表读出的结论一致。不算独立证据（同一份文档、同一个读法），但至少说明这不是
	// 这一次新押的注。
	FirstSaveFlag bool `json:"first_save_flag"`
}

// QueryUserBalance 查询用户当前的代币余额。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证。自研小程序传 GetStableAccessToken 换来的 access_token，第三方
//	             平台代商家调用传 authorizer_access_token——两者在这里是同一种东西。
//	appKey       商家密钥，算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	sessionKey   **这个用户的**用户密钥，算 signature。由 Code2Session 用前端 wx.login
//	             的 code 换来，会过期。它与 appKey 不能互换：一个签用户、一个签商家。
//	req          查询条件：OpenID 与 UserIP。
//
// 这是三个用户态接口里唯一只读的一个，也是另外两个的最佳搭档：赠币 / 扣币之前先查一眼
// 余额，事后好对账（这两个动作都没有「查一下我这单到底成没成」的接口，见 PresentCurrency）。
//
// 官方文档：POST /xpay/query_user_balance
func QueryUserBalance(ctx context.Context, accessToken, appKey, sessionKey string, req QueryUserBalanceRequest) (*QueryUserBalanceResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := checkSessionKey(sessionKey); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp QueryUserBalanceResponse
	if err := PostWithUserSig(ctx, accessToken, appKey, sessionKey, "/xpay/query_user_balance", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 2/4  currency_pay —— 扣减代币
//
//	POST /xpay/currency_pay  access_token + pay_sig + signature
// ---------------------------------------------------------------------------

// PayItem 是 CurrencyPayRequest.PayItem 里的一项道具明细，会被记进账户流水。
type PayItem struct {
	// ProductID 物品 ID。对应 JSON 数组里的 productid。
	ProductID string `json:"productid"`
	// UnitPrice 单价。
	UnitPrice int64 `json:"unit_price"`
	// Quantity 数量。
	Quantity int64 `json:"quantity"`
}

// MarshalPayItems 把道具明细序列化成 CurrencyPayRequest.PayItem 要的那个字符串。
//
// currency_pay 的 payitem 要求的是一段**JSON 数组的字符串**，序列化结果就是下面这样
// （紧凑、没有空格）：
//
//	[{"productid":"A","unit_price":100,"quantity":2}]
//
// 手写容易出错（漏引号、字段名写错），交给这个函数。注意**字段名是官方的 payitem
// （单数、无下划线）**，而这个函数的名字是复数——两者不一致，别照着函数名去写字段名。
//
// 不传参数时得到 `[]`（一个空数组，不是 `null`）。这一处是**有意与直觉反着来的**：Go
// 的可变参数在没有实参时是 nil 切片，而 json.Marshal 把 nil 切片编成 `null`——`null`
// 送过去只会换来一个费解的参数错误，所以这里显式兜成空数组。
//
// 物品 ID 里如果有尖括号或 &，json.Marshal 会把它们写成 Unicode 转义（全包统一的序列化
// 口径）。这里**不用管**：payitem 是一段**字符串值**，微信那边 JSON 解码一次就还原成原
// 字符，别为了「看起来干净」去关掉转义（SetEscapeHTML）。
//
// error 返回值**实际取不到**：PayItem 的字段只有 string 与 int64，json.Marshal 不会失败。
// 留着它是为了调用点的形状与别的序列化一致（`s, err := ...`），不代表它会失败。
func MarshalPayItems(items ...PayItem) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("wechat_virtualpay: 序列化 payitem 失败: %w", err)
	}
	return string(raw), nil
}

// CurrencyPayRequest 是扣减代币的请求体。
//
// ⚠️ 官方这一页的参数表把本接口**所有**请求体字段的「必填」列都标成了「否」——这显然是
// 文档生成的问题：OpenID / Amount / OrderID / UserIP 缺任何一个都不可能调用成功。
// 本包按**实际语义**处理：这四个字段不加 omitempty（始终发送），并且在 validate() 里就
// 拦下空值——本地直接点出是哪个字段，好过发出去再从微信的「参数错误」里猜。PayItem 与
// Remark 才是真正可选的（都带 omitempty）。
type CurrencyPayRequest struct {
	// OpenID 用户的 openid。必填（依据是实际语义，不是上面那条不可信的必填列）。
	OpenID string `json:"openid"`
	// UserIP 用户 IP，形如 1.1.1.1。必填（本包只查非空，不查格式，理由见文件头）。
	UserIP string `json:"user_ip"`
	// Amount 要扣减的代币数量。必填，须大于 0（0 与负数在扣减代币上没有含义）。
	Amount int64 `json:"amount"`
	// OrderID 本单在你这边的业务单号。必填。
	//
	// 退款要拿它当 CancelCurrencyPayRequest.PayOrderID 传回去，所以**要存下来**。
	OrderID string `json:"order_id"`
	// PayItem 道具明细，值是**一个 JSON 数组的字符串**，会记进账户流水。可选。
	//
	// 用 MarshalPayItems 生成。⚠️ 字段名是官方的 payitem（单数、无下划线），而那个生成
	// 函数的全名是 MarshalPayItems（复数）——两者不一致，别照着函数名推断字段名。
	PayItem string `json:"payitem,omitempty"`
	// Remark 备注。可选。
	Remark string `json:"remark,omitempty"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

func (r CurrencyPayRequest) validate() error {
	if r.OpenID == "" {
		return fmt.Errorf("wechat_virtualpay: OpenID 不能为空")
	}
	if r.UserIP == "" {
		return fmt.Errorf("wechat_virtualpay: UserIP 不能为空（官方标必填，形如 1.1.1.1）")
	}
	if r.OrderID == "" {
		return fmt.Errorf("wechat_virtualpay: OrderID 不能为空（本单在你这边的业务单号，退款时要拿它当 PayOrderID 传回来）")
	}
	return checkCoinAmount(r.Amount, "要扣减")
}

// CurrencyPayResponse 是扣减代币的响应体。
type CurrencyPayResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// OrderID 订单号，就是请求里传的那个。
	OrderID string `json:"order_id"`
	// Balance 扣减后的总余额，包括有价和赠送部分。
	Balance int64 `json:"balance"`
	// UsedPresentAmount 本次用掉赠送部分的代币数量。对账时用它看这次扣了多少「送的」。
	UsedPresentAmount int64 `json:"used_present_amount"`
}

// CurrencyPay 扣减指定用户的代币，一般用于代币支付。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryUserBalance。
//	appKey       商家密钥，算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	sessionKey   **这个用户的**用户密钥，算 signature（见 QueryUserBalance）。
//	req          扣减参数：OpenID、UserIP、Amount、OrderID，另有可选的 PayItem/Remark，
//	             见 CurrencyPayRequest。
//
// ⚠️ 本接口**没有按单号查询的接口**：扣款这一趟网络超时，你没法回头问一句「这笔到底扣
// 没扣」。所以 OrderID 要存好，重试时**复用同一个单号**——微信靠单号判重，换个单号重发
// 就是真的再扣一次。要确定结果，办法是用 QueryUserBalance 对账余额。
//
// 官方文档：POST /xpay/currency_pay
func CurrencyPay(ctx context.Context, accessToken, appKey, sessionKey string, req CurrencyPayRequest) (*CurrencyPayResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := checkSessionKey(sessionKey); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp CurrencyPayResponse
	if err := PostWithUserSig(ctx, accessToken, appKey, sessionKey, "/xpay/currency_pay", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 3/4  cancel_currency_pay —— 代币支付退款
//
//	POST /xpay/cancel_currency_pay  access_token + pay_sig + signature
// ---------------------------------------------------------------------------

// CancelCurrencyPayRequest 是代币支付退款的请求体。
type CancelCurrencyPayRequest struct {
	// OpenID 用户的 openid。必填。
	OpenID string `json:"openid"`
	// UserIP 用户 IP，形如 1.1.1.1。必填（本包只查非空，不查格式，理由见文件头）。
	UserIP string `json:"user_ip"`
	// PayOrderID **原代币支付单号**，即当初调 CurrencyPay 时传的 OrderID。必填。
	PayOrderID string `json:"pay_order_id"`
	// OrderID **本次退款单**的单号。必填，而且重试要复用同一个。
	//
	// ⚠️ 与订单类的 RefundOrderRequest.OrderID **同名反义**，别直接复制粘贴：那边的
	// order_id 指**原支付单**，那边的「本次退款单」叫 RefundOrderID；本接口反过来，
	// order_id 就是本次退款单，原单在 PayOrderID 里。两个接口功能相近，这一处最容易抄错。
	OrderID string `json:"order_id"`
	// Amount 要退还的代币数量。必填，须大于 0。
	Amount int64 `json:"amount"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ **沙箱必须配沙箱 AppKey**——env=1 配现网那把会报签名错误（268490003）。
	Env int `json:"env"`
}

func (r CancelCurrencyPayRequest) validate() error {
	if r.OpenID == "" {
		return fmt.Errorf("wechat_virtualpay: OpenID 不能为空")
	}
	if r.UserIP == "" {
		return fmt.Errorf("wechat_virtualpay: UserIP 不能为空（官方标必填，形如 1.1.1.1）")
	}
	if r.PayOrderID == "" {
		return fmt.Errorf("wechat_virtualpay: PayOrderID 不能为空（原代币支付单号，即 CurrencyPay 时传的 OrderID）")
	}
	if r.OrderID == "" {
		return fmt.Errorf("wechat_virtualpay: OrderID 不能为空（本次退款单的单号，与原单 PayOrderID 不是一回事）")
	}
	return checkCoinAmount(r.Amount, "要退款")
}

// CancelCurrencyPayResponse 是代币支付退款的响应体。
type CancelCurrencyPayResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// OrderID 本次退款单的单号，就是请求里传的那个。
	OrderID string `json:"order_id"`
}

// CancelCurrencyPay 把已经扣掉的代币退回给用户，是 CurrencyPay 的逆操作。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryUserBalance。
//	appKey       商家密钥，算 pay_sig。**必须与 req.Env 配套**（env=0 现网、env=1 沙箱）。
//	sessionKey   **这个用户的**用户密钥，算 signature（见 QueryUserBalance）。
//	req          退款参数：OpenID、UserIP、PayOrderID（原支付单）、OrderID（本次退款单）、
//	             Amount，见 CancelCurrencyPayRequest。⚠️ 两个单号的含义与订单类的
//	             RefundOrderRequest 正好相反，别抄错。
//
// 与 CurrencyPay 一样，**没有按单号查询的接口**：超时就复用同一个 OrderID 重试，要确认
// 结果只能用 QueryUserBalance 对账。
//
// 官方文档：POST /xpay/cancel_currency_pay
func CancelCurrencyPay(ctx context.Context, accessToken, appKey, sessionKey string, req CancelCurrencyPayRequest) (*CancelCurrencyPayResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := checkAppKey(appKey, req.Env); err != nil {
		return nil, err
	}
	if err := checkSessionKey(sessionKey); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp CancelCurrencyPayResponse
	if err := PostWithUserSig(ctx, accessToken, appKey, sessionKey, "/xpay/cancel_currency_pay", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// 4/4  present_currency —— 代币赠送
//
//	POST /xpay/present_currency  access_token（无签名）
// ---------------------------------------------------------------------------

// PresentCurrencyRequest 是向用户赠送代币的请求体。
//
// ⚠️ 注意它**没有 user_ip**：三个用户态接口都要 UserIP，本接口不要（赠送不是用户自己
// 发起的动作，没有「用户的 IP」可传）。别照着上面那三个结构体顺手补一个。
type PresentCurrencyRequest struct {
	// OpenID 收到代币的用户的 openid。必填。
	OpenID string `json:"openid"`
	// OrderID 赠送单号。必填，重试时必须复用同一个——微信靠它判重（见 PresentCurrency）。
	OrderID string `json:"order_id"`
	// Amount 要赠送的代币数量。必填，须大于 0。
	Amount int64 `json:"amount"`
	// Env 调用环境：**0=现网（默认）/ 1=沙箱**。不填即现网——零值正好是 0，而且
	// requestBody 还会替你兜一个 0（官方把这个字段标为必填）。
	// ⚠️ 本接口没有签名，所以上面那句「沙箱必须配沙箱 AppKey」对它不适用——但环境仍然
	// 是发出去的（env=1 时赠送会打到沙箱账户上）。
	Env int `json:"env"`
}

func (r PresentCurrencyRequest) validate() error {
	if r.OpenID == "" {
		return fmt.Errorf("wechat_virtualpay: OpenID 不能为空")
	}
	if r.OrderID == "" {
		return fmt.Errorf("wechat_virtualpay: OrderID 不能为空（赠送单号，重试要复用同一个）")
	}
	return checkCoinAmount(r.Amount, "要赠送")
}

// PresentCurrencyResponse 是代币赠送的响应体。
type PresentCurrencyResponse struct {
	// 公共头（errcode/errmsg）内嵌在最前面，调用方用 resp.ErrCode 直接读。
	ResponseHeader
	// Balance 赠送后用户的总余额。
	Balance int64 `json:"balance"`
	// OrderID 赠送单号，就是请求里传的那个。
	OrderID string `json:"order_id"`
	// PresentBalance 用户收到的总赠送金额。
	PresentBalance int64 `json:"present_balance"`
}

// PresentCurrency 向指定用户赠送代币。
//
// 入参：
//
//	ctx          请求上下文，超时与取消由它管（client 另有 15 秒超时兜底）。
//	accessToken  调用凭证，同 QueryUserBalance。
//	req          赠送参数：OpenID、OrderID（赠送单号）、Amount，见 PresentCurrencyRequest。
//
// 注意它**没有 appKey 参数**：官方 query 参数表里本接口只有 access_token，没有 pay_sig
// （与订单类的 NotifyProvideGoods 同档）。这不是漏看，参数表本身就是证据。若实测
// resp.ErrCode 是 268490003（签名错误），说明它其实也要 pay_sig，那时改调 PostWithPaySig。
//
// 「`pre-rewrite` 分支上那份被删掉的实现」本接口也是 `callMerchant`（即本档）——两次照
// 同一张参数表读出的结论一致。不算独立证据，但至少说明这不是这一次新押的注。
//
// ⚠️ 本接口**没有按单号查询的接口**——官方没提供查赠送单的方法。所以赠送失败时没法回头
// 问一句「到底赠出去没有」，只能重试，而重试的两种回答都是好的：
//
//	errcode=0          赠送成功
//	errcode=268490004  重复操作（这一单已经送过了）——同样是「成了」的回答
//
// 但重试必须**复用同一个 OrderID**：微信靠单号判重，换个单号再发就是真的又送一次，而
// 本接口没有对账手段，「送重了」在返回里看不出来（要靠 QueryUserBalance 查余额才发现）。
//
// 官方文档：POST /xpay/present_currency
func PresentCurrency(ctx context.Context, accessToken string, req PresentCurrencyRequest) (*PresentCurrencyResponse, error) {
	if err := checkAccessToken(accessToken); err != nil {
		return nil, err
	}
	if err := checkEnv(req.Env); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	var resp PresentCurrencyResponse
	if err := PostTokenOnly(ctx, accessToken, "/xpay/present_currency", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
