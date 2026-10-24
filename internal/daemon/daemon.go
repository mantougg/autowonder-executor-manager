// Package daemon 装配并运行 awem 常驻进程：单实例锁、config/logs/supervisor/
// platform 组件组装、生命周期（启动写 daemon.json、退出清理）与信号收口
// （design.md §4）。本地 HTTP API 服务在 internal/api（T06 F2 起接入）。
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"awem/internal/api"
	"awem/internal/config"
	"awem/internal/launchcmd"
	"awem/internal/logs"
	"awem/internal/platform"
	"awem/internal/supervisor"
	"awem/internal/version"
)

// Options 是 Run 的入参（测试可注入的项放这里）。
type Options struct {
	Paths config.Paths // awem 目录布局（必填）
}

// app 聚合 daemon 运行时的全部组件，供生命周期各阶段使用。
type app struct {
	paths   config.Paths
	cfg     config.Config // 启动时加载的配置（port 写回后同步更新，F2）
	token   string        // API 鉴权 token
	logs    *logs.Manager
	watcher *config.Watcher
	sup     *supervisor.Supervisor
	plat    *platRef    // 平台联动（可热替换；T08-F6）
	api     *api.Server // 本地 API（serve 阶段创建）

	startedAt time.Time
}

// platRef 是平台联动的稳定引用（api.Options 持有它；热加载重建 Service
// 时替换内部实现，api 侧无感）。ping 为连通探测（doctor 用）。
type platRef struct {
	mu    sync.RWMutex
	inner platform.Syncer
	svc   *platform.Service // nil=禁用
}

func (r *platRef) Lookup(executorID string) (platform.Status, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.inner.Lookup(executorID)
}

func (r *platRef) Ping(ctx context.Context) error {
	r.mu.RLock()
	svc := r.svc
	r.mu.RUnlock()
	if svc == nil {
		return errors.New("未配置 [platform] 段")
	}
	return svc.Ping(ctx)
}

// swap 热替换内部实现（platform 段变化时重建 Service）。
func (r *platRef) swap(svc *platform.Service) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.svc = svc
	if svc == nil {
		r.inner = platform.Noop{}
	} else {
		r.inner = svc
	}
}

// Run 启动 daemon 并阻塞直到 ctx 取消（调用方负责把 SIGINT/SIGTERM 接到 ctx）。
// 流程：加单实例锁 → 装配组件 → 写 daemon.json → 记启动事件 → 服务循环 →
// 优雅收口（停执行器、清 daemon.json、记停止事件）。
// 启动失败（锁被持有、配置无效、目录不可写）返回错误，daemon 不运行。
func Run(ctx context.Context, opts Options) error {
	if opts.Paths == (config.Paths{}) {
		return errors.New("daemon 启动参数缺失 Paths")
	}
	p := opts.Paths
	if err := config.EnsureDir(p); err != nil {
		return err
	}

	// 单实例：锁被持有即拒绝启动，并尽量报告现有实例的 pid/port
	lock, err := AcquireLock(filepath.Join(p.DataDir, "daemon.lock"))
	if err != nil {
		if errors.Is(err, ErrLockHeld) {
			if info, alive, rerr := config.ReadDaemonInfo(p); rerr == nil && alive {
				return fmt.Errorf("已有 awem daemon 运行中（pid %d，port %d）；如需重启请先停止现有实例", info.PID, info.Port)
			}
			return fmt.Errorf("已有 awem daemon 正在启动或异常残留（%w），请稍后重试", err)
		}
		return err
	}
	defer lock.Release()

	a, err := bootstrap(p)
	if err != nil {
		return err
	}
	defer a.close()

	// 写元数据与清理收口在 serve 内（拿到实际监听端口后）；退出清理由
	// 此 defer 兜底（serve 正常返回前也会清理，ClearDaemonInfo 幂等）
	defer func() { _ = config.ClearDaemonInfo(p) }()

	// 首份配置注入 supervisor（autostart 条目随即拉起，T05 F4）
	a.applyConfig(a.cfg)

	// 文件监视热加载（design §5.1；写回端口等外部变更由 Reload 统一处理）
	go func() { _ = a.watcher.Start(ctx) }()
	// 平台状态轮询（未配置 [platform] 时 Start 立即返回，无开销）
	a.plat.mu.RLock()
	svc := a.plat.svc
	a.plat.mu.RUnlock()
	if svc != nil {
		go svc.Start(ctx)
	}

	return a.serve(ctx)
}

