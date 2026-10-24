// Package platform 负责平台状态联动（只读）：通过个人 MCP 令牌调用
// list_projects / list_executors / get_executor_launch_config 三个白名单
// 工具（代码层强制，见 whitelist_test.go），轮询执行器平台在线状态
// 并做配置漂移检测。
//
// 入口：NewService(Options) 构造 Service（poller 周期轮询 + syncer
// 状态聚合）；Lookup 查单执行器平台状态，Summary 报 configured/connected，
// Ping 连通性检查。MCP 传输层在 mcp.go；归属自动发现在 discovery.go；
// 五字段漂移对比在 drift.go。不发起任何平台写操作。
package platform
