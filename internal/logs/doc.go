// Package logs 负责日志体系：收割执行器子进程 stdout/stderr 到
// data/logs/executors/<name>.log（按大小滚动 + 保留策略清理），
// 以及管理事件日志 data/logs/events.log（JSONL）。
//
// 入口：NewManager 构造 Manager；Sink 取执行器日志写入端；
// Tail / TailOffset / ReadLines 读历史；Follow / FollowFrom 实时订阅
// （Subscription，供 CLI -f 与 Web WS 用）；Purge 删除执行器全部日志；
// Cleanup 按保留天数清理。事件日志用 EventLog（OpenEventLog）；
// 敏感值输出统一过 MaskSecret 掩码。
package logs
