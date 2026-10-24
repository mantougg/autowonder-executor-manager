// state.go：守护状态机的状态与参数（T05-F2/F5）。
// 状态枚举是 CLI/Web/事件共用的对外契约，记录于任务文档变更记录。
package supervisor

import "time"

// State 是执行器的本地守护状态（对外契约：CLI/Web/事件引用）。
// spawn 在 unit 锁内同步完成，"启动中"瞬态不对外暴露，故无 Starting 值。
type State string

const (
	StateRunning  State = "running"  // 子进程运行中
	StateBackoff  State = "backoff"  // 崩溃后退避等待自动拉起
	StateStopping State = "stopping" // 人为停止进行中（宽限期内）
	StateStopped  State = "stopped"  // 已停止（人为停止后的稳态）
	StateFailed   State = "failed"   // 终态失败（崩溃后不再拉起 / 启动失败）
)

// RunningLike 报告 state 是否"在管/在跑"（幂等 start 的判断依据）。
func RunningLike(s State) bool {
	return s == StateRunning || s == StateBackoff || s == StateStopping
}

// ConnState 是从 connect 日志解析出的平台连接状态（F5；本地进程活着 ≠ 平台
// ONLINE，两者并列展示——design §6）。
type ConnState string

const (
	ConnUnknown      ConnState = "unknown"      // 未见过连接日志（刚启动/无日志）
	ConnConnected    ConnState = "connected"    // ws connected
	ConnReconnecting ConnState = "reconnecting" // 断线重连中
)

// BackoffParams 是守护参数（design §6 默认值；测试注入毫秒级快参数）。
type BackoffParams struct {
	Initial     time.Duration // 退避起步间隔（默认 1s）
	Max         time.Duration // 退避上限（默认 5m）
	StableReset time.Duration // 稳定运行多久后重置计数（默认 10m）
	Grace       time.Duration // 人为停止的优雅宽限，超时强杀（默认 10s）
}

// DefaultBackoffParams 返回设计默认参数（1s ×2 上限 5m，稳定 10m 重置，宽限 10s）。
func DefaultBackoffParams() BackoffParams {
	return BackoffParams{
		Initial:     time.Second,
		Max:         5 * time.Minute,
		StableReset: 10 * time.Minute,
		Grace:       10 * time.Second,
	}
}

// delay 返回第 step 次（从 0 起）崩溃后的退避间隔：Initial×2^step，封顶 Max。
func (p BackoffParams) delay(step int) time.Duration {
	d := p.Initial
	for i := 0; i < step && d < p.Max; i++ {
		d *= 2
	}
	if d > p.Max {
		d = p.Max
	}
	return d
}
