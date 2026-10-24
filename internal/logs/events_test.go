package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Test事件写入读回JSONL 写事件 → 读回 JSONL 逐行解析成功且字段齐。
func Test事件写入读回JSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.log")
	el, err := OpenEventLog(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	if err := el.Record(EventExecutorStart, "e1", "模型 qfmodel"); err != nil {
		t.Fatal(err)
	}
	if err := el.Record(EventLogCleanup, "", "删除 3 个过期文件"); err != nil {
		t.Fatal(err)
	}
	el.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("应有 2 行 JSONL，实际 %d", len(lines))
	}
	var ev1, ev2 Event
	if err := json.Unmarshal([]byte(lines[0]), &ev1); err != nil {
		t.Fatalf("第 1 行不是合法 JSON：%v", err)
	}
	if ev1.Type != EventExecutorStart || ev1.Executor != "e1" || ev1.Detail != "模型 qfmodel" {
		t.Fatalf("字段不符：%+v", ev1)
	}
	if !ev1.Time.After(before.Add(-time.Second)) {
		t.Fatalf("时间字段异常：%v", ev1.Time)
	}
	if err := json.Unmarshal([]byte(lines[1]), &ev2); err != nil {
		t.Fatalf("第 2 行不是合法 JSON：%v", err)
	}
	if ev2.Executor != "" || strings.Contains(lines[1], "\"executor\"") {
		t.Fatalf("空 executor 应 omitempty：%s", lines[1])
	}
}

// Test事件类型枚举覆盖 §11 场景。
func Test事件类型枚举覆盖(t *testing.T) {
	for _, want := range []string{
		"daemon_start", "daemon_stop",
		"executor_start", "executor_stop", "executor_crash", "executor_restart",
		"config_loaded", "config_reload_failed", "config_changed",
		"service_installed", "log_cleanup",
	} {
		found := false
		for _, typ := range []EventType{
			EventDaemonStart, EventDaemonStop,
			EventExecutorStart, EventExecutorStop, EventExecutorCrash, EventExecutorRestart,
			EventConfigLoaded, EventConfigReloadFailed, EventConfigChanged,
			EventServiceInstalled, EventLogCleanup,
		} {
			if string(typ) == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("缺少事件类型 %q", want)
		}
	}
}

// Test事件日志滚动复用 LogSink 参数。
func Test事件日志滚动(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.log")
	el, err := OpenEventLog(path, 120, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if err := el.Record(EventConfigChanged, "", "变更 "+strings.Repeat("x", 10)); err != nil {
			t.Fatal(err)
		}
	}
	el.Close()
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("事件日志应滚动产生 .1：%v", err)
	}
}

// TestMaskSecret 掩码策略。
func TestMaskSecret(t *testing.T) {
	cases := map[string]string{
		"":                     "***",
		"abc":                  "***",
		"abcd":                 "***",
		"abcde":                "ab***",
		"executor-token-12345": "exec***2345",
	}
	for in, want := range cases {
		if got := MaskSecret(in); got != want {
			t.Fatalf("MaskSecret(%q) = %q，期望 %q", in, got, want)
		}
	}
}
