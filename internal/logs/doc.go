// Package logs 负责日志体系：收割执行器子进程 stdout/stderr 到
// data/logs/executors/<name>.log（按大小滚动 + 保留策略），
// 以及管理事件日志 data/logs/events.log。
// 完整实现见任务 T04。
package logs
