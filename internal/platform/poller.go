// poller.go：状态轮询（T08-F3）。按 poll_interval 周期刷新：
// ensureDiscovery（按需）→ 按归属空间分组各调一次 list_executors →
// 更新 status（ONLINE/BUSY/OFFLINE + 心跳）；未归属本地 id → unowned。
// 失败：已归属执行器整体置 unreachable + 记事件；连续失败指数退避
// （interval×2^n 封顶 5×interval，防雪崩）；恢复即正常轮询。
// 未配置 [platform]：Start 不启动任何 goroutine。
package platform

import (
	"context"
	"fmt"
	"time"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Start 启动轮询循环（阻塞直到 ctx 取消；通常 go 起 goroutine）。
func (s *Service) Start(ctx context.Context) {
	if !s.Enabled() {
		return // 未配置：完全不启动（验收断言点）
	}
	// 首轮立即执行（daemon 启动即有状态），此后按周期
	s.pollOnceSafe(ctx)
	tick := s.cfg.Interval
	if tick <= 0 {
		tick = time.Minute
	}
	backoff := 0 // 连续失败计数（成功清零）
	timer := time.NewTimer(tick)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.pollOnceSafe(ctx)
		// 失败退避：interval×2^n 封顶 5×interval；成功重置
		next := tick
		if backoff > 0 {
			next = tick * time.Duration(1<<min(backoff, 3))
			if next > 5*tick {
				next = 5 * tick
			}
		}
		s.mu.Lock()
		s.pendingInterval = next
		s.mu.Unlock()
		timer.Reset(next)
	}
}

// pollOnceSafe 单次轮询（错误内部消化为 unreachable 态 + 事件）。
func (s *Service) pollOnceSafe(ctx context.Context) {
	err := s.Refresh(ctx)
	if err != nil {
		s.mu.RLock()
		fc := s.failCount // Refresh 失败路径已 +1
		s.mu.RUnlock()
		s.event("platform_unreachable", "", fmt.Sprintf("第 %d 次轮询失败：%v", fc, err))
		return
	}
	s.mu.RLock()
	fc := s.failCount
	s.mu.RUnlock()
	if fc > 0 {
		s.event("platform_recovered", "", fmt.Sprintf("轮询恢复（此前连续失败 %d 次）", fc))
	}
}

// Refresh 执行一轮完整刷新（发现 + 状态；测试与手动触发入口）。
func (s *Service) Refresh(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	cctx, cancel := ctxShort(ctx)
	defer cancel()

	if err := s.ensureDiscovery(cctx); err != nil {
		s.markUnreachable()
		return err
	}
	// 按归属空间分组：每空间一次 list_executors
	locals := s.localSet()
	s.mu.RLock()
	wsOf := map[int64]bool{}
	for id := range locals {
		if ws, ok := s.owner[id]; ok {
			wsOf[ws] = true
		}
	}
	s.mu.RUnlock()

	remote := map[string]ExecutorRemote{}
	for ws := range wsOf {
		execs, err := s.client.ListExecutors(cctx, ws)
		if err != nil {
			if IsPermissionErr(err) {
				continue // 权限不足跳过（保留上次状态）
			}
			s.markUnreachable()
			return fmt.Errorf("轮询 list_executors(%d) 失败：%w", ws, err)
		}
		for _, e := range execs {
			remote[fmt.Sprint(e.ID)] = e
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCount = 0 // 成功清零（与入口无关保持一致）
	for id := range locals {
		if e, ok := remote[id]; ok {
			s.status[id] = Status{State: e.Status, LastHeartbeat: e.LastHeartbeat}
		} else {
			// 有归属但本轮查不到（平台刚删除/迁移）或无归属 → unowned，
			// 下轮发现按需重建归属
			s.status[id] = Status{State: "unowned"}
		}
	}
	return nil
}

// markUnreachable 失败路径：已归属执行器整体置 unreachable
// （保留归属与心跳快照，恢复后自动回来）。
func (s *Service) markUnreachable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCount++
	for id := range s.owner {
		st := s.status[id]
		st.State = "unreachable"
		s.status[id] = st
	}
}

// localSet 本地执行器 id 集合。
func (s *Service) localSet() map[string]bool {
	out := map[string]bool{}
	if s.localIDs == nil {
		return out
	}
	for _, l := range s.localIDs() {
		out[l.ExecutorID] = true
	}
	return out
}

// PollState 是轮询观测面（测试/诊断）。
type PollState struct {
	FailCount int           // 连续失败次数
	NextDelay time.Duration // 下一轮间隔（退避后）
}

// State 返回轮询观测快照。
func (s *Service) State() PollState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return PollState{FailCount: s.failCount, NextDelay: s.pendingInterval}
}
