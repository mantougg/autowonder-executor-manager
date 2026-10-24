// Package api 提供仅绑定 127.0.0.1 的本地 HTTP API（REST + WS 日志流）
// 与内嵌 Web 静态资源服务（go:embed 的 React SPA），CLI 与 Web 页共用
// 这一套接口。
//
// 入口：New(Options) 构造 Server，Handler() 返回 http.Handler。
// 路由与鉴权（token 头 + Host/Origin 防 CSRF，静态资源免鉴权）在
// server.go；对外 JSON 视图契约在 schema.go（与 CLI --json、Web
// api.ts 同构，字段只增不改）；WS 实时日志在 ws.go。
package api
