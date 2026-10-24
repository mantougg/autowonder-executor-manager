// discovery.go：执行器归属自动发现（T08-F2）。
// list_projects → 逐空间 list_executors → 按本地 executor-id 匹配归属。
// 缓存落盘 data/platform-cache.json（冷启动免全量扫描）；权限不足空间跳过；
// 本地 id 平台不存在 → unowned 态（不算错误）。
package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// cacheFile 是落盘缓存的结构。
type cacheFile struct {
	Owner map[string]int64 `json:"owner"` // executor_id -> workspace_id
}

// loadCache 读缓存（损坏/不存在自愈为空）。
func (s *Service) loadCache() {
	if s.cachePath == "" {
		return
	}
	data, err := os.ReadFile(s.cachePath)
	if err != nil {
		return
	}
	var c cacheFile
	if json.Unmarshal(data, &c) == nil && c.Owner != nil {
		s.owner = c.Owner
	}
}

// saveCache 落盘缓存（失败只影响下次冷启动，不报错打扰）。
func (s *Service) saveCache() {
	if s.cachePath == "" || len(s.owner) == 0 {
		return
	}
	data, err := json.Marshal(cacheFile{Owner: s.owner})
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.cachePath), 0o700)
	_ = os.WriteFile(s.cachePath, data, 0o600)
}

// discover 全量发现归属：可见空间 × 空间执行器 → 重建 owner/remote 快照。
// 权限不足的空间跳过（design §7.4）；平台不可达返回错误（调用方决定重试）。
func (s *Service) discover(ctx context.Context) error {
	projects, err := s.client.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("list_projects 失败：%w", err)
	}
	owner := map[string]int64{}
	remote := map[string]ExecutorRemote{}
	for _, p := range projects {
		execs, err := s.client.ListExecutors(ctx, p.ID)
		if err != nil {
			if IsPermissionErr(err) {
				s.event("platform_skip", "", fmt.Sprintf("空间 %d（%s）无权限，跳过", p.ID, p.Name))
				continue
			}
			return fmt.Errorf("list_executors(%d) 失败：%w", p.ID, err)
		}
		for _, e := range execs {
			owner[fmt.Sprint(e.ID)] = p.ID
			remote[fmt.Sprint(e.ID)] = e
		}
	}
	s.mu.Lock()
	s.owner = owner
	s.remote = remote
	s.mu.Unlock()
	s.saveCache()
	return nil
}

// ensureDiscovery 按需触发发现：缓存未覆盖任一本地 id（或从未发现过）时
// 执行全量发现；缓存全命中则跳过（省调用，验收断言调用次数）。
func (s *Service) ensureDiscovery(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	locals := s.localIDs()
	if len(locals) == 0 {
		return nil
	}
	s.mu.RLock()
	covered := len(s.owner) > 0
	for _, l := range locals {
		if _, ok := s.owner[l.ExecutorID]; !ok {
			covered = false
			break
		}
	}
	s.mu.RUnlock()
	if covered {
		return nil
	}
	return s.discover(ctx)
}
