// discovery_test.go：归属自动发现验收（T08-F2）。
package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newDiscoveryEnv 构造假平台 + Syncer。
type discoveryEnv struct {
	f *fakeMCP
	s *Service
}

func newDiscoveryEnv(t *testing.T, locals ...LocalExecutor) *discoveryEnv {
	t.Helper()
	f := newFakeMCP(t)
	dir := t.TempDir()
	s := NewService(Options{
		Config:    &SyncerConfig{BaseURL: f.server.URL, Token: "tok-1", Interval: time.Minute},
		CachePath: filepath.Join(dir, "platform-cache.json"),
		LocalIDs:  func() []LocalExecutor { return locals },
		OnEvent:   func(typ, executorID, detail string) {},
	})
	return &discoveryEnv{f: f, s: s}
}

// seedTwoWorkspaces 两空间：10000 有 10086/10087，10001 有 20001。
func seedTwoWorkspaces(f *fakeMCP) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tools["autowonder.list_projects"] = func(map[string]any) (string, error) {
		return `[{"workspaceId":10000,"name":"ws-a"},{"workspaceId":10001,"name":"ws-b"}]`, nil
	}
	f.tools["autowonder.list_executors"] = func(args map[string]any) (string, error) {
		ws := int64(args["workspaceId"].(float64))
		switch ws {
		case 10000:
			return `[{"id":10086,"name":"e1","status":"ONLINE"},{"id":10087,"name":"e2","status":"OFFLINE"}]`, nil
		case 10001:
			return `[{"id":20001,"name":"e3","status":"BUSY"}]`, nil
		}
		return `[]`, nil
	}
}

func Test发现多空间映射正确(t *testing.T) {
	e := newDiscoveryEnv(t,
		LocalExecutor{ExecutorID: "10086"},
		LocalExecutor{ExecutorID: "20001"},
	)
	seedTwoWorkspaces(e.f)
	if err := e.s.ensureDiscovery(context.Background()); err != nil {
		t.Fatalf("发现失败：%v", err)
	}
	e.s.mu.RLock()
	defer e.s.mu.RUnlock()
	if e.s.owner["10086"] != 10000 || e.s.owner["20001"] != 10001 {
		t.Fatalf("归属映射：%v", e.s.owner)
	}
	// 缓存落盘
	data, err := os.ReadFile(e.s.cachePath)
	if err != nil {
		t.Fatalf("缓存未落盘：%v", err)
	}
	var c cacheFile
	if json.Unmarshal(data, &c) != nil || c.Owner["10086"] != 10000 {
		t.Fatalf("缓存内容：%s", data)
	}
}

func Test发现权限不足空间跳过(t *testing.T) {
	e := newDiscoveryEnv(t, LocalExecutor{ExecutorID: "20001"})
	f := e.f
	f.mu.Lock()
	f.tools["autowonder.list_projects"] = func(map[string]any) (string, error) {
		return `[{"workspaceId":10000,"name":"ws-a"},{"workspaceId":10001,"name":"ws-b"}]`, nil
	}
	f.tools["autowonder.list_executors"] = func(args map[string]any) (string, error) {
		ws := int64(args["workspaceId"].(float64))
		if ws == 10000 {
			return "", staticErr("permission denied")
		}
		return `[{"id":20001,"name":"e3","status":"ONLINE"}]`, nil
	}
	f.mu.Unlock()

	var events []string
	e.s.onEvent = func(typ, id, detail string) { events = append(events, typ) }
	if err := e.s.ensureDiscovery(context.Background()); err != nil {
		t.Fatalf("权限不足应跳过不失败：%v", err)
	}
	e.s.mu.RLock()
	defer e.s.mu.RUnlock()
	if e.s.owner["20001"] != 10001 {
		t.Fatalf("可达空间的归属应建立：%v", e.s.owner)
	}
	if len(events) == 0 {
		t.Fatal("跳过应有事件记录")
	}
}

func Test缓存命中避免重复扫描(t *testing.T) {
	e := newDiscoveryEnv(t, LocalExecutor{ExecutorID: "10086"})
	seedTwoWorkspaces(e.f)
	if err := e.s.ensureDiscovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	n1 := e.f.callCount("autowonder.list_projects")
	if n1 != 1 {
		t.Fatalf("首次应全量扫描（list_projects 1 次），实际 %d", n1)
	}
	// 二次：缓存已覆盖本地 id → 不再扫描
	if err := e.s.ensureDiscovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n2 := e.f.callCount("autowonder.list_projects"); n2 != 1 {
		t.Fatalf("缓存命中不应重复扫描，list_projects 已调 %d 次", n2)
	}
	// 新本地 id 出现 → 重新发现
	e.s.localIDs = func() []LocalExecutor {
		return []LocalExecutor{{ExecutorID: "10086"}, {ExecutorID: "30000"}}
	}
	if err := e.s.ensureDiscovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n3 := e.f.callCount("autowonder.list_projects"); n3 != 2 {
		t.Fatalf("新增本地 id 应触发重新发现（第 2 次扫描），实际 %d", n3)
	}
}

func Test平台侧删除后重发现(t *testing.T) {
	e := newDiscoveryEnv(t, LocalExecutor{ExecutorID: "10086"})
	seedTwoWorkspaces(e.f)
	if err := e.s.ensureDiscovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 平台侧删除 10086：强制全量发现后本地 10086 应变 unowned
	e.f.mu.Lock()
	e.f.tools["autowonder.list_executors"] = func(args map[string]any) (string, error) {
		ws := int64(args["workspaceId"].(float64))
		if ws == 10000 {
			return `[{"id":10087,"name":"e2","status":"OFFLINE"}]`, nil
		}
		return `[]`, nil
	}
	e.f.mu.Unlock()
	if err := e.s.discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.s.mu.RLock()
	_, stillOwned := e.s.owner["10086"]
	e.s.mu.RUnlock()
	if stillOwned {
		t.Fatal("平台删除后归属应移除")
	}
}

func Test未配置平台静默禁用(t *testing.T) {
	dir := t.TempDir()
	s := NewService(Options{CachePath: filepath.Join(dir, "c.json")})
	if s.Enabled() {
		t.Fatal("无配置应禁用")
	}
	if err := s.ensureDiscovery(context.Background()); err != nil {
		t.Fatalf("禁用态发现应为 no-op：%v", err)
	}
	if _, ok := s.Lookup("10086"); ok {
		t.Fatal("禁用态 Lookup 恒未命中")
	}
}
