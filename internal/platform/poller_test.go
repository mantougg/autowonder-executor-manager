// poller_test.go：状态轮询验收（T08-F3）。
package platform

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newPollerEnv(t *testing.T, locals ...LocalExecutor) *discoveryEnv {
	t.Helper()
	f := newFakeMCP(t)
	s := NewService(Options{
		Config:    &SyncerConfig{BaseURL: f.server.URL, Token: "tok-1", Interval: 50 * time.Millisecond},
		CachePath: filepath.Join(t.TempDir(), "c.json"),
		LocalIDs:  func() []LocalExecutor { return locals },
	})
	return &discoveryEnv{f: f, s: s}
}

func Test轮询状态变化反映到Lookup(t *testing.T) {
	e := newPollerEnv(t, LocalExecutor{ExecutorID: "10086"})
	seedTwoWorkspaces(e.f) // 10086 ONLINE
	if err := e.s.Refresh(context.Background()); err != nil {
		t.Fatalf("刷新失败：%v", err)
	}
	st, ok := e.s.Lookup("10086")
	if !ok || st.State != "ONLINE" {
		t.Fatalf("应查到 ONLINE：%+v ok=%v", st, ok)
	}
	// 平台侧状态变化 → 下轮反映
	e.f.mu.Lock()
	e.f.tools["autowonder.list_executors"] = func(args map[string]any) (string, error) {
		return `[{"id":10086,"name":"e1","status":"BUSY","lastHeartbeatAt":"2026-10-25T03:00:00+08:00"}]`, nil
	}
	e.f.mu.Unlock()
	if err := e.s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, _ = e.s.Lookup("10086")
	if st.State != "BUSY" || st.LastHeartbeat.IsZero() {
		t.Fatalf("状态应更新为 BUSY 且带心跳：%+v", st)
	}
	// 未归属本地 id → unowned（不算错误）
	e.s.localIDs = func() []LocalExecutor {
		return []LocalExecutor{{ExecutorID: "10086"}, {ExecutorID: "99999"}}
	}
	if err := e.s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st, ok := e.s.Lookup("99999"); !ok || st.State != "unowned" {
		t.Fatalf("未归属应为 unowned：%+v ok=%v", st, ok)
	}
}

func Test轮询不可达与恢复(t *testing.T) {
	e := newPollerEnv(t, LocalExecutor{ExecutorID: "10086"})
	seedTwoWorkspaces(e.f)
	if err := e.s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 平台挂：全部请求 500
	e.f.mu.Lock()
	e.f.slow = false
	e.f.tools = map[string]func(map[string]any) (string, error){} // 所有工具不存在 → 一般错误
	e.f.mu.Unlock()
	err := e.s.Refresh(context.Background())
	if err == nil {
		t.Fatal("平台异常应返回错误")
	}
	st, ok := e.s.Lookup("10086")
	if !ok || st.State != "unreachable" {
		t.Fatalf("失败后应为 unreachable：%+v", st)
	}
	if ps := e.s.State(); ps.FailCount != 1 {
		t.Fatalf("失败计数：%+v", ps)
	}
	// 恢复：状态自动回来
	seedTwoWorkspaces(e.f)
	if err := e.s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, _ = e.s.Lookup("10086")
	if st.State != "ONLINE" {
		t.Fatalf("恢复后应回到 ONLINE：%+v", st)
	}
	if ps := e.s.State(); ps.FailCount != 0 {
		t.Fatalf("成功后失败计数应清零：%+v", ps)
	}
}

func Test轮询循环Start与周期刷新(t *testing.T) {
	e := newPollerEnv(t, LocalExecutor{ExecutorID: "10086"})
	seedTwoWorkspaces(e.f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { e.s.Start(ctx); close(done) }()
	// 首轮立即 + 周期 50ms：200ms 内应有多次 list_executors 调用
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if e.f.callCount("autowonder.list_executors") >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := e.f.callCount("autowonder.list_executors"); n < 3 {
		t.Fatalf("周期轮询未发生（仅 %d 次）", n)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start 未随 ctx 退出")
	}
}

func Test未配置平台轮询完全不启动(t *testing.T) {
	s := NewService(Options{})
	done := make(chan struct{})
	go func() { s.Start(context.Background()); close(done) }()
	select {
	case <-done:
		// 立即返回 = 未启动循环
	case <-time.After(300 * time.Millisecond):
		t.Fatal("未配置时 Start 应立即返回（不启动轮询）")
	}
}
