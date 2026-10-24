// platform.go：平台状态联动的对外接口（T06 由 daemon 装配空实现预留，
// T08 实现 MCP 只读轮询）。接口保持极简：API/CLI 只需要"按 executor-id
// 查平台侧状态"，联动细节（发现/轮询/漂移）全部封装在实现内部。
package platform

import "time"

// Status 是单个执行器的平台侧状态快照（与本地进程状态并列展示，design §6）。
type Status struct {
	State         string     // ONLINE / BUSY / OFFLINE（空=未知/未联动）
	LastHeartbeat time.Time // 平台侧最近心跳（零值=未知）
}

// Syncer 供给 API/CLI 的平台状态查询接口。
// Lookup 按 executor-id 查最近一次轮询的状态；未联动或未匹配到时 ok=false。
type Syncer interface {
	Lookup(executorID string) (Status, bool)
}

// Noop 是 T08 之前的空实现：一律"未联动"。
type Noop struct{}

// Lookup 实现 Syncer：恒返回未匹配。
func (Noop) Lookup(string) (Status, bool) { return Status{}, false }
