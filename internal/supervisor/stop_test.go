// stop_test.go：T05-F3 停止与进程树终止验收。
package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// pgidEmpty 报告进程组内是否已无任何进程（信号 0 探测，ESRCH=组空）。
func pgidEmpty(pgid int) bool {
	return pgid > 0 && syscall.Kill(-pgid, 0) == syscall.ESRCH
}

// 验收：假进程树（父再起子进程）stop 后无残留进程（pgid 遍历断言），
// 优雅期退出则不强杀（事件区分）。
func TestStopTreeNoResidueGraceful(t *testing.T) {
	e := newTestEnv(t)
	childPidFile := filepath.Join(e.dir, "child.pid")
	e.setCfg(t, execWith("e1", "-fake-mode=tree -fake-child-pidfile="+childPidFile))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	pgid := e.snapshotOf(t, "e1").PID // Setpgid：组长 pid = pgid

	// 等子进程就位（进程树成形）
	var childPid int
	waitFor(t, "子进程 pid 文件出现", func() bool {
		if data, err := os.ReadFile(childPidFile); err == nil {
			childPid = atoiSafe(string(data))
			return childPid > 0
		}
		return false
	})
	if !pidAlive(childPid) {
		t.Fatalf("子进程 %d 未存活", childPid)
	}

	if err := e.sup.StopExecutor("e1"); err != nil {
		t.Fatalf("停止失败：%v", err)
	}
	st := e.snapshotOf(t, "e1")
	if st.State != StateStopped || st.PID != 0 {
		t.Fatalf("停止后状态不符：state=%s pid=%d", st.State, st.PID)
	}
	// 整树无残留：父与子都死，进程组为空
	waitFor(t, "子进程退出", func() bool { return !pidAlive(childPid) })
	waitFor(t, "进程组清空", func() bool { return pgidEmpty(pgid) })
	events := e.eventsText(t)
	if !strings.Contains(events, "人为停止（优雅退出") {
		t.Fatalf("应为优雅退出事件：%s", events)
	}
	if strings.Contains(events, "强杀") {
		t.Fatalf("优雅期内退出不应强杀：%s", events)
	}
}

// 验收：强杀路径——忽略 SIGTERM 的假进程在宽限期后被 SIGKILL 整组终止，
// 事件明确记录强杀。
func TestStopForcedKillAfterGrace(t *testing.T) {
	e := newTestEnv(t)
	e.sup.opts.Backoff.Grace = 300 * time.Millisecond // 注入短宽限
	readyFile := filepath.Join(e.dir, "ready")
	e.setCfg(t, execWith("e1", "-fake-mode=ignore-term -fake-ready-file="+readyFile))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	pgid := e.snapshotOf(t, "e1").PID
	// 等 SIGTERM 忽略生效（就绪文件），避免信号落在 exec 初始化窗口
	waitFor(t, "ignore-term 就绪", func() bool {
		_, err := os.Stat(readyFile)
		return err == nil
	})
	start := time.Now()
	if err := e.sup.StopExecutor("e1"); err != nil {
		t.Fatalf("停止失败：%v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 250*time.Millisecond {
		t.Fatalf("未等待宽限即强杀（elapsed=%s），宽限语义失效", elapsed)
	}
	waitFor(t, "进程组清空", func() bool { return pgidEmpty(pgid) })
	st := e.snapshotOf(t, "e1")
	if st.State != StateStopped {
		t.Fatalf("停止后状态=%s", st.State)
	}
	events := e.eventsText(t)
	if !strings.Contains(events, "宽限超时强杀") {
		t.Fatalf("应记录强杀事件：%s", events)
	}
}

// 验收：已停止再 stop = no-op 成功（幂等）；从未启动的执行器 stop 也成功。
func TestStopIdempotent(t *testing.T) {
	e := newTestEnv(t)
	ex := execWith("never", "-fake-mode=run")
	e.setCfg(t, ex)
	// 从未启动：no-op 成功
	if err := e.sup.StopExecutor("never"); err != nil {
		t.Fatalf("未启动的执行器 stop 应成功：%v", err)
	}
	if err := e.sup.StartExecutor("never"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	for i := 0; i < 3; i++ {
		if err := e.sup.StopExecutor("never"); err != nil {
			t.Fatalf("第 %d 次停止失败：%v", i+1, err)
		}
	}
	if st := e.snapshotOf(t, "never"); st.State != StateStopped {
		t.Fatalf("状态=%s", st.State)
	}
	// 停止未知执行器 → 未找到错误
	if err := e.sup.StopExecutor("ghost"); err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("未知执行器应报未找到：%v", err)
	}
}

// atoiSafe 宽松解析整数（pid 文件内容）。
func atoiSafe(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
