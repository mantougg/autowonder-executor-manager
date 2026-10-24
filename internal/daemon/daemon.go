// Package daemon 装配并运行 awem 常驻进程：单实例锁、config/logs/supervisor/
// platform 组件组装、生命周期（启动写 daemon.json、退出清理）与信号收口
// （design.md §4）。本地 HTTP API 服务在 internal/api（T06 F2 起接入）。
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"awem/internal/config"
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
	plat    platform.Syncer

	startedAt time.Time
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

	// 写元数据与启动事件（失败不致命：daemon.json 写失败时 CLI 惰性拉起
	// 将探活失败，记事件提示即可）
	if err := config.WriteDaemonInfo(p, config.DaemonInfo{PID: os.Getpid(), Port: a.cfg.Server.Port}); err != nil {
		a.eventf(logs.EventDaemonStart, "", "daemon.json 写入失败（不影响运行）："+err.Error())
	} else {
		a.eventf(logs.EventDaemonStart, "",
			fmt.Sprintf("awem %s 启动（pid %d，port %d，执行器 %d 个）",
				version.Version, os.Getpid(), a.cfg.Server.Port, len(a.cfg.Executors)))
	}
	defer func() { _ = config.ClearDaemonInfo(p) }()

	// 首份配置注入 supervisor（autostart 条目随即拉起，T05 F4）
	a.applyConfig(a.cfg)

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
		plat: platform.Noop{}, // T08 替换为 MCP 只读联动
	}
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

// applyConfig 应用一份配置（复制后交 supervisor；同步内存副本）。
func (a *app) applyConfig(c config.Config) {
	a.cfg = c
	cc := c
	a.sup.ApplyConfig(&cc)
}

// serve 是 daemon 的服务主循环。T06-F1 仅等待退出信号；
// F2 起在此挂载本地 HTTP API。
func (a *app) serve(ctx context.Context) error {
	a.startedAt = time.Now()
	<-ctx.Done()
	a.eventf(logs.EventDaemonStop, "", "收到退出信号，daemon 收口中")
	return nil
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
