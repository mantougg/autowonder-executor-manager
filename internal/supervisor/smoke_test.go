// smoke_test.go：T05-F7 假长驻子进程冒烟套件（`go test ./internal/supervisor/... -run Smoke`）。
// 串联：autostart 拉起 → 日志收割 → 崩溃自动拉起 → 稳定重置 → 优雅停 →
// 强杀路径 → 事件序列 → 进程树无残留/无僵尸清扫断言。作为 T06/T10 回归基线。
// 全程假 npx（注入 ResolveNpx），无真实网络/npx 依赖。
package supervisor

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// directChildrenEmpty 断言测试进程无未收割的直接子进程（防僵尸）。
func directChildrenEmpty(t *testing.T) {
	t.Helper()
	path := filepath.Join("/proc", strconv.Itoa(os.Getpid()), "task", strconv.Itoa(os.Getpid()), "children")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("非 Linux 或无 /proc，跳过僵尸断言：%v", err)
	}
	if s := strings.TrimSpace(string(data)); s != "" {
		t.Fatalf("存在未收割的子进程（僵尸或泄漏）：%s", s)
	}
}

// TestSmokeFullLifecycle 一条龙冒烟：守护语义 + 日志/事件 + 进程清理。
func TestSmokeFullLifecycle(t *testing.T) {
	e := newTestEnv(t)
	e.sup.opts.Backoff = BackoffParams{
		Initial: 20 * time.Millisecond, Max: 80 * time.Millisecond,
		StableReset: 150 * time.Millisecond, Grace: 400 * time.Millisecond,
	}
	var pgids []int
	remember := func(pid int) { pgids = append(pgids, pid) }

	// ① autostart 拉起 + 日志收割 + 连接状态解析
	connLine := "2026-10-25 10:00:00.001 ws connected url=wss://a/ws executorId=10086"
	a := execWith("smoke", "-fake-mode=run -fake-lines='"+connLine+"'")
	a.Autostart = true
	e.setCfg(t, a)
	waitFor(t, "autostart 拉起", func() bool { return e.snapshotOf(t, "smoke").State == StateRunning })
	pid1 := e.snapshotOf(t, "smoke").PID
	remember(pid1)
	waitFor(t, "连接行落盘", func() bool {
		for _, l := range e.tailLog(t, "smoke", 10) {
			if strings.Contains(l, "ws connected") {
				return true
			}
		}
		return false
	})
	waitFor(t, "conn=connected", func() bool { return e.snapshotOf(t, "smoke").Conn == ConnConnected })

	// ② 外部击杀（崩溃）→ 自动拉起，新进程 pid 变化、连接状态重置
	syscall.Kill(pid1, syscall.SIGKILL)
	waitFor(t, "自动拉起且 running", func() bool {
		st := e.snapshotOf(t, "smoke")
		return st.Restarts == 1 && st.State == StateRunning
	})
	pid2 := e.snapshotOf(t, "smoke").PID
	if pid2 == pid1 {
		t.Fatalf("拉起后 pid 未变化")
	}
	remember(pid2)

	// ③ 稳定运行 >150ms 后再崩溃 → 退避计数重置（事件佐证）
	time.Sleep(250 * time.Millisecond)
	syscall.Kill(pid2, syscall.SIGKILL)
	waitFor(t, "第二次拉起", func() bool { return e.snapshotOf(t, "smoke").Restarts == 2 })
	if !strings.Contains(e.eventsText(t), "退避计数已重置") {
		t.Fatalf("稳定重置事件缺失")
	}
	pid3 := e.snapshotOf(t, "smoke").PID
	remember(pid3)

	// ④ 人为停止：优雅退出、进程组清空、人为停止不拉起
	if err := e.sup.StopExecutor("smoke"); err != nil {
		t.Fatalf("停止失败：%v", err)
	}
	waitFor(t, "进程组清空", func() bool { return pgidEmpty(pid3) })
	time.Sleep(150 * time.Millisecond)
	if st := e.snapshotOf(t, "smoke"); st.State != StateStopped || st.Restarts != 2 {
		t.Fatalf("停止后不应再拉起：state=%s restarts=%d", st.State, st.Restarts)
	}

	// ⑤ 强杀路径：忽略 SIGTERM 的假进程 + 短宽限
	ready := filepath.Join(e.dir, "ready2")
	b := execWith("tank", "-fake-mode=ignore-term -fake-ready-file="+ready)
	e.setCfg(t, a, b)
	if err := e.sup.StartExecutor("tank"); err != nil {
		t.Fatalf("启动 tank 失败：%v", err)
	}
	pid4 := e.snapshotOf(t, "tank").PID
	remember(pid4)
	waitFor(t, "tank 就绪", func() bool { _, err := os.Stat(ready); return err == nil })
	start := time.Now()
	if err := e.sup.StopExecutor("tank"); err != nil {
		t.Fatalf("停止 tank 失败：%v", err)
	}
	if elapsed := time.Since(start); elapsed < 350*time.Millisecond {
		t.Fatalf("未等满宽限即返回（%s）", elapsed)
	}
	waitFor(t, "tank 进程组清空", func() bool { return pgidEmpty(pid4) })
	if !strings.Contains(e.eventsText(t), "宽限超时强杀") {
		t.Fatalf("缺强杀事件")
	}

	// ⑥ 事件序列：smoke 的 start < crash < restart < stop 行序
	var seq []string
	for _, ev := range e.readEvents(t) {
		if ev.Executor == "smoke" {
			seq = append(seq, ev.Type)
		}
	}
	joined := strings.Join(seq, ",")
	for _, want := range []string{"executor_start", "executor_crash", "executor_restart", "executor_stop"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("事件序列缺 %s：%v", want, seq)
		}
	}
	if !(strings.Index(joined, "executor_start") < strings.Index(joined, "executor_crash") &&
		strings.Index(joined, "executor_crash") < strings.Index(joined, "executor_restart") &&
		strings.Index(joined, "executor_restart") < strings.Index(joined, "executor_stop")) {
		t.Fatalf("事件顺序不合理：%v", seq)
	}

	// ⑦ 收尾清扫：Shutdown 干净返回 + 全部历史 pgid 为空 + 无未收割子进程
	e.sup.Shutdown()
	for _, pg := range pgids {
		if !pgidEmpty(pg) {
			t.Fatalf("进程组 %d 仍有残留进程", pg)
		}
	}
	directChildrenEmpty(t)
}

// TestSmokeTreeKill 进程树终止冒烟（T06/T10 回归基线）：父起子进程，
// Shutdown 后整树清空。
func TestSmokeTreeKill(t *testing.T) {
	e := newTestEnv(t)
	childPid := filepath.Join(e.dir, "c.pid")
	a := execWith("tree1", "-fake-mode=tree -fake-child-pidfile="+childPid)
	a.Autostart = true
	e.setCfg(t, a)
	waitFor(t, "子进程就位", func() bool {
		if data, err := os.ReadFile(childPid); err == nil {
			return atoiSafe(string(data)) > 0
		}
		return false
	})
	var kid int
	if data, err := os.ReadFile(childPid); err == nil {
		kid = atoiSafe(string(data))
	}
	pgid := e.snapshotOf(t, "tree1").PID
	e.sup.Shutdown()
	waitFor(t, "子进程退出", func() bool { return !pidAlive(kid) })
	waitFor(t, "进程组清空", func() bool { return pgidEmpty(pgid) })
	directChildrenEmpty(t)
}
