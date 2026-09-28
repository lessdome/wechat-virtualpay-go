package wechat_virtualpay_go

import (
	"encoding/json"
	"strings"
	"testing"
)

// 第 1 项：关掉 HTML 转义、不留尾部换行、可复现。
func TestMarshalNoHTMLEscape(t *testing.T) {
	got, err := marshalNoHTMLEscape(map[string]string{"k": "a<b>c&d"})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"k":"a<b>c&d"}`
	if string(got) != want {
		t.Errorf("got %q want %q", got, want)
	}

	// 输出里不该出现任何反斜杠——出现了就说明发生了转义
	if strings.ContainsRune(string(got), rune(92)) {
		t.Errorf("输出里出现了反斜杠，说明转义没关掉: %s", got)
	}

	// 与标准库对照：json.Marshal 一定会转义，所以本包不能直接用它
	std, err := json.Marshal(map[string]string{"k": "a<b>c&d"})
	if err != nil {
		t.Fatal(err)
	}
	if string(std) == string(got) {
		t.Error("标准库输出与本函数相同，说明本函数没起作用")
	}
	if !strings.ContainsRune(string(std), rune(92)) {
		t.Errorf("标准库这次没有转义，本用例的前提不成立: %s", std)
	}
	t.Logf("本包:   %s", got)
	t.Logf("标准库: %s", std)

	// 可复现：同一输入两次结果一致
	again, _ := marshalNoHTMLEscape(map[string]string{"k": "a<b>c&d"})
	if string(again) != string(got) {
		t.Error("同一输入两次序列化结果不同")
	}
}
