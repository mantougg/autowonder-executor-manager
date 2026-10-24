// Package daemon 负责 daemon 进程生命周期与 CLI 端的惰性拉起：
// 单实例锁（lock*.go）、127.0.0.1 监听与端口写回 daemon.json（listen.go）、
// 配置热加载接线、优雅退出；CLI 侧 Client 连接本地 API，连不上时自动
// 后台拉起 daemon（client.go，detach_unix.go / detach_windows.go 平台分离）。
//
// 入口：Run(ctx, Options) 前台运行 daemon（cmd/awem 的 daemon 子命令）；
// Connect(ctx, ClientOptions) 给 CLI 用（失败映射退出码 5）；
// Client.FollowLogs 走 WS 订阅实时日志（wsclient.go）。
package daemon
