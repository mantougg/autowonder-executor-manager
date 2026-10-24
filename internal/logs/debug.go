// debug.go：--debug 透传支持（T04-F5）。
// 执行器的 debug 开关（来自 T03 解析的 --debug）贯通到日志与事件层：
// ① 启动事件显式记录 debug 标记（上游文档警告 debug 的 rotated 诊断日志
// 长跑占磁盘、默认关，事件标记便于排障）；② 提供从整条命令查询开关的
// 便捷入口，供 Supervisor（T05）spawn 时附加参数。
package logs

import "awem/internal/launchcmd"

// RecordExecutorStart 记录执行器启动事件；debug=true 时附标记与磁盘提示。
func (e *EventLog) RecordExecutorStart(executor string, debug bool) error {
	detail := ""
	if debug {
		detail = "debug=true（附加 --debug 启动：rotated 诊断日志长跑占磁盘，注意清理）"
	}
	return e.Record(EventExecutorStart, executor, detail)
}

// DebugEnabled 从整条启动命令查询 --debug 开关（解析失败按 false）。
func DebugEnabled(command string) bool {
	p, err := launchcmd.Parse(command)
	return err == nil && p.Debug
}
