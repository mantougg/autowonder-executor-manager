package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDebug启动事件标记 debug=true 的事件含标记；false 时无额外行为。
func TestDebug启动事件标记(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.log")
	el, err := OpenEventLog(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := el.RecordExecutorStart("e-debug", true); err != nil {
		t.Fatal(err)
	}
	if err := el.RecordExecutorStart("e-normal", false); err != nil {
		t.Fatal(err)
	}
	el.Close()

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("应有 2 条事件，实际 %d", len(lines))
	}
	if !strings.Contains(lines[0], "e-debug") || !strings.Contains(lines[0], "debug=true") {
		t.Fatalf("debug 启动事件应含标记：%s", lines[0])
	}
	if strings.Contains(lines[1], "debug=") {
		t.Fatalf("默认 false 不应有 debug 标记：%s", lines[1])
	}
	if !strings.Contains(lines[1], "e-normal") {
		t.Fatalf("正常启动事件缺失：%s", lines[1])
	}
}

// TestDebugEnabled命令查询 从命令串查询开关。
func TestDebugEnabled命令查询(t *testing.T) {
	base := "npx -y autowonder@1.0.0 connect --ws-url ws://h --token t --executor-id 1 --provider qoder"
	if !DebugEnabled(base + " --debug") {
		t.Fatal("带 --debug 应返回 true")
	}
	if DebugEnabled(base) {
		t.Fatal("不带 --debug 应返回 false")
	}
	if DebugEnabled("not a command") {
		t.Fatal("解析失败应按 false")
	}
}
