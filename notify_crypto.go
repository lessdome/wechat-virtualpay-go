package wechat_virtualpay_go

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// 本文件实现微信「消息推送配置」的验签与加解密。
//
// 这套机制**与支付签名毫无关系**，不要混淆：
//
//	signature / msg_signature 用 SHA-1，密钥是**开发者在 MP 后台自填的 Token**；
//	pay_sig / signature（支付那两个）用 HMAC-SHA256，密钥是 AppKey / session_key。
//
// 两种模式的差异：
//
//	明文模式   signature     = sha1( sort([Token, timestamp, nonce]).join("") )
//	安全模式   msg_signature = sha1( sort([Token, timestamp, nonce, Encrypt]).join("") )
//
// ⚠️ 安全模式下**不要用 signature 验证**，要用 msg_signature——官方文档对此有明确
// 警告。URL 上两个参数都会带，容易拿错。

// sha1SortedHex 把若干字符串按字典序排序后拼接，取 SHA-1 的十六进制。
//
// 这是微信消息推送验签的核心算法。注意是**字符串拼接**（无分隔符），
// 且排序是逐字节的字典序（Go 的 sort.Strings 即如此）。
func sha1SortedHex(parts ...string) string {
	sort.Strings(parts)
	h := sha1.New()
	// 微信规定如此，非安全问题。
	h.Write([]byte(strings.Join(parts, ""))) //nolint:gosec
	return hex.EncodeToString(h.Sum(nil))
}

// verifyPlainSignature 校验明文模式的 signature。
func verifyPlainSignature(token, timestamp, nonce, signature string) bool {
	expected := sha1SortedHex(token, timestamp, nonce)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

// verifyEncryptedSignature 校验安全模式的 msg_signature。
func verifyEncryptedSignature(token, timestamp, nonce, encrypt, msgSignature string) bool {
	expected := sha1SortedHex(token, timestamp, nonce, encrypt)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(msgSignature)) == 1
}

// decodeAESKey 由 EncodingAESKey 还原出 32 字节的 AESKey。
//
// 规则：EncodingAESKey 是 43 个字符，尾部补一个 "=" 后做 base64 解码，得 32 字节。
func decodeAESKey(encodingAESKey string) ([]byte, error) {
	if len(encodingAESKey) != 43 {
		return nil, fmt.Errorf("wechat_virtualpay_go: EncodingAESKey 应为 43 个字符，实际 %d 个", len(encodingAESKey))
	}
	key, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: EncodingAESKey 不是合法的 base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("wechat_virtualpay_go: AESKey 应为 32 字节，实际 %d 字节", len(key))
	}
	return key, nil
}

// wechatPKCS7BlockSize 是微信规定的 PKCS#7 填充块大小。
//
// ⚠️ 这是**密钥长度 32**，不是 AES 的块大小 16 —— 很容易搞错。官方文档原文：
//
//	PKCS#7：K 为秘钥字节数（采用 32），Buf 为待加密的内容，N 为其字节数。
//	Buf 需要被填充为 K 的整数倍。在 Buf 的尾部填充(K - N%K)个字节。
//
// 官方样例可以印证：一段 205 字节的明文，密文是 224 字节，即补了 19 个字节，
// 而 19 = 32 - 205%32。若按 16 计算只会补 3 个字节（208 字节密文），对不上。
//
// 用错会导致解密时填充校验失败——而且失败得很隐蔽：字节数恰好是 16 的倍数时
// 也可能"看起来正常"，直到遇到真实的推送才炸。
const wechatPKCS7BlockSize = 32

// pkcs7Unpad 去掉 PKCS#7 填充，并校验填充字节的一致性。
//
// 校验填充是必要的：不校验会给出一个 padding oracle。
// maxPad 为填充块大小，微信规定为 wechatPKCS7BlockSize。
func pkcs7Unpad(data []byte, maxPad int) ([]byte, error) {
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, errors.New("wechat_virtualpay_go: 密文长度不是 16 的整数倍")
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > maxPad || pad > len(data) {
		return nil, fmt.Errorf("wechat_virtualpay_go: PKCS#7 填充长度 %d 非法（应在 1..%d）", pad, maxPad)
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, errors.New("wechat_virtualpay_go: PKCS#7 填充字节不一致")
		}
	}
	return data[:len(data)-pad], nil
}

