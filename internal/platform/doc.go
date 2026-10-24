// Package platform 负责平台状态联动（只读）：通过个人 MCP 令牌调用
// list_projects / list_executors / get_executor_launch_config 三个白名单
// 工具，轮询执行器平台在线状态并做漂移检测。
// 不发起任何平台写操作。完整实现见任务 T08。
package platform
