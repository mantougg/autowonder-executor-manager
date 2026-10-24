// awem 是 AutoWonder Executor Manager 的唯一二进制入口：
// CLI 子命令、daemon、内嵌 Web 页同体（design.md §4）。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"awem/internal/cli"
	"awem/internal/config"
	"awem/internal/daemon"
	"awem/internal/exitcode"
	"awem/internal/logs"
	"awem/internal/service"
	"awem/internal/version"
)

func main() {
	os.Exit(buildRegistry().Dispatch(os.Args[1:]))
}

// buildRegistry 注册全部子命令。T01 只实现 version，
// 其余按 design.md §8.1 占位注册（调用即报"未实现"），
// 后续任务逐个填肉而不改分发骨架。
func buildRegistry() *cli.Registry {
	r := cli.NewRegistry()

	r.Register(&cli.Command{
		Name:    "version",
		Summary: "显示版本号",
		Usage:   "awem version",
		Run: func(g cli.GlobalFlags, args []string) error {
			if g.JSON {
				out, err := json.Marshal(map[string]string{
					"version": version.Version,
					"go":      runtime.Version(),
					"os":      runtime.GOOS,
					"arch":    runtime.GOARCH,
				})
				if err != nil {
					return err
				}
				fmt.Println(string(out))
				return nil
			}
			fmt.Println("awem " + version.Version)
			fmt.Printf("  %s/%s（%s）\n", runtime.GOOS, runtime.GOARCH, runtime.Version())
			return nil
		},
	})

	// daemon：前台常驻入口（service/systemd 托管对象，design §4/§12）。
	r.Register(&cli.Command{
		Name:    "daemon",
		Summary: "前台运行常驻 daemon（服务托管/调试用；CLI 会自动拉起，无需手工运行）",
		Usage:   "awem daemon",
		Run: func(g cli.GlobalFlags, args []string) error {
			if len(args) > 0 {
				return exitcode.Usagef("awem daemon 不接受参数")
			}
			p, err := config.DefaultLayout()
			if err != nil {
				return exitcode.Wrap(exitcode.General, err)
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := daemon.Run(ctx, daemon.Options{Paths: p}); err != nil {
				return exitcode.Wrap(exitcode.General, err)
			}
			return nil
		},
	})

	placeholders := []cli.Command{
		{Name: "list", Summary: "列出全部执行器（本地+平台状态）", Usage: "awem list"},
		{Name: "status", Summary: "查看运行态、pid、uptime、重启次数等", Usage: "awem status [名称]"},
		{Name: "start", Summary: "启动执行器（无名称=全部）", Usage: "awem start [名称]"},
		{Name: "stop", Summary: "停止执行器（无名称=全部，需确认）", Usage: "awem stop [名称]"},
		{Name: "restart", Summary: "重启执行器（无名称=全部）", Usage: "awem restart [名称]"},
		{Name: "edit", Summary: "结构化字段编辑（--model 等；--command 整条替换）", Usage: "awem edit <名称> [--model …]"},
		{Name: "remove", Summary: "删除执行器配置（--purge 同时删本地日志，需确认）", Usage: "awem remove <名称> [--purge]"},
		{Name: "log", Summary: "查看执行器日志（-f 跟随）", Usage: "awem log <名称> [-f] [--lines N]"},
		{Name: "import", Summary: "解析旧 executor-manager.sh 导入", Usage: "awem import [--with-logs]"},
		{Name: "doctor", Summary: "环境体检（Node/npx/平台连通/磁盘）", Usage: "awem doctor"},
	}
	for i := range placeholders {
		r.Register(&placeholders[i])
	}

	// service：系统服务安装/卸载/状态（T10）。
	r.Register(&cli.Command{
		Name:    "service",
		Summary: "系统服务安装/卸载/状态（Linux systemd user / macOS launchd / Windows 登录自启）",
		Usage:   "awem service install|uninstall|status",
		Run: func(g cli.GlobalFlags, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("用法：awem service install|uninstall|status")
			}
			p, err := config.DefaultLayout()
			if err != nil {
				return exitcode.Wrap(exitcode.General, err)
			}
			var el *logs.EventLog
			if mgr := logs.NewManager(p.ExecutorsLogDir, p.EventsLog, 0, 0, 0); mgr != nil {
				el, _ = mgr.EventLog()
			}
			switch args[0] {
			case "install":
				if err := service.Install(el); err != nil {
					return exitcode.Wrap(exitcode.General, err)
				}
				fmt.Println("服务安装完成")
				return nil
			case "uninstall":
				if err := service.Uninstall(el); err != nil {
					return exitcode.Wrap(exitcode.General, err)
				}
				fmt.Println("服务已卸载")
				return nil
			case "status":
				info, err := service.Status()
				if err != nil {
					return exitcode.Wrap(exitcode.General, err)
				}
				if g.JSON {
					out, _ := json.Marshal(info)
					fmt.Println(string(out))
					return nil
				}
				if !info.Installed {
					fmt.Printf("未安装（%s）\n", info.Detail)
				} else if info.Running {
					fmt.Printf("已安装且运行中：%s\n", info.Detail)
				} else {
					fmt.Printf("已安装未运行：%s\n", info.Detail)
				}
				if info.PID > 0 {
					fmt.Printf("daemon pid %d，端口 %d\n", info.PID, info.Port)
				}
				return nil
			default:
				return exitcode.Usagef("未知子命令 %q（应为 install|uninstall|status）", args[0])
			}
		},
	})

	// API 依赖型子命令集（T07 逐功能点扩充；重名覆盖上面的占位注册）
	cli.RegisterCommands(r, cli.Deps{})

	return r
}
