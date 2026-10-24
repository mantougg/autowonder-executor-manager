// list_test.go：awem list 验收（T07-F2）。
package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"awem/internal/config"
	"awem/internal/exitcode"
)

func TestList有执行器表格(t *testing.T) {
	e := newCLIEnv(t,
		config.Executor{Name: "e1", Command: validCommand, Autostart: true,
			Restart: config.RestartPolicy{Enabled: config.BoolPtr(true)}},
		config.Executor{Name: "e2", Command: strings.Replace(validCommand, "10086", "10087", 1)},
	)
	if code := e.run(false, "list"); code != exitcode.Success {
		t.Fatalf("list 失败：%d %s", code, e.output())
	}
	out := e.output()
	for _, want := range []string{"e1", "e2", "stopped", "unknown", "qfmodel", "共 2 个执行器"} {
		if !strings.Contains(out, want) {
			t.Fatalf("表格应含 %q：\n%s", want, out)
		}
	}
	// 平台/漂移 T08 前为 —
	if !strings.Contains(out, "—") {
		t.Fatalf("平台列应有占位 —：\n%s", out)
	}
}

func TestList空列表引导(t *testing.T) {
	e := newCLIEnv(t)
	if code := e.run(false, "list"); code != exitcode.Success {
		t.Fatalf("空列表也应成功：%d", code)
	}
	out := e.output()
	if !strings.Contains(out, "还没有执行器") || !strings.Contains(out, "awem add") {
		t.Fatalf("空列表应给引导：%s", out)
	}
}

func TestListJSON与APISchema一致(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "j1", Command: validCommand})
	if code := e.run(true, "list"); code != 0 {
		t.Fatalf("list --json 失败：%d %s", code, e.output())
	}
	var m struct {
		Executors []map[string]any `json:"executors"`
	}
	if err := json.Unmarshal([]byte(e.output()), &m); err != nil {
		t.Fatalf("非法 JSON：%v\n%s", err, e.output())
	}
	if len(m.Executors) != 1 {
		t.Fatalf("应 1 个执行器：%d", len(m.Executors))
	}
	for _, key := range []string{"name", "command", "autostart", "restart", "parsed", "local", "platform", "drift"} {
		if _, ok := m.Executors[0][key]; !ok {
			t.Fatalf("--json 视图缺字段 %q：%v", key, m.Executors[0])
		}
	}
}
