// log_test.go：awem log 验收（T07-F7）。
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"awem/internal/config"
	"awem/internal/daemon"
	"awem/internal/exitcode"
)

// seedLog 预写 n 行日志。
func seedLog(t *testing.T, e *cliEnv, name string, n int) {
	t.Helper()
	sink, err := e.mgr.Sink(name)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		_, _ = sink.Write([]byte(fmt.Sprintf("行%d\n", i)))
	}
}

func TestLog尾部行数(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "lg1", Command: validCommand})
	seedLog(t, e, "lg1", 10)
	if code := e.run(false, "log", "lg1", "--lines=3"); code != exitcode.Success {
		t.Fatalf("log 失败：%d %s", code, e.output())
	}
	out := e.output()
	if !strings.Contains(out, "行8") || strings.Contains(out, "行7") || !strings.Contains(out, "行10") {
		t.Fatalf("tail=3 应只有最后 3 行：%s", out)
	}
	// 默认 100 行（写入 10 行全在窗口内）
	if code := e.run(false, "log", "lg1"); code != exitcode.Success {
		t.Fatalf("log 默认失败：%d", code)
	}
	if !strings.Contains(e.output(), "行10") || !strings.Contains(e.output(), "行1") {
		t.Fatal("默认 100 行应含全部 10 行")
	}
	// 非法 --lines
	if code := e.run(false, "log", "lg1", "--lines=abc"); code != exitcode.Usage {
		t.Fatalf("非法 --lines 应 2，实际 %d", code)
	}
}

func TestLog空态与不存在(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "lg2", Command: validCommand})
	if code := e.run(false, "log", "lg2"); code != exitcode.Success {
		t.Fatalf("空日志应成功：%d %s", code, e.output())
	}
	if !strings.Contains(e.output(), "暂无日志") {
		t.Fatalf("空态提示：%s", e.output())
	}
	if code := e.run(false, "log", "nope"); code != exitcode.NotFound {
		t.Fatalf("不存在应 3，实际 %d", code)
	}
}

func TestLogJSON结构化行数组(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "lg3", Command: validCommand})
	seedLog(t, e, "lg3", 3)
	if code := e.run(true, "log", "lg3", "--lines=2"); code != exitcode.Success {
		t.Fatalf("log --json 失败：%d %s", code, e.output())
	}
	var m struct {
		Lines []string `json:"lines"`
		Tail  int      `json:"tail"`
	}
	if err := json.Unmarshal([]byte(e.output()), &m); err != nil {
		t.Fatalf("非法 JSON：%v\n%s", err, e.output())
	}
	if len(m.Lines) != 2 || m.Tail != 2 {
		t.Fatalf("--json 行数组：%+v", m)
	}
}

// TestLog跟随收到新行并干净退出 -f 数据面冒烟：真 WS 客户端收到
// backlog 与持续新行，主动停止（Ctrl-C 等价）后干净返回。
func TestLog跟随收到新行并干净退出(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "lg4", Command: validCommand})
	seedLog(t, e, "lg4", 2)

	writeNew := time.AfterFunc(400*time.Millisecond, func() {
		sink, _ := e.mgr.Sink("lg4")
		_, _ = sink.Write([]byte("跟随新行X\n"))
	})
	defer writeNew.Stop()

	cl := daemon.NewClientAt(e.server.URL, "cli-test-token")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var got []string
	err := cl.FollowLogs(ctx, "lg4", 2, func(ev daemon.LogEvent) bool {
		switch ev.Type {
		case "backlog":
			got = append(got, ev.Lines...)
		case "line":
			got = append(got, ev.Text)
			if ev.Text == "跟随新行X" {
				return false // 读到目标行主动退出（Ctrl-C 等价收口）
			}
		}
		return true
	})
	if err != nil {
		t.Fatalf("FollowLogs 返回错误：%v", err)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "行2") || !strings.Contains(joined, "跟随新行X") {
		t.Fatalf("跟随应收到 backlog 与新行：%v", got)
	}
}
