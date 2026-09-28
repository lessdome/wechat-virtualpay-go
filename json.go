package wechat_virtualpay_go

import (
	"bytes"
	"encoding/json"
)

// marshalNoHTMLEscape 序列化 JSON，并关掉 HTML 转义。
//
// 真正要守的不变量只有一条：**只序列化一次，签的串与发的串是同一份字节**。
// 官方 2.6 的参考脚本把这条写在注释里——「实际使用时只需要保证，参与签名的
// post_body 和真正发起 http 请求的一致即可」，而它自己的示例干脆不用 json.dumps，
// 直接硬编码一个字面串，理由就是「JSON 数据序列化结果，不同语言/版本结果可能不同」。
// 所以问题从来不是「该不该转义」，而是「别序列化两次」。
//
// 关掉 HTML 转义是顺带的：Go 的 encoding/json 默认会把 < > & 这三个字符写成
// Unicode 转义序列（本包把这层转义关掉了）。它不影响签名是否成立——签的串就是
// 发的串——但会让下发给小程序端的 signData 多一层没人需要的转义，排查时容易看错。
//
// 本包内部一律通过此函数序列化，绝不直接使用 json.Marshal。
func marshalNoHTMLEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// json.Encoder.Encode 会在末尾追加一个换行；去掉它，让返回值与请求体、
	// 与参与签名的原文完全一致。
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
