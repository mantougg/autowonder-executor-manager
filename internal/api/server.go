// server.go：本地 REST API 服务（T06-F3）。路由选型（记入变更记录）：
// 标准库 net/http.ServeMux 的方法模式（Go 1.22+，`GET /api/executors/{name}`），
// 零第三方依赖即可覆盖本任务全部资源化路由。
// 鉴权（F3 先挂 token 校验；Host/Origin 防 CSRF 在 F5 补齐）：
// 全部 /api/* 与 /ws/* 校验 token（Authorization: Bearer <t> 或 X-Awem-Token: <t>），
// 静态资源（SPA 壳）免鉴权。
package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"awem/internal/config"
	"awem/internal/logs"
	"awem/internal/platform"
	"awem/internal/supervisor"
	"awem/internal/version"
)

// Options 是 Server 的依赖注入（daemon 装配真身，测试注入替身）。
type Options struct {
	Paths     config.Paths
	Token     string            // API 鉴权 token
	Logs      *logs.Manager     // 日志读取（tail/follow）
	Sup       *supervisor.Supervisor
	Watcher   *config.Watcher   // 当前配置 + Reload 发布入口
	Plat      platform.Syncer   // 平台状态（T08 前 Noop）
	Port      int               // 实际监听端口（/api/status 展示）
	Version   string            // 版本（空=internal/version）
}

// Server 是本地 API 服务；并发安全（写路径串行化）。
type Server struct {
	opts      Options
	startedAt time.Time
	mu        sync.Mutex // 配置写路径串行化（读当前→改→校验→落盘→发布）
}

// New 创建 API 服务。
func New(opts Options) *Server {
	if opts.Version == "" {
		opts.Version = version.Version
	}
	return &Server{opts: opts, startedAt: time.Now()}
}

// StartedAt 记录 daemon 服务就绪时刻（daemon 传入，避免测试重启重置）。
func (s *Server) SetStartedAt(t time.Time) { s.startedAt = t }

// Handler 返回完整 HTTP 路由（含鉴权中间件与静态资源挂载点）。
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	s.registerExecutorRoutes(api)
	s.registerSettingsRoutes(api)

	root := http.NewServeMux()
	root.Handle("/api/", s.auth(api))
	root.Handle("/ws/", s.auth(s.wsHandler())) // F4 实现；先占位拒绝升级
	root.HandleFunc("/", s.spaHandler)         // 静态资源（免鉴权 SPA 壳，F7 完善 fallback）
	return root
}

// auth 校验 token 头后转发（无/错 token 一律 401）。
// Host/Origin 防 CSRF 校验在 F5 加固。
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.checkToken(r) {
			writeAPIError(w, &apiError{
				Status: http.StatusUnauthorized, Code: "unauthorized",
				Message: "缺少或错误的 API token（Authorization: Bearer <t> 或 X-Awem-Token: <t>）",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// checkToken 提取并比对 token 头。
func (s *Server) checkToken(r *http.Request) bool {
	tok := ""
	if h := r.Header.Get("Authorization"); h != "" {
		tok = strings.TrimSpace(strings.TrimPrefix(h, "Bearer"))
	}
	if tok == "" {
		tok = strings.TrimSpace(r.Header.Get("X-Awem-Token"))
	}
	return tok != "" && tok == s.opts.Token
}

// wsHandler WebSocket 占位（F4 实现日志实时流）。
func (s *Server) wsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "WebSocket 端点尚未实现"})
	})
}

// spaHandler 静态资源（F7 升级为完整 SPA fallback；当前返回内嵌首页）。
func (s *Server) spaHandler(w http.ResponseWriter, r *http.Request) {
	s.serveStatic(w, r)
}

// currentConfig 返回当前生效配置。
func (s *Server) currentConfig() config.Config { return s.opts.Watcher.Current() }

// mutateConfig 串行执行配置写路径：读当前 → fn 修改 → 校验 → 落盘 →
// Watcher.Reload 同步发布（订阅者 Supervisor.ApplyConfig 即时生效；
// 发布与文件监视共用同一入口，见 F7）。reloadErr 非 nil 时配置未变（已回滚语义
// 由调用方处理：磁盘仍是旧配置）。
func (s *Server) mutateConfig(fn func(*config.Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.currentConfig()
	if err := fn(&next); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return &apiError{Status: http.StatusBadRequest, Code: "invalid_config", Message: err.Error()}
	}
	if err := config.Save(s.opts.Paths.ConfigFile, next); err != nil {
		return &apiError{Status: http.StatusInternalServerError, Code: "internal",
			Message: "配置落盘失败：" + err.Error()}
	}
	s.opts.Watcher.Reload()
	return nil
}
