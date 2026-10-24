// backoff_test.go：T05-F2 守护状态机验收（退避参数用毫秒级注入值）。
package supervisor

import (
	"strings"
	"syscall"
	"testing"
	"time"
)

// 验收：启动 → 杀掉 → 自动拉起；退避间隔按注入快参数缩短（20ms/40ms 序列）。
func TestCrashAutoRestartWithBackoff(t *testing.T) {
	e := newTestEnv(t)
	e.setCfg(t, execWith("e1", "-fake-mode=crash -fake-after=30ms -fake-exit-code=7"))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	// 崩溃两次以上：第 1 次退避 20ms、第 2 次 40ms（快参数序列）
	waitFor(t, "自动拉起 ≥2 次", func() bool {
		return e.snapshotOf(t, "e1").Restarts >= 2
	})
	st := e.snapshotOf(t, "e1")
	if st.Restarts < 2 {
		t.Fatalf("拉起次数=%d", st.Restarts)
	}
	events := e.eventsText(t)
	if !strings.Contains(events, `"executor_crash"`) || !strings.Contains(events, "exit 7") {
		t.Fatalf("崩溃事件缺失或无退出码：%s", events)
	}
	if !strings.Contains(events, `"executor_restart"`) {
		t.Fatalf("缺少自动拉起事件：%s", events)
	}
	if !strings.Contains(events, "第 1 次自动拉起（退避 20ms）") {
		t.Fatalf("第 1 次退避间隔不符（want 20ms）：%s", events)
	}
	if !strings.Contains(events, "第 2 次自动拉起（退避 40ms）") {
		t.Fatalf("第 2 次退避间隔不符（want 40ms，×2 增长）：%s", events)
	}
	_ = e.sup.StopExecutor("e1")
}

// 验收：稳定运行满 StableReset（注入 150ms，快进等价）后崩溃 → 退避从 1s
// （注入 20ms）重新起步，且崩溃事件带重置说明。
func TestStableRunResetsBackoff(t *testing.T) {
	e := newTestEnv(t)
	e.setCfg(t, execWith("e1", "-fake-mode=run"))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	pid1 := e.snapshotOf(t, "e1").PID

	// 第一次崩溃：短命（未稳定）→ step 0→1，下次退避 40ms
	syscall.Kill(pid1, syscall.SIGKILL)
	waitFor(t, "第一次自动拉起", func() bool {
		return e.snapshotOf(t, "e1").Restarts == 1
	})
	pid2 := e.snapshotOf(t, "e1").PID
	if pid2 == pid1 {
		t.Fatalf("拉起后 pid 未变化：%d", pid2)
	}
	// 新进程稳定运行 >150ms 后再崩溃 → 退避计数应重置，第 2 次拉起退避 20ms（而非 40ms）
	time.Sleep(250 * time.Millisecond)
	syscall.Kill(pid2, syscall.SIGKILL)
	waitFor(t, "第二次自动拉起", func() bool {
		return e.snapshotOf(t, "e1").Restarts == 2
	})
	events := e.eventsText(t)
	if !strings.Contains(events, "退避计数已重置") {
		t.Fatalf("稳定运行重置未记录：%s", events)
	}
	if !strings.Contains(events, "第 2 次自动拉起（退避 20ms）") {
		t.Fatalf("稳定后应从 20ms 重新起步（want 20ms not 40ms）：%s", events)
	}
	_ = e.sup.StopExecutor("e1")
}

// 验收：restart.enabled=false 崩溃后不拉起，状态 Failed。
func TestCrashNoRestartWhenDisabled(t *testing.T) {
	e := newTestEnv(t)
	ex := execWith("e1", "-fake-mode=crash -fake-after=20ms")
	ex.Restart.Enabled = new(bool) // false
	e.setCfg(t, ex)
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	waitFor(t, "崩溃后进入 Failed", func() bool {
		return e.snapshotOf(t, "e1").State == StateFailed
	})
	time.Sleep(150 * time.Millisecond) // 超过注入退避上限，确认确实不拉起
	st := e.snapshotOf(t, "e1")
	if st.State != StateFailed || st.Restarts != 0 || st.PID != 0 {
		t.Fatalf("不应拉起：state=%s restarts=%d pid=%d", st.State, st.Restarts, st.PID)
	}
	events := e.eventsText(t)
	if !strings.Contains(events, "restart.enabled=false 不拉起") {
		t.Fatalf("事件应说明不拉起原因：%s", events)
	}
}

// 验收（与 F3 联测）：人为 stop 不触发自动拉起。
func TestManualStopDoesNotRestart(t *testing.T) {
	e := newTestEnv(t)
	e.setCfg(t, execWith("e1", "-fake-mode=run"))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	if err := e.sup.StopExecutor("e1"); err != nil {
		t.Fatalf("停止失败：%v", err)
	}
	time.Sleep(200 * time.Millisecond) // 远超注入退避间隔
	st := e.snapshotOf(t, "e1")
	if st.State != StateStopped || st.Restarts != 0 {
		t.Fatalf("人为停止后不应拉起：state=%s restarts=%d", st.State, st.Restarts)
	}
	if !strings.Contains(e.eventsText(t), `"executor_stop"`) {
		t.Fatal("缺少人为停止事件")
	}
	// 再次 stop = 幂等成功
	if err := e.sup.StopExecutor("e1"); err != nil {
		t.Fatalf("重复停止应 no-op 成功：%v", err)
	}
}