// bootstrap 装配全部运行时组件（锁获取之后调用）。
func bootstrap(p config.Paths) (*app, error) {
	cfg, err := config.Load(p.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("daemon 启动中止（启动时配置必须可解析）：%w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("daemon 启动中止（启动时配置必须有效）：%w", err)
	}
	token, err := config.APIToken(p)
	if err != nil {
		return nil, err
	}
	mgr := logs.NewManager(p.ExecutorsLogDir, p.EventsLog,
		int64(cfg.Log.MaxSizeMB)*1024*1024, 0, cfg.Log.RetentionDays)

	watcher := config.NewWatcher(p.ConfigFile, cfg)
	sup := supervisor.New(supervisor.Options{Logs: mgr})

	a := &app{
		paths: p, cfg: cfg, token: token,
		logs: mgr, watcher: watcher, sup: sup,
		plat: &platRef{inner: platform.Noop{}},
	}
	// 平台联动装配（[platform] 段启用时为 MCP 只读 Service，T08）
	a.swapPlatform(cfg)
	// 热加载：新配置发布给 supervisor；失败（语法/校验）记事件并沿用旧配置
	watcher.Subscribe(func(c config.Config) {
		a.applyConfig(c)
		a.eventf(logs.EventConfigChanged, "", "配置热加载生效（执行器重新对齐）")
	})
	watcher.OnError(func(err error) {
		a.eventf(logs.EventConfigReloadFailed, "", err.Error())
	})
	return a, nil
}

// applyConfig 应用一份配置（复制后交 supervisor；同步内存副本）；
// [platform] 段变化时热重建联动 Service（F6）。
func (a *app) applyConfig(c config.Config) {
	old := a.cfg
	a.cfg = c
	cc := c
	a.sup.ApplyConfig(&cc)
	if !platformSame(old.Platform, c.Platform) {
		a.eventf(logs.EventPlatformSync, "", "检测到 [platform] 段变化，重建平台联动")
		a.swapPlatform(c)
	}
}

// platformSame 比较两段平台配置是否等价（nil 语义参与）。
func platformSame(a, b *config.PlatformConfig) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// serve 是 daemon 的服务主循环：监听本地 API（port=0 写回，F2）→
// 写 daemon.json（实际端口）→ 记启动事件 → HTTP 服务直到 ctx 取消 →
// 优雅收口。
func (a *app) serve(ctx context.Context) error {
	ln, cfg, err := listenWithWriteBack(a.paths, a.cfg, func(detail string) {
		a.eventf(logs.EventDaemonStart, "", detail)
	})
	if err != nil {
		return err
	}
	defer ln.Close()
	if cfg.Server.Port != a.cfg.Server.Port {
		// 端口已写回：同步内存与 watcher，抑制文件监视触发的等价重载
		a.cfg = cfg
		a.watcher.SetCurrent(cfg)
	}

	// 元数据用实际端口写入；写失败不致命（CLI 探活失败，记事件提示）
	if err := config.WriteDaemonInfo(a.paths, config.DaemonInfo{PID: os.Getpid(), Port: a.cfg.Server.Port}); err != nil {
		a.eventf(logs.EventDaemonStart, "", "daemon.json 写入失败（不影响运行）："+err.Error())
	}
	a.eventf(logs.EventDaemonStart, "",
		fmt.Sprintf("awem %s 启动（pid %d，port %d，执行器 %d 个）",
			version.Version, os.Getpid(), a.cfg.Server.Port, len(a.cfg.Executors)))

	srv := &http.Server{Handler: a.httpHandler()}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	a.startedAt = time.Now()
	if a.api != nil {
		a.api.SetStartedAt(a.startedAt)
	}

	select {
	case <-ctx.Done():
		a.eventf(logs.EventDaemonStop, "", "收到退出信号，daemon 收口中")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		<-serveErr
		return nil
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP 服务异常退出：%w", err)
		}
		return nil
	}
}

// httpHandler 组装本地 API 服务（REST + WS + 静态资源 + 鉴权）。
func (a *app) httpHandler() http.Handler {
	a.api = api.New(api.Options{
		Paths:          a.paths,
		Token:          a.token,
		Logs:           a.logs,
		Sup:            a.sup,
		Watcher:        a.watcher,
		Plat:           a.plat,
		Ping:           a.plat.Ping,
		PlatConfigured: func() bool { return a.watcher.Current().Platform != nil },
		Port:           a.cfg.Server.Port,
	})
	return a.api.Handler()
}

// swapPlatform 按 [platform] 段（重）建联动 Service；段删除 → Noop。
// 平台轮询的生命周期：重建即换新实例，旧实例轮询随 daemon ctx 或下一轮
// tick 的实例替换自然停止（Service 自身无外部可观察写，重复一轮无害）。
func (a *app) swapPlatform(cfg config.Config) *platform.Service {
	if cfg.Platform == nil {
		a.plat.swap(nil)
		return nil
	}
	svc := platform.NewService(platform.Options{
		Config: &platform.SyncerConfig{
			BaseURL:  cfg.Platform.BaseURL,
			Token:    cfg.Platform.MCPToken,
			Interval: cfg.Platform.PollInterval,
		},
		CachePath: filepath.Join(a.paths.DataDir, "platform-cache.json"),
		LocalIDs:  a.localExecutors,
		OnEvent: func(typ, executorID, detail string) {
			a.eventf(logs.EventPlatformSync, executorID, typ+"："+detail)
		},
	})
	a.plat.swap(svc)
	go svc.Start(context.Background())
	return svc
}

// localExecutors 从当前配置解析本地执行器联动字段（发现与漂移的本地侧）。
func (a *app) localExecutors() []platform.LocalExecutor {
	cfg := a.watcher.Current()
	out := make([]platform.LocalExecutor, 0, len(cfg.Executors))
	for _, e := range cfg.Executors {
		if p, err := launchcmd.Parse(e.Command); err == nil {
			out = append(out, platform.LocalExecutor{
				ExecutorID:      p.ExecutorID,
				Model:           p.Model,
				ReasoningEffort: p.ReasoningEffort,
				ContextWindow:   p.ContextWindow,
				MemoryMode:      p.MemoryMode,
				MaxTasks:        p.MaxTasks,
			})
		}
	}
	return out
}

// close 释放全部组件（defer 调用，幂等由各组件保证）。
func (a *app) close() {
	a.sup.Shutdown()
	a.logs.Close()
}

// eventf 记录管理事件（事件日志不可用时不阻塞主流程）。
func (a *app) eventf(typ logs.EventType, name, detail string) {
	if el, err := a.logs.EventLog(); err == nil {
		_ = el.Record(typ, name, detail)
	}
}
