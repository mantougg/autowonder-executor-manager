// connstate_test.go：T05-F5 连接状态解析与快照字段验收。
package supervisor

import (
	"syscall"
	"testing"
	"time"
)

// 验收：注入模拟日志行 → 状态判定正确（关键字实标自真实 daemon）。
func TestParseConnLine(t *testing.T) {
	cases := []struct {
		line string
		want ConnState
		ok   bool
	}{
		{"2026-10-25 10:00:00.123 ws connected url=wss://a/ws/executor executorId=10086", ConnConnected, true},
		{"ws connected executorId=10086", ConnConnected, true},
		{"WS CONNECTED executorId=1", ConnConnected, true}, // 大小写不敏感
		{"ws disconnected, reconnecting executorId=10086 retryIn=15s", ConnReconnecting, true},
		{"ws disconnected before stable interval, reconnecting executorId=1 retryIn=2s", ConnReconnecting, true},
		{"runtime is restarting; retry after reconnect", ConnReconnecting, true},
		{"dispatch state=FAILED dispatchId=abc error=x", ConnUnknown, false},   // 任务日志不算连接事件
		{"Connecting to wss://a/ws/executor...", ConnUnknown, false},          // shim 启动行 ≠ 已连接
		{"Commands: 3  Status: 4", ConnUnknown, false},
		{"", ConnUnknown, false},
	}
	for _, tc := range cases {
		got, ok := ParseConnLine(tc.line)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseConnLine(%q) = (%s,%v), want (%s,%v)", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

// 验收：假进程打印真实关键字日志行 → 快照连接状态经历
// 连接 → 断线重连 的迁移；spawn 后默认 unknown。
func TestConnStateTransitionsFromLogs(t *testing.T) {
	e := newTestEnv(t)
	lines := "2026-10-25 10:00:00.001 ws connected url=wss://a/ws executorId=10086" +
		"|2026-10-25 10:00:00.500 ws disconnected, reconnecting executorId=10086 retryIn=15s"
	e.setCfg(t, execWith("e1", "-fake-mode=run -fake-lines='"+lines+"' -fake-line-delay=100ms"))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	st := e.snapshotOf(t, "e1")
	if st.Conn != ConnUnknown {
		t.Fatalf("刚启动应 unknown：%s", st.Conn)
	}
	waitFor(t, "连接状态=connected", func() bool {
		return e.snapshotOf(t, "e1").Conn == ConnConnected
	})
	waitFor(t, "连接状态=reconnecting（断线迁移）", func() bool {
		return e.snapshotOf(t, "e1").Conn == ConnReconnecting
	})
	// 日志原文落盘（关键字行进 LogSink，供 tail/Web 查看）
	found := false
	for _, l := range e.tailLog(t, "e1", 10) {
		if len(l) > 0 && l[0] == '2' && l != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("日志行未落盘：%v", e.tailLog(t, "e1", 10))
	}
	_ = e.sup.StopExecutor("e1")
}

// 验收：快照字段齐（pid/uptime/重启次数/连接状态），uptime 与重启次数随
// 冒烟过程正确变化。
func TestSnapshotFieldsChangeOverLifecycle(t *testing.T) {
	e := newTestEnv(t)
	e.setCfg(t, execWith("e1", "-fake-mode=run"))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	st1 := e.snapshotOf(t, "e1")
	time.Sleep(120 * time.Millisecond)
	st2 := e.snapshotOf(t, "e1")
	if st2.Uptime <= st1.Uptime {
		t.Fatalf("uptime 应增长：%v → %v", st1.Uptime, st2.Uptime)
	}
	if st2.PID != st1.PID || st2.Restarts != 0 {
		t.Fatalf("稳定运行 pid/重启数不应变化：%+v", st2)
	}
	// 崩溃自动拉起：重启数 +1、pid 换新、uptime 归零重算
	syscall.Kill(st2.PID, syscall.SIGKILL)
	waitFor(t, "自动拉起", func() bool {
		return e.snapshotOf(t, "e1").Restarts == 1
	})
	st3 := e.snapshotOf(t, "e1")
	if st3.PID == st2.PID {
		t.Fatalf("拉起后 pid 应变化")
	}
	if st3.Uptime > st2.Uptime {
		t.Fatalf("新进程 uptime 应从零重算：%v（旧 %v）", st3.Uptime, st2.Uptime)
	}
	if st3.Conn != ConnUnknown {
		t.Fatalf("新进程连接状态应重置 unknown：%s", st3.Conn)
	}
	_ = e.sup.StopExecutor("e1")
}
