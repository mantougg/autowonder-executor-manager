// Package supervisor 负责执行器子进程守护：按配置 spawn
// `npx -y autowonder@<ver> connect …`、整进程树优雅终止、崩溃自动拉起
// （指数退避，稳定运行后重置）、运行状态快照。
//
// 入口：New(Options) 构造 Supervisor；ApplyConfig 全量对齐配置（热加载
// 也走这里）；StartExecutor / StopExecutor / RestartExecutor 单执行器动作；
// Snapshot / SnapshotOne 查询状态（UnitStatus）。
//
// 状态机见 state.go（State），连接状态解析见 connstate.go（ConnState、
// ParseConnLine）；平台差异在 proc_unix.go / proc_windows.go 与
// npx_unix.go / npx_windows.go 成对维护。测试用 internal/fakeproc
// 假长驻子进程，不依赖真实 npx。
package supervisor
