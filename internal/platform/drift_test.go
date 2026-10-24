// drift_test.go：配置漂移检测验收（T08-F4）。
package platform

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestDrift字段不一致与一致(t *testing.T) {
	f := newFakeMCP(t)
	seedTwoWorkspaces(f)
	// 平台配置：model 与本地不同、reasoning-effort 相同、context-window 不同
	f.mu.Lock()
	f.tools["autowonder.get_executor_launch_config"] = func(args map[string]any) (string, error) {
		return `{"model":"dfmodel","reasoningEffort":"medium","contextWindow":400000,"memoryMode":"provider-local","maxConcurrentDispatches":5}`, nil
	}
	f.mu.Unlock()
	s := NewService(Options{
		Config: &SyncerConfig{BaseURL: f.server.URL, Token: "tok-1", Interval: time.Minute},
		LocalIDs: func() []LocalExecutor {
			return []LocalExecutor{{
				ExecutorID: "10086", Model: "qfmodel", ReasoningEffort: "medium",
				ContextWindow: "1000000",
			}}
		},
		CachePath: filepath.Join(t.TempDir(), "c.json"),
	})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, ok := s.Lookup("10086")
	if !ok {
		t.Fatal("应有联动状态")
	}
	if len(st.Drift) != 2 {
		t.Fatalf("应恰有 model 与 context-window 两处漂移：%+v", st.Drift)
	}
	byField := map[string]DriftField{}
	for _, d := range st.Drift {
		byField[d.Field] = d
	}
	if d := byField["model"]; d.Local != "qfmodel" || d.Remote != "dfmodel" {
		t.Fatalf("model 漂移双侧值：%+v", d)
	}
	if d := byField["context-window"]; d.Local != "1000000" || d.Remote != "400000" {
		t.Fatalf("context-window 漂移双侧值：%+v", d)
	}
	if _, has := byField["reasoning-effort"]; has {
		t.Fatal("一致字段不应计入漂移")
	}

	// 全部一致 → 无 drift
	f.mu.Lock()
	f.tools["autowonder.get_executor_launch_config"] = func(args map[string]any) (string, error) {
		return `{"model":"qfmodel","reasoningEffort":"medium","contextWindow":1000000}`, nil
	}
	f.mu.Unlock()
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Lookup("10086")
	if len(st.Drift) != 0 {
		t.Fatalf("全部一致应无漂移：%+v", st.Drift)
	}
}

func TestDrift查询失败静默跳过(t *testing.T) {
	f := newFakeMCP(t)
	seedTwoWorkspaces(f)
	var called bool
	f.mu.Lock()
	f.tools["autowonder.get_executor_launch_config"] = func(args map[string]any) (string, error) {
		called = true
		return "", staticErr("permission denied") // 权限不足
	}
	f.mu.Unlock()
	s := NewService(Options{
		Config: &SyncerConfig{BaseURL: f.server.URL, Token: "tok-1", Interval: time.Minute},
		LocalIDs: func() []LocalExecutor {
			return []LocalExecutor{{ExecutorID: "10086", Model: "qfmodel"}}
		},
		CachePath: filepath.Join(t.TempDir(), "c.json"),
	})
	// Refresh 不因漂移查询失败而报错（状态照常更新）
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("权限不足应静默跳过：%v", err)
	}
	if !called {
		t.Fatal("漂移查询应已尝试")
	}
	st, _ := s.Lookup("10086")
	if st.State != "ONLINE" || len(st.Drift) != 0 {
		t.Fatalf("状态应正常且无漂移：%+v", st)
	}
}

func TestDrift未归属不查询(t *testing.T) {
	f := newFakeMCP(t)
	seedTwoWorkspaces(f)
	f.mu.Lock()
	f.tools["autowonder.get_executor_launch_config"] = func(args map[string]any) (string, error) {
		t.Error("未归属执行器不应触发 launch_config 查询")
		return `{}`, nil
	}
	f.mu.Unlock()
	s := NewService(Options{
		Config: &SyncerConfig{BaseURL: f.server.URL, Token: "tok-1", Interval: time.Minute},
		LocalIDs: func() []LocalExecutor {
			return []LocalExecutor{{ExecutorID: "99999", Model: "qfmodel"}} // 平台不存在
		},
		CachePath: filepath.Join(t.TempDir(), "c.json"),
	})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Lookup("99999")
	if st.State != "unowned" {
		t.Fatalf("应为 unowned：%+v", st)
	}
}
