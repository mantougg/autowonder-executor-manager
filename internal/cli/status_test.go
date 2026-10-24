// status_test.go：awem status 验收（T07-F3）。
package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"awem/internal/config"
	"awem/internal/exitcode"
)

func TestStatus单个详细(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "s1", Command: validCommand})
	// 准备：直接经 supervisor 启动（测试准备不走被测命令）
	if err := e.sup.StartExecutor("s1"); err != nil {
		t.Fatalf("准备启动失败：%v", err)
	}
	if code := e.run(false, "status", "s1"); code != exitcode.Success {
		t.Fatalf("status 失败：%d %s", code, e.output())
	}
	out := e.output()
	for _, want := range []string{"执行器：s1", "运行状态：running", "pid：", "uptime：", "连接状态：", "执行器 ID：10086", "provider：qoder", "autowonder@0.3.22", "平台状态：—", "配置漂移：无"} {
		if !strings.Contains(out, want) {
			t.Fatalf("详情应含 %q：\n%s", want, out)
		}
	}
}

func TestStatus全部摘要(t *testing.T) {
	e := newCLIEnv(t,
		config.Executor{Name: "a1", Command: validCommand},
		config.Executor{Name: "a2", Command: strings.Replace(validCommand, "10086", "10088", 1)},
	)
	if code := e.run(false, "status"); code != exitcode.Success {
		t.Fatalf("status 全部失败：%d %s", code, e.output())
	}
	if !strings.Contains(e.output(), "共 2 个执行器") {
		t.Fatalf("全部摘要应复用列表形态：%s", e.output())
	}
}

func TestStatus不存在退出码3(t *testing.T) {
	e := newCLIEnv(t)
	if code := e.run(false, "status", "nope"); code != exitcode.NotFound {
		t.Fatalf("不存在应退出码 3，实际 %d：%s", code, e.output())
	}
	if code := e.run(true, "status", "nope"); code != exitcode.NotFound {
		t.Fatalf("--json 同样 3，实际 %d", code)
	}
}

func TestStatusJSON字段覆盖设计8_1(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "js", Command: validCommand})
	if code := e.run(true, "status", "js"); code != 0 {
		t.Fatalf("status --json 失败：%d %s", code, e.output())
	}
	var m struct {
		Executor struct {
			Local struct {
				State     string `json:"state"`
				PID       int    `json:"pid"`
				StartedAt string `json:"started_at"`
				UptimeMS  int64  `json:"uptime_ms"`
				Restarts  int    `json:"restarts"`
				Conn      string `json:"conn"`
			} `json:"local"`
			Parsed *struct {
				Model string `json:"model"`
			} `json:"parsed"`
			Platform any `json:"platform"`
			Drift    any `json:"drift"`
		} `json:"executor"`
	}
	if err := json.Unmarshal([]byte(e.output()), &m); err != nil {
		t.Fatalf("非法 JSON：%v\n%s", err, e.output())
	}
	l := m.Executor.Local
	if l.State == "" || l.Conn == "" || m.Executor.Parsed == nil {
		t.Fatalf("--json 应覆盖运行态/连接/模型等全部信息项：%+v", m.Executor)
	}
}
