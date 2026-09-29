package wechat_virtualpay

import (
	"encoding/json"
	"testing"
)

// 与官方示例逐字一致：{"ErrCode":0,"ErrMsg":"success"}
func TestAckMarshalMatchesDocExample(t *testing.T) {
	b, err := json.Marshal(Ack{ErrCode: 0, ErrMsg: "success"})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"ErrCode":0,"ErrMsg":"success"}`
	if string(b) != want {
		t.Fatalf("与官方示例不一致（got %s want %s）", b, want)
	}
}

// 零值即成功应答——这条是**刻意钉住**的：协议如此，写在这里免得有人以为它会是失败。
func TestAckZeroValueIsSuccess(t *testing.T) {
	var ack Ack
	b, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"ErrCode":0,"ErrMsg":""}`
	if string(b) != want {
		t.Fatalf("零值的序列化结果变了（got %s want %s）——这会让「忘了赋值」从成功悄悄变成别的", b, want)
	}
}

// iOS 退款问询应答：字段名/大小写与官方一致
func TestIOSRefundQueryResponseMarshal(t *testing.T) {
	b, err := json.Marshal(IOSRefundQueryResponse{
		ResultCode: 1, ResultInfo: "拒绝退款", Evidence: "订单已发货",
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"result_code":1,"result_info":"拒绝退款","evidence":"订单已发货"}`
	if string(b) != want {
		t.Fatalf("与官方字段表不一致（got %s want %s）", b, want)
	}
}