// pkcs7Pad 按 PKCS#7 填充到 k 的整数倍。
func pkcs7Pad(data []byte, k int) []byte {
	pad := k - len(data)%k
	out := make([]byte, 0, len(data)+pad)
	out = append(out, data...)
	for i := 0; i < pad; i++ {
		out = append(out, byte(pad))
	}
	return out
}

// aesDecrypt 解密并校验 appid，返回明文 msg。
//
// 解密后的完整结构为：
//
//	random(16B) + msg_len(4B, 网络字节序) + msg + appid
func aesDecrypt(aesKey []byte, encryptB64, expectAppID string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encryptB64)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: Encrypt 不是合法的 base64: %w", err)
	}
	plain, err := aesCBCDecrypt(aesKey, ciphertext)
	if err != nil {
		return nil, err
	}

	// 16 字节随机数 + 4 字节长度
	if len(plain) < 20 {
		return nil, fmt.Errorf("wechat_virtualpay_go: 解密结果过短（%d 字节），不可能是合法报文", len(plain))
	}
	msgLen := int(binary.BigEndian.Uint32(plain[16:20]))
	if msgLen < 0 || 20+msgLen > len(plain) {
		return nil, fmt.Errorf("wechat_virtualpay_go: 解密结果里的消息长度 %d 越界（总长 %d）", msgLen, len(plain))
	}
	msg := plain[20 : 20+msgLen]
	appID := string(plain[20+msgLen:])

	// 校验 appid：防止拿别人的报文来打自己的接口。
	if appID != expectAppID {
		return nil, fmt.Errorf("wechat_virtualpay_go: 解密结果里的 appid %q 与配置的 %q 不符", appID, expectAppID)
	}
	return msg, nil
}

// aesEncrypt 按微信的格式加密一段明文，返回 base64 后的密文。
func aesEncrypt(aesKey, appID string, msg []byte, random16 []byte) (string, error) {
	if len(random16) != 16 {
		return "", fmt.Errorf("wechat_virtualpay_go: 随机串必须为 16 字节，实际 %d 字节", len(random16))
	}
	buf := make([]byte, 0, 16+4+len(msg)+len(appID))
	buf = append(buf, random16...)
	buf = append(buf, byte(len(msg)>>24), byte(len(msg)>>16), byte(len(msg)>>8), byte(len(msg)))
	buf = append(buf, msg...)
	buf = append(buf, appID...)

	ciphertext, err := aesCBCEncrypt([]byte(aesKey), buf)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// randomBytes 生成 n 字节密码学安全随机数。
func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 生成随机数失败: %w", err)
	}
	return b, nil
}

// aesCBCDecrypt 用 AES-256-CBC 解密，IV 取密钥的前 16 字节。
func aesCBCDecrypt(key, ciphertext []byte) ([]byte, error) {
	if len(ciphertext)%aes.BlockSize != 0 || len(ciphertext) == 0 {
		return nil, errors.New("wechat_virtualpay_go: 密文长度不是 16 的整数倍")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 构造 AES cipher 失败: %w", err)
	}
	plain := make([]byte, len(ciphertext))
	// 微信规定 IV = AESKey 前 16 字节。
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(plain, ciphertext)
	return pkcs7Unpad(plain, wechatPKCS7BlockSize)
}

// aesCBCEncrypt 用 AES-256-CBC 加密，IV 取密钥的前 16 字节。
func aesCBCEncrypt(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("wechat_virtualpay_go: 构造 AES cipher 失败: %w", err)
	}
	padded := pkcs7Pad(plain, wechatPKCS7BlockSize)
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(out, padded)
	return out, nil
}
