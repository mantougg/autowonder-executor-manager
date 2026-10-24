// syncer.go：平台联动器骨架（T08-F2 起）。实现 T06 预留的 api 层
// platform.Syncer 接口；内部聚合归属发现（F2）、状态轮询（F3）与漂移
// 检测（F4）。未配置 [platform] 段 → 不启动、Lookup 恒未命中（静默降级，
// design §7.2"不许报错打扰"）。
package platform

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// min 返回较小值（退避封顶用）。

// DriftField 是一个漂移字段（本地 vs 平台）。
type DriftField struct {
	Field  string `json:"field"`  // 字段名（model/reasoning-effort/…）
	Local  string `json:"local"`  // 本地值
	Remote string `json:"remote"` // 平台值
}

// Status 是单个执行器的平台侧联动快照（状态 + 漂移 + 归属展示信息）。
// State 取值：ONLINE / BUSY / OFFLINE / unreachable（平台不可达）/
// unowned（本地 id 平台不存在，不算错误）。
// Name/AgentName/Project/Squad 来自平台 list_executors（执行器中文名、
// 所属数字工人中文名、所属项目名、所属小队名），供 Web/CLI 展示。
type Status struct {
	State         string
	LastHeartbeat time.Time
	Drift         []DriftField
	Name          string   // 平台侧执行器名（接入时的中文名，如 NO1快速全栈开发）
	AgentName     string   // 所属数字工人中文名（如 快速全栈开发）
	Project       string   // 所属项目（工作空间）名
	Squad         string   // 所属小队名（多个顿号分隔）
	SquadNames    []string // 所属小队数组（树形分组用；空 = 无小队）
}

// Syncer 接口是 api 层消费面（T06 预留；Noop 为禁用实现）。
type Syncer interface {
	Lookup(executorID string) (Status, bool)
}

// LocalExecutor 是本地执行器的联动相关字段（daemon 从 config 经 T03
// 解析注入；漂移检测的本地侧）。
type LocalExecutor struct {
	ExecutorID      string
	Model           string
	ReasoningEffort string
	ContextWindow   string
	MemoryMode      string
	MaxTasks        string
}

// SyncerConfig 是联动配置（由 config.PlatformConfig 转换；nil = 禁用）。
type SyncerConfig struct {
	BaseURL  string
	Token    string
	Interval time.Duration
}

// Options 是 NewService 的依赖注入。
type Options struct {
	Config    *SyncerConfig                        // nil = 联动禁用
	CachePath string                               // 归属缓存落盘路径（空=纯内存）
	LocalIDs  func() []LocalExecutor               // 本地执行器集合（发现与漂移的本地侧）
	OnEvent   func(typ, executorID, detail string) // 事件回调（daemon 接事件日志）
}

// Service 聚合联动状态；并发安全（Syncer 接口的实现）。
type Service struct {
	cfg       *SyncerConfig
	client    *Client
	cachePath string
	localIDs  func() []LocalExecutor
	onEvent   func(typ, executorID, detail string)

	mu       sync.RWMutex
	owner    map[string]int64          // executor_id(字符串) -> workspace_id
	projects map[int64]string          // workspace_id -> 项目名（list_projects）
	remote   map[string]ExecutorRemote // executor_id -> 最近平台侧信息
	status   map[string]Status         // executor_id -> 联动快照（含 unreachable/unowned）

	// 轮询状态（poller.go）
	failCount       int           // 连续失败次数（成功清零）
	pendingInterval time.Duration // 下一轮间隔（失败退避后）
	lastOK          *bool         // 最近一次轮询成败（nil=尚未轮询）
}

// NewService 创建联动器（禁用时等价 Noop；不启动轮询，Start 由 daemon 调）。
func NewService(o Options) *Service {
	s := &Service{
		cachePath: o.CachePath,
		localIDs:  o.LocalIDs,
		onEvent:   o.OnEvent,
		owner:     map[string]int64{},
		projects:  map[int64]string{},
		remote:    map[string]ExecutorRemote{},
		status:    map[string]Status{},
	}
	if o.Config == nil {
		return s
	}
	s.cfg = o.Config
	// 端点推导（真实平台实测）：令牌编在路径中，形如
	// <base_url>/api/mcp/<mcp_token>/；Bearer 头平台兼容，由客户端附带。
	base := strings.TrimRight(o.Config.BaseURL, "/")
	s.client = NewClient(base+"/api/mcp/"+o.Config.Token+"/", o.Config.Token)
	s.loadCache()
	return s
}

// Lookup 实现 api 消费面：本地 id 有联动数据即返回（含 unreachable/unowned 态）。
func (s *Service) Lookup(executorID string) (Status, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.status[executorID]
	return st, ok
}

// event 上报联动事件（回调缺省丢弃）。
func (s *Service) event(typ, executorID, detail string) {
	if s.onEvent != nil {
		s.onEvent(typ, executorID, detail)
	}
}

// Enabled 报告联动是否启用。
func (s *Service) Enabled() bool { return s.cfg != nil && s.client != nil }

// ctxShort 单次刷新上下文（防止单次调用拖垮轮询周期）。
func ctxShort(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 30*time.Second)
}

// Summary 返回联动健康摘要（daemon 顶栏/doctor 数据源）：
// configured=false 表示未配置 [platform]（联动禁用）；configured=true 时
// connected 指示最近一次轮询成败（nil=尚未轮询过）。
func (s *Service) Summary() (configured bool, connected *bool) {
	if !s.Enabled() {
		return false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastOK == nil {
		return true, nil
	}
	return true, s.lastOK
}

// Ping 用当前客户端调一次 list_projects（doctor 平台连通检查用；
// 不修改任何缓存状态）。成功返回 nil。
func (s *Service) Ping(ctx context.Context) error {
	if !s.Enabled() {
		return errNotConfigured
	}
	cctx, cancel := ctxShort(ctx)
	defer cancel()
	_, err := s.client.ListProjects(cctx)
	return err
}

// errNotConfigured 表示平台联动未配置。
var errNotConfigured = fmt.Errorf("未配置 [platform] 段")
