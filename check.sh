#!/usr/bin/env bash
# 本地检查脚本：跑 gofmt / vet / test，并把结果写入 check-report.txt。
#
# 为什么要写文件：Claude 运行在隔离沙箱里，无法执行你本机的 Go。
# 但共享文件夹是双向的 —— 你把结果写进这个文件，Claude 就能直接读到，
# 省去反复复制粘贴。
#
# 用法：
#     cd ~/Desktop/code/wechat-virtualpay-go
#     sh check.sh
#
# 跑完后告诉 Claude "跑完了" 即可。

cd "$(dirname "$0")" || exit 1

REPORT="check-report.txt"

{
	echo "=================== check report ==================="
	echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
	echo
	echo "=== go version ==="
	go version 2>&1 || echo "(找不到 go！)"
	echo
	echo "=== gofmt -l（列出未格式化的文件；为空即全部已格式化）==="
	gofmt -l . 2>&1
	echo
	echo "=== go vet ./... ==="
	go vet ./... 2>&1
	echo "vet 退出码: $?"
	echo
	echo "=== go test ./... ==="
	go test ./... 2>&1
	echo "test 退出码: $?"
	echo
	echo "=== go test ./... -v（详细，便于看每个用例）==="
	go test ./... -v 2>&1
	echo
	echo "=================== 结束 ==================="
} | tee "$REPORT"

echo
echo "报告已写入: $REPORT —— 告诉 Claude『跑完了』即可。"
