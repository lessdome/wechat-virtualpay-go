package wechat_virtualpay_go

import (
	"bytes"
	"encoding/json"
)

// marshalNoHTMLEscape 序列化 JSON，并关闭 HTML 转义。
//
// 为什么这个函数是整个 SDK 的地基之一：
//
// 虚拟支付的签名（pay_sig）是对「一段具体的 JSON 字符串」做 HMAC 得到的。
// 也就是说，服务端构建的字符串、算签名用的字符串、下发给前端的字符串、
// 微信最终收到并校验的字符串——这四份必须是同一个字节序列。
//
// 而 Go 标准库的 json.Marshal 默认会把 < > & 转义成 < > &。
// 一旦你的参数里（比如道具名、attach 透传数据）含有这些字符，
// 转义后的字符串就和微信预期的原文不一致，签名随即失败——
// 而微信只会回一个笼统的「签名错误」，极难排查：服务端接口报 268490003，
// 小程序端拉起支付报 -15006。
//
// 所以本 SDK 内部一律通过此函数序列化，绝不直接使用 json.Marshal。
//
// 另外：json.Encoder.Encode 会在末尾追加一个换行符，这里一并去掉，
// 保证返回值与请求/签名使用的原文完全一致。
func marshalNoHTMLEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
