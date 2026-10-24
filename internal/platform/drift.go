// drift.go：配置漂移检测（T08-F4）。对比本地解析字段与平台
// get_executor_launch_config：不一致产出 DriftField 列表（字段/本地值/
// 平台值）写入 Status.Drift；全部一致 → 空。v1 不回写平台（design #9）。
// 权限不足或查询失败 → 该项跳过、无错误传播（保留上次 drift 或置空）。
// 频率：与轮询同周期（选定记录——漂移只在轮询时可见变化，独立更低频
// 意义不大；如后续有压力再拆）。
package platform

import (
	"context"
	"fmt"
	"strconv"
)

// driftPairs 本地字段 → 平台 LaunchConfig 字段对比清单（顺序稳定）。
var driftPairs = []struct {
	field  string // 展示名（与 CLI edit flag 一致）
	local  func(LocalExecutor) string
	remote func(LaunchConfig) string
}{
	{"model", func(l LocalExecutor) string { return l.Model }, func(c LaunchConfig) string { return c.Model }},
	{"reasoning-effort", func(l LocalExecutor) string { return l.ReasoningEffort }, func(c LaunchConfig) string { return c.ReasoningEffort }},
	{"context-window", func(l LocalExecutor) string { return l.ContextWindow },
		func(c LaunchConfig) string { return c.ContextWindow }},
	{"memory-mode", func(l LocalExecutor) string { return l.MemoryMode }, func(c LaunchConfig) string { return c.MemoryMode }},
	{"max-tasks", func(l LocalExecutor) string { return l.MaxTasks },
		func(c LaunchConfig) string {
			if c.MaxConcurrentDispatches == 0 {
				return ""
			}
			return fmt.Sprint(c.MaxConcurrentDispatches)
		}},
}

// checkDrift 对全部已归属本地执行器执行漂移检测（Refresh 尾部调用；
// ctx 已带 30s 超时）。本地值非空且与平台不一致才计漂移——本地字段
// 未配置（空串）不参与比较（无法判定意图）。
func (s *Service) checkDrift(ctx context.Context) {
	if s.localIDs == nil {
		return
	}
	drifts := map[string][]DriftField{}
	for _, l := range s.localIDs() {
		s.mu.RLock()
		ws, owned := s.owner[l.ExecutorID]
		s.mu.RUnlock()
		if !owned {
			continue
		}
		cfg, err := s.client.GetExecutorLaunchConfig(ctx, ws, parseID(l.ExecutorID))
		if err != nil {
			// 权限不足/查询失败：跳过该项（design §7.4/§7.5），不传播
			continue
		}
		var fields []DriftField
		for _, p := range driftPairs {
			lv := p.local(l)
			if lv == "" {
				continue // 本地未配置该字段：不判漂移
			}
			rv := p.remote(*cfg)
			if rv == "" {
				continue // 平台未返回该字段：不判漂移
			}
			if lv != rv {
				fields = append(fields, DriftField{Field: p.field, Local: lv, Remote: rv})
			}
		}
		drifts[l.ExecutorID] = fields
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, fs := range drifts {
		if st, ok := s.status[id]; ok {
			st.Drift = fs
			s.status[id] = st
		}
	}
}

// parseID 字符串 id → int64（解析失败返回 0，后续查询自然失败跳过）。
func parseID(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
