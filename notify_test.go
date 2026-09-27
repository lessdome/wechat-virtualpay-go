package virtualpay

import (
	"crypto/aes"
	"encoding/json"
	"encoding/xml"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// 本文件里的常量全部来自微信官方《消息推送》文档正文——文档把完整输入与期望输出
// 都印在了示例里，可以直接当权威测试向量用。这批向量覆盖了验签与 AES 加解密，
// 是本包能对微信协议做的最强验证（比任何自生成向量都可靠）。

const (
	// 官方示例：Token 令牌。
	officialToken = "AAAAA"
	// 官方示例：EncodingAESKey（43 个 'A'，解码后是 32 字节全零密钥）。
	officialAESKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	// 官方示例：小程序 AppID，写在解密后的明文尾部。
	officialAppID = "wxba5fad812f8e6fb9"

	// 官方示例：明文模式。
	officialPlainTimestamp = "1714037059"
	officialPlainNonce     = "486452656"
	// 官方给出的明文模式 signature。
	officialPlainSignature = "899cf89e464efb63f54ddac96b0a0a235f53aa78"

	// 官方示例：安全模式。
	officialAESTimestamp = "1714112445"
	officialAESNonce     = "415670741"
	// 官方给出的 msg_signature。
	officialMsgSignature = "046e02f8204d34f8ba5fa3b1db94908f3df2e9b3"
	// 官方给出的密文。
	officialEncrypt = "+qdx1OKCy+5JPCBFWw70tm0fJGb2Jmeia4FCB7kao+/Q5c/ohsOzQHi8khUOb05JCpj0JB4RvQMkUyus8TPxLKJGQqcvZqzDpVzazhZv6JsXUnnR8XGT740XgXZUXQ7vJVnAG+tE8NUd4yFyjPy7GgiaviNrlCTj+l5kdfMuFUPpRSrfMZuMcp3Fn2Pede2IuQrKEYwKSqFIZoNqJ4M8EajAsjLY2km32IIjdf8YL/P50F7mStwntrA2cPDrM1kb6mOcfBgRtWygb3VIYnSeOBrebufAlr7F9mFUPAJGj04="
	// 官方给出的解密后明文 msg（明文长度 167 字节）。
	officialDecryptedMsg = `{"ToUserName":"gh_97417a04a28d","FromUserName":"o9AgO5Kd5ggOC-bXrbNODIiE3bGY","CreateTime":1714112445,"MsgType":"event","Event":"debug_demo","debug_str":"hello world"}`

	// 官方示例：回包加密。
	officialReplyTimestamp = "1713424427"
	officialReplyNonce     = "415670741"
	officialReplyRandom16  = "707722b803182950"
	officialReplyPlain     = `{"demo_resp":"good luck"}`
	// 官方给出的回包密文与 MsgSignature。
	officialReplyEncrypt      = "ELGduP2YcVatjqIS+eZbp80MNLoAUWvzzyJxgGzxZO/5sAvd070Bs6qrLARC9nVHm48Y4hyRbtzve1L32tmxSQ=="
	officialReplyMsgSignature = "1b9339964ed2e271e7c7b6ff2b0ef902fc94dea1"
)

func officialNotifier(t *testing.T) *Notifier {
	t.Helper()
	n, err := NewNotifier(NotifyConfig{
		AppID:          officialAppID,
		Token:          officialToken,
		EncodingAESKey: officialAESKey,
	})
	if err != nil {
		t.Fatalf("NewNotifier: %v", err)
	}
	return n
}

// TestOfficialPlainSignature 用官方样例验证明文模式验签。
func TestOfficialPlainSignature(t *testing.T) {
	if !verifyPlainSignature(officialToken, officialPlainTimestamp, officialPlainNonce, officialPlainSignature) {
		t.Fatal("明文模式验签未通过官方样例")
	}
	if verifyPlainSignature(officialToken, officialPlainTimestamp, officialPlainNonce, "deadbeef") {
		t.Fatal("错误的 signature 不应通过")
	}
	// 换个 nonce 应当失败
	if verifyPlainSignature(officialToken, officialPlainTimestamp, "1", officialPlainSignature) {
		t.Fatal("nonce 不匹配时不应通过")
	}
}

// TestOfficialEncryptedMsgSignature 用官方样例验证安全模式验签。
func TestOfficialEncryptedMsgSignature(t *testing.T) {
	if !verifyEncryptedSignature(officialToken, officialAESTimestamp, officialAESNonce, officialEncrypt, officialMsgSignature) {
		t.Fatal("安全模式验签未通过官方样例")
	}
	// 文档明确警告：安全模式不要用 signature 验证。这里确认两者确实不同，
	// 防止将来有人"顺手"把两种情况合并。
	plainStyle := sha1SortedHex(officialToken, officialAESTimestamp, officialAESNonce)
	if plainStyle == officialMsgSignature {
		t.Fatal("安全模式的 msg_signature 不应等于不带 Encrypt 的三参版本")
	}
}

// TestOfficialAESDecrypt 用官方样例验证解密与 appid 校验。
func TestOfficialAESDecrypt(t *testing.T) {
	key, err := decodeAESKey(officialAESKey)
	if err != nil {
		t.Fatalf("decodeAESKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("AESKey 应为 32 字节，实际 %d", len(key))
	}

	msg, err := aesDecrypt(key, officialEncrypt, officialAppID)
	if err != nil {
		t.Fatalf("aesDecrypt: %v", err)
	}
	if string(msg) != officialDecryptedMsg {
		t.Fatalf("解密结果不符\n得到 %s\n期望 %s", msg, officialDecryptedMsg)
	}

	// appid 不匹配时必须失败（防止拿别人的报文打自己的接口）
	if _, err := aesDecrypt(key, officialEncrypt, "wx0000000000000000"); err == nil {
		t.Fatal("appid 不匹配时应当报错")
	}
}

// TestOfficialAESEncrypt 验证加密结果与官方样例**逐字节一致**。
//
// 这是本包对微信加密格式最硬的验证：给定官方用的随机串，我们必须产出官方那串密文。
func TestOfficialAESEncrypt(t *testing.T) {
	key, err := decodeAESKey(officialAESKey)
	if err != nil {
		t.Fatalf("decodeAESKey: %v", err)
	}
	got, err := aesEncrypt(string(key), officialAppID, []byte(officialReplyPlain), []byte(officialReplyRandom16))
	if err != nil {
		t.Fatalf("aesEncrypt: %v", err)
	}
	if got != officialReplyEncrypt {
		t.Fatalf("加密结果与官方不符\n得到 %s\n期望 %s", got, officialReplyEncrypt)
	}

	// 回包的 MsgSignature 算法（四参、含 Encrypt）
	if sig := sha1SortedHex(officialToken, officialReplyTimestamp, officialReplyNonce, officialReplyEncrypt); sig != officialReplyMsgSignature {
		t.Fatalf("回包 MsgSignature 不符\n得到 %s\n期望 %s", sig, officialReplyMsgSignature)
	}
}

// TestParseEncrypted 端到端：安全模式（密文信封 → 验签 → 解密 → 解析事件）。
func TestParseEncrypted(t *testing.T) {
	n := officialNotifier(t)

	body := []byte(`{"Encrypt":"` + officialEncrypt + `"}`)
	q := url.Values{
		"timestamp":     {officialAESTimestamp},
		"nonce":         {officialAESNonce},
		"msg_signature": {officialMsgSignature},
		"encrypt_type":  {"aes"},
		// URL 上也会带 signature，但安全模式必须忽略它——这里故意填个错的，
		// 如果实现拿它验签就会失败。
		"signature": {"this-should-be-ignored"},
	}

	notif, err := n.Parse(q, body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// 官方样例是 debug_demo 事件，属未知事件——不应报错，但要能拿到事件名与明文。
	if notif.Event != "debug_demo" {
		t.Fatalf("Event = %q，期望 debug_demo", notif.Event)
	}
	if notif.Format != NotifyFormatJSON {
		t.Fatalf("Format = %q，期望 json", notif.Format)
	}
	if string(notif.Plain) != officialDecryptedMsg {
		t.Fatalf("明文不符\n得到 %s\n期望 %s", notif.Plain, officialDecryptedMsg)
	}
	if notif.Event.IsKnownEvent() {
		t.Fatal("debug_demo 不应被识别为已知事件")
	}
}

// TestParseEncryptedTampered 确认篡改密文会被拒绝（验签先于解密）。
func TestParseEncryptedTampered(t *testing.T) {
	n := officialNotifier(t)
	body := []byte(`{"Encrypt":"AAAA` + officialEncrypt + `"}`)
	q := url.Values{
		"timestamp":     {officialAESTimestamp},
		"nonce":         {officialAESNonce},
		"msg_signature": {officialMsgSignature},
		"encrypt_type":  {"aes"},
	}
	if _, err := n.Parse(q, body); err == nil {
		t.Fatal("密文被篡改后应当验签失败")
	}
}

// TestParsePlainJSON 端到端：明文模式 + JSON + 已知事件（道具发货）。
func TestParsePlainJSON(t *testing.T) {
	n := officialNotifier(t)

	const ts, nonce = "1714037059", "486452656"
	payload := `{"ToUserName":"gh_97417a04a28d","FromUserName":"o9AgO5Kd5ggOC-bXrbNODIiE3bGY",` +
		`"CreateTime":1714037059,"MsgType":"event","Event":"xpay_goods_deliver_notify",` +
		`"OpenId":"oUser123","OutTradeNo":"ORDER20260101001","Env":0,` +
		`"WeChatPayInfo":{"MchOrderNo":"WX123456","TransactionId":"TX1","PaidTime":1714037059},` +
		`"GoodsInfo":{"ProductId":"prod_001","Quantity":2,"OrigPrice":100,"ActualPrice":80,"Attach":"ord1"}}`

	q := url.Values{
		"timestamp": {ts},
		"nonce":     {nonce},
		"signature": {sha1SortedHex(officialToken, ts, nonce)},
	}

	notif, err := n.Parse(q, []byte(payload))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if notif.Event != EventGoodsDeliver || !notif.Event.IsKnownEvent() {
		t.Fatalf("Event = %q", notif.Event)
	}
	g := notif.GoodsDeliver
	if g == nil {
		t.Fatal("GoodsDeliver 为空")
	}
	if g.OpenID != "oUser123" || g.OutTradeNo != "ORDER20260101001" {
		t.Fatalf("基础字段解析错误: %+v", g)
	}
	if g.WeChatPayInfo == nil || g.WeChatPayInfo.MchOrderNo != "WX123456" {
		t.Fatalf("WeChatPayInfo 解析错误: %+v", g.WeChatPayInfo)
	}
	if g.GoodsInfo == nil || g.GoodsInfo.Quantity != 2 || g.GoodsInfo.ActualPrice != 80 {
		t.Fatalf("GoodsInfo 解析错误: %+v", g.GoodsInfo)
	}
}

// TestParsePlainXML 端到端：明文模式 + XML（含 CDATA 与嵌套结构）。
//
// XML 与 JSON 共用同一批结构体（双 tag），这条测试确保 XML 路径真的可用——
// MP 后台的数据格式是可配置的，用 XML 的接入方不少。
func TestParsePlainXML(t *testing.T) {
	n := officialNotifier(t)

	const ts, nonce = "1714037059", "486452656"
	payload := `<xml>` +
		`<ToUserName><![CDATA[gh_97417a04a28d]]></ToUserName>` +
		`<FromUserName><![CDATA[o9AgO5Kd5ggOC-bXrbNODIiE3bGY]]></FromUserName>` +
		`<CreateTime>1714037059</CreateTime>` +
		`<MsgType><![CDATA[event]]></MsgType>` +
		`<Event><![CDATA[xpay_goods_deliver_notify]]></Event>` +
		`<OpenId><![CDATA[oUser123]]></OpenId>` +
		`<OutTradeNo><![CDATA[ORDER20260101001]]></OutTradeNo>` +
		`<Env>0</Env>` +
		`<WeChatPayInfo><MchOrderNo><![CDATA[WX123456]]></MchOrderNo>` +
		`<TransactionId><![CDATA[TX1]]></TransactionId><PaidTime>1714037059</PaidTime></WeChatPayInfo>` +
		`<GoodsInfo><ProductId><![CDATA[prod_001]]></ProductId><Quantity>2</Quantity>` +
		`<OrigPrice>100</OrigPrice><ActualPrice>80</ActualPrice><Attach><![CDATA[ord1]]></Attach></GoodsInfo>` +
		`</xml>`

	q := url.Values{
		"timestamp": {ts},
		"nonce":     {nonce},
		"signature": {sha1SortedHex(officialToken, ts, nonce)},
	}

	notif, err := n.Parse(q, []byte(payload))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if notif.Format != NotifyFormatXML {
		t.Fatalf("Format = %q，期望 xml", notif.Format)
	}
	g := notif.GoodsDeliver
	if g == nil {
		t.Fatal("GoodsDeliver 为空")
	}
	if g.OpenID != "oUser123" || g.OutTradeNo != "ORDER20260101001" {
		t.Fatalf("基础字段解析错误: %+v", g)
	}
	if g.WeChatPayInfo == nil || g.WeChatPayInfo.MchOrderNo != "WX123456" || g.WeChatPayInfo.PaidTime != 1714037059 {
		t.Fatalf("WeChatPayInfo 解析错误: %+v", g.WeChatPayInfo)
	}
	if g.GoodsInfo == nil || g.GoodsInfo.ProductID != "prod_001" || g.GoodsInfo.Quantity != 2 {
		t.Fatalf("GoodsInfo 解析错误: %+v", g.GoodsInfo)
	}
}

// TestParseRejects 覆盖各种应当被拒绝的输入。
func TestParseRejects(t *testing.T) {
	n := officialNotifier(t)

	base := func() url.Values {
		return url.Values{
			"timestamp": {"1714037059"},
			"nonce":     {"486452656"},
			"signature": {sha1SortedHex(officialToken, "1714037059", "486452656")},
		}
	}

	t.Run("空 body", func(t *testing.T) {
		if _, err := n.Parse(base(), nil); err == nil {
			t.Fatal("空 body 应当报错")
		}
	})
	t.Run("缺 timestamp", func(t *testing.T) {
		q := base()
		q.Del("timestamp")
		if _, err := n.Parse(q, []byte(`{"Event":"x"}`)); err == nil {
			t.Fatal("缺 timestamp 应当报错")
		}
	})
	t.Run("验签不通过", func(t *testing.T) {
		q := base()
		q.Set("signature", "0000000000000000000000000000000000000000")
		if _, err := n.Parse(q, []byte(`{"Event":"x"}`)); err != ErrInvalidSignature {
			t.Fatalf("期望 ErrInvalidSignature，实际 %v", err)
		}
	})
	t.Run("安全模式但未配置密钥", func(t *testing.T) {
		plainOnly, err := NewNotifier(NotifyConfig{AppID: officialAppID, Token: officialToken})
		if err != nil {
			t.Fatal(err)
		}
		if plainOnly.SupportsEncrypted() {
			t.Fatal("未配置 EncodingAESKey 时 SupportsEncrypted 应为 false")
		}
		q := base()
		q.Set("encrypt_type", "aes")
		if _, err := plainOnly.Parse(q, []byte(`{"Encrypt":"xxx"}`)); err == nil {
			t.Fatal("未配置密钥却收到安全模式推送，应当报错")
		}
	})
	t.Run("首字符无法识别", func(t *testing.T) {
		if _, err := n.Parse(base(), []byte("not json nor xml")); err == nil {
			t.Fatal("无法识别的格式应当报错")
		}
	})
}

// TestAckFormats 验证两种格式的应答体。
func TestAckFormats(t *testing.T) {
	jsonBody, ct := Ack(NotifyFormatJSON)
	if ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(jsonBody, &got); err != nil {
		t.Fatalf("JSON 应答无法解析: %v (%s)", err, jsonBody)
	}
	if got["ErrCode"] != float64(0) || got["ErrMsg"] != "success" {
		t.Fatalf("JSON 应答内容不符: %v", got)
	}

	xmlBody, ct := Ack(NotifyFormatXML)
	if ct != "application/xml; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if !strings.Contains(string(xmlBody), "<ErrCode>0</ErrCode>") {
		t.Fatalf("XML 应答缺少 ErrCode: %s", xmlBody)
	}
	var back ackPayload
	if err := xml.Unmarshal(xmlBody, &back); err != nil {
		t.Fatalf("XML 应答无法解析: %v (%s)", err, xmlBody)
	}
	if back.ErrCode != 0 || back.ErrMsg != "success" {
		t.Fatalf("XML 应答内容不符: %+v", back)
	}

	errBody, _ := AckError(NotifyFormatJSON, 1, "发货失败")
	if !strings.Contains(string(errBody), `"ErrCode":1`) {
		t.Fatalf("失败应答不符: %s", errBody)
	}
}

// TestParseHTTP 验证从 *http.Request 取值的便捷路径。
func TestParseHTTP(t *testing.T) {
	n := officialNotifier(t)

	const ts, nonce = "1714037059", "486452656"
	payload := `{"ToUserName":"gh_97417a04a28d","FromUserName":"oU","CreateTime":1714037059,` +
		`"MsgType":"event","Event":"xpay_coin_pay_notify","OpenId":"oUser123",` +
		`"OutTradeNo":"ORDER1","Env":0,"CoinInfo":{"Quantity":10,"OrigPrice":100,"ActualPrice":100,"Attach":"a"}}`

	r := httptest.NewRequest("POST", "/notify?timestamp="+ts+"&nonce="+nonce+
		"&signature="+sha1SortedHex(officialToken, ts, nonce), strings.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")

	notif, err := n.ParseHTTP(r)
	if err != nil {
		t.Fatalf("ParseHTTP: %v", err)
	}
	if notif.Event != EventCoinPay {
		t.Fatalf("Event = %q", notif.Event)
	}
	if notif.CoinPay == nil || notif.CoinPay.CoinInfo == nil || notif.CoinPay.CoinInfo.Quantity != 10 {
		t.Fatalf("CoinPay 解析错误: %+v", notif.CoinPay)
	}
}

// TestEncryptResponse 验证安全模式下的加密应答可被解回。
func TestEncryptResponse(t *testing.T) {
	n := officialNotifier(t)

	plain := []byte(`{"result_code":0,"result_info":"ok","evidence":"用户未收到道具"}`)
	body, err := n.EncryptResponse(plain, NotifyFormatJSON)
	if err != nil {
		t.Fatalf("EncryptResponse: %v", err)
	}

	var env struct {
		Encrypt string `json:"Encrypt"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("应答信封无法解析: %v", err)
	}
	// 用同样的密钥解回来，验证格式自洽
	back, err := aesDecrypt(n.aesKey, env.Encrypt, officialAppID)
	if err != nil {
		t.Fatalf("aesDecrypt: %v", err)
	}
	if string(back) != string(plain) {
		t.Fatalf("往返不一致\n得到 %s\n期望 %s", back, plain)
	}

	// 未配置密钥时不应能加密
	plainOnly, err := NewNotifier(NotifyConfig{AppID: officialAppID, Token: officialToken})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plainOnly.EncryptResponse(plain, NotifyFormatJSON); err == nil {
		t.Fatal("未配置 EncodingAESKey 时 EncryptResponse 应当报错")
	}
}

// TestNewNotifierValidation 覆盖配置校验。
func TestNewNotifierValidation(t *testing.T) {
	cases := map[string]NotifyConfig{
		"缺 AppID": {Token: "t"},
		"缺 Token": {AppID: "a"},
		"密钥长度不对":  {AppID: "a", Token: "t", EncodingAESKey: "tooshort"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewNotifier(cfg); err == nil {
				t.Fatal("期望报错，实际通过")
			}
		})
	}
}

// TestPKCS7BlockSizeIs32 锁死填充块大小这个反直觉的细节。
//
// 微信的 PKCS#7 填充块是**密钥长度 32**，不是 AES 的 16 字节块。搞错的话解密时
// 填充校验会失败，而且只有在真实推送到来时才暴露——单元测试不专门覆盖就发现不了。
//
// 官方样例是硬证据：205 字节明文 → 224 字节密文，补了 19 个 0x13，
// 而 19 = 32 - 205%32（按 16 算只会补 3 个字节、得到 208 字节）。
func TestPKCS7BlockSizeIs32(t *testing.T) {
	if wechatPKCS7BlockSize != 32 {
		t.Fatalf("微信的 PKCS#7 填充块应为 32（密钥长度），实际 %d", wechatPKCS7BlockSize)
	}

	padded := pkcs7Pad(make([]byte, 205), wechatPKCS7BlockSize)
	if len(padded) != 224 {
		t.Fatalf("205 字节按 32 填充后应为 224 字节，实际 %d", len(padded))
	}
	if padded[len(padded)-1] != 19 {
		t.Fatalf("填充字节应为 19，实际 %d", padded[len(padded)-1])
	}
	// 对照组：按 AES 块大小 16 填充只能得到 208 字节，那正是错误实现的症状。
	if got := len(pkcs7Pad(make([]byte, 205), aes.BlockSize)); got != 208 {
		t.Fatalf("对照组：按 16 填充应为 208，实际 %d", got)
	}

	back, err := pkcs7Unpad(padded, wechatPKCS7BlockSize)
	if err != nil {
		t.Fatalf("pkcs7Unpad: %v", err)
	}
	if len(back) != 205 {
		t.Fatalf("往返后长度应为 205，实际 %d", len(back))
	}

	// 填充字节不一致时必须报错（防 padding oracle）
	bad := make([]byte, 32)
	bad[31] = 8
	bad[30] = 7
	if _, err := pkcs7Unpad(bad, wechatPKCS7BlockSize); err == nil {
		t.Fatal("填充字节不一致时应当报错")
	}
	// 填充长度为 0 时也必须报错
	if _, err := pkcs7Unpad(make([]byte, 32), wechatPKCS7BlockSize); err == nil {
		t.Fatal("填充长度为 0 时应当报错")
	}
}
