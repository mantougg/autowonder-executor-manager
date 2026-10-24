// autostart_test.go：T05-F4 autostart 拉起与热加载语义验收。
package supervisor

import (
	"strings"
	"testing"

	"awem/internal/config"
)

// 验收：autostart=true 在 Supervisor 应用配置（daemon 启动）时被拉起，
// autostart=false 不动。
func TestApplyConfigAutostart(t *testing.T) {
	e := newTestEnv(t)
	a := execWith("auto", "-fake-mode=run")
	a.Autostart = true
	b := execWith("manual", "-fake-mode=run") // autostart=false
	e.setCfg(t, a, b)
	waitFor(t, "auto 被拉起", func() bool {
		return e.snapshotOf(t, "auto").State == StateRunning
	})
	if st := e.snapshotOf(t, "manual"); st.State != StateStopped {
		t.Fatalf("autostart=false 不应被拉起：state=%s", st.State)
	}
	_ = e.sup.StopExecutor("auto")
}

// 验收：某条启动失败（npx 解析失败一次）不影响其余条目，逐条记事件。
func TestApplyConfigFailureIsolation(t *testing.T) {
	e := newTestEnv(t)
	calls := 0
	e.sup.opts.ResolveNpx = func() (string, error) {
		calls++
		if calls == 1 {
			return "", ErrNpxNotFound // 第一条拉起时失败，第二条应照常
		}
		return fakeNpxPath, nil
	}
	e1 := execWith("first", "-fake-mode=run")
	e1.Autostart = true
	e2 := execWith("second", "-fake-mode=run")
	e2.Autostart = true
	e.setCfg(t, e1, e2)

	waitFor(t, "second 被拉起（不受 first 失败影响）", func() bool {
		return e.snapshotOf(t, "second").State == StateRunning
	})
	st := e.snapshotOf(t, "first")
	if st.State != StateFailed {
		t.Fatalf("first 应为 failed：%s", st.State)
	}
	events := e.eventsText(t)
	if !strings.Contains(events, `"executor":"first"`) || !strings.Contains(events, "executor_spawn_fail") {
		t.Fatalf("失败条目应逐条记事件：%s", events)
	}
	_ = e.sup.StopExecutor("second")
}

// 验收：热加载语义——新增 autostart 条目即时拉起；存量未运行的不动；
// 运行中进程不因配置重放而重启（pid 不变）；消失条目被停止并移除。
func TestApplyConfigHotReload(t *testing.T) {
	e := newTestEnv(t)
	a := execWith("keep", "-fake-mode=run")
	b := execWith("addlater", "-fake-mode=run")
	b.Autostart = true
	e.setCfg(t, a) // 第一份：仅 keep（不自动拉起）
	if st := e.snapshotOf(t, "keep"); st.State != StateStopped {
		t.Fatalf("首份配置 keep 未拉起应为 stopped：%s", st.State)
	}

	// 第二份：keep 保持不拉起，addlater(autostart) 新增即拉
	e.setCfg(t, a, b)
	waitFor(t, "addlater 即时拉起", func() bool {
		return e.snapshotOf(t, "addlater").State == StateRunning
	})
	if st := e.snapshotOf(t, "keep"); st.State != StateStopped {
		t.Fatalf("存量未运行的 keep 不应被动拉起：%s", st.State)
	}
	pidBefore := e.snapshotOf(t, "addlater").PID

	// 第三份：keep 手动启动后重放配置——运行中的 addlater 不重启（pid 不变）
	if err := e.sup.StartExecutor("keep"); err != nil {
		t.Fatalf("启动 keep 失败：%v", err)
	}
	e.setCfg(t, a, b)
	waitFor(t, "keep 运行", func() bool { return e.snapshotOf(t, "keep").State == StateRunning })
	if got := e.snapshotOf(t, "addlater").PID; got != pidBefore {
		t.Fatalf("配置重放不应重启运行中的执行器：pid %d → %d", pidBefore, got)
	}

	// 第四份：移除 keep（运行中）→ 被停止并从快照消失；addlater 继续运行
	e.setCfg(t, b)
	waitFor(t, "keep 被停止移除", func() bool {
		_, ok := e.sup.SnapshotOne("keep")
		return !ok
	})
	events := e.eventsText(t)
	if !strings.Contains(events, `"executor_stop","executor":"keep"`) {
		t.Fatalf("移除运行中条目应产生停止事件：%s", events)
	}
	if st := e.snapshotOf(t, "addlater"); st.State != StateRunning {
		t.Fatalf("addlater 不应受影响：%s", st.State)
	}
	_ = e.sup.StopExecutor("addlater")
}

// 编译期确认测试配置字段（防止 execWith 变更后静默漂移）。
var _ = config.Executor{}
