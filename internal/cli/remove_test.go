// remove_test.go：awem remove 验收（T07-F6）。
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awem/internal/config"
	"awem/internal/exitcode"
)

func TestRemove确认与落盘(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "rm1", Command: validCommand})
	// 拒绝 → 取消，配置保留
	e.setAnswers("n")
	if code := e.run(false, "remove", "rm1"); code != exitcode.Success {
		t.Fatalf("拒绝应取消成功：%d %s", code, e.output())
	}
	if !strings.Contains(e.output(), "已取消") {
		t.Fatalf("应提示已取消：%s", e.output())
	}
	// 确认 → 删除
	e.setAnswers("y")
	if code := e.run(false, "remove", "rm1"); code != exitcode.Success {
		t.Fatalf("确认删除失败：%d %s", code, e.output())
	}
	onDisk, _ := config.Load(e.p.ConfigFile)
	if onDisk.FindExecutor("rm1") != nil {
		t.Fatal("删除后配置不应残留")
	}
	// --yes 跳过确认
	e2 := newCLIEnv(t, config.Executor{Name: "rm2", Command: validCommand})
	if code := e2.run(false, "remove", "rm2", "--yes"); code != exitcode.Success {
		t.Fatalf("--yes 删除失败：%d %s", code, e2.output())
	}
	// 不存在 → 3
	if code := e.run(false, "remove", "nope", "--yes"); code != exitcode.NotFound {
		t.Fatalf("不存在应 3，实际 %d", code)
	}
}

func TestRemovePurge与日志保留(t *testing.T) {
	// 不带 purge：日志文件保留
	e := newCLIEnv(t, config.Executor{Name: "rm3", Command: validCommand})
	sink, _ := e.mgr.Sink("rm3")
	_, _ = sink.Write([]byte("日志行\n"))
	logPath := filepath.Join(e.p.ExecutorsLogDir, "rm3.log")
	if code := e.run(false, "remove", "rm3", "--yes"); code != exitcode.Success {
		t.Fatalf("删除失败：%d %s", code, e.output())
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("不带 purge 应保留日志：%v", err)
	}
	// 带 purge：日志删除
	e2 := newCLIEnv(t, config.Executor{Name: "rm4", Command: validCommand})
	sink2, _ := e2.mgr.Sink("rm4")
	_, _ = sink2.Write([]byte("日志行\n"))
	logPath2 := filepath.Join(e2.p.ExecutorsLogDir, "rm4.log")
	if code := e2.run(false, "remove", "rm4", "--purge", "--yes"); code != exitcode.Success {
		t.Fatalf("purge 删除失败：%d %s", code, e2.output())
	}
	if _, err := os.Stat(logPath2); !os.IsNotExist(err) {
		t.Fatalf("--purge 应删除日志，err=%v", err)
	}
}

func TestRemove运行中先停后删(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "rm5", Command: validCommand})
	if err := e.sup.StartExecutor("rm5"); err != nil {
		t.Fatal(err)
	}
	// 确认文案应说明将先停止
	e.setAnswers("y")
	if code := e.run(false, "remove", "rm5"); code != exitcode.Success {
		t.Fatalf("运行中删除失败：%d %s", code, e.output())
	}
	if !strings.Contains(e.output(), "正在运行，将先停止其进程") {
		t.Fatalf("确认文案应说明先停止：%s", e.output())
	}
	if !strings.Contains(e.output(), "已停止进程") {
		t.Fatalf("结果应报告已停止：%s", e.output())
	}
	// 进程收口断言（冒烟）：unit 状态 stopped 且无残留 pid
	if st, ok := e.sup.SnapshotOne("rm5"); ok && st.PID > 0 {
		t.Fatalf("删除后不应残留进程：%+v", st)
	}
	// 拒绝路径：运行中不确认 → 取消，进程仍在
	e2 := newCLIEnv(t, config.Executor{Name: "rm6", Command: validCommand})
	if err := e2.sup.StartExecutor("rm6"); err != nil {
		t.Fatal(err)
	}
	e2.setAnswers("n")
	if code := e2.run(false, "remove", "rm6"); code != exitcode.Success {
		t.Fatalf("拒绝应取消：%d %s", code, e2.output())
	}
	if st, _ := e2.sup.SnapshotOne("rm6"); st.State != "running" {
		t.Fatalf("取消删除后进程应仍在运行：%+v", st)
	}
}
