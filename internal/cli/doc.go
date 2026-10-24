// Package cli 实现子命令注册与分发（Registry / Command）、全局 flag
// （--json / --yes）解析，以及错误类型到退出码（0/1/2/3/4/5）的统一映射。
//
// 装配入口：RegisterCommands（commands.go）。全部数据操作走本地 API
// （红线：不绕过 API 直改状态），依赖经 Deps 注入——生产真连 daemon
// （daemon.Connect 惰性拉起），测试直连 httptest 假 daemon。
package cli
