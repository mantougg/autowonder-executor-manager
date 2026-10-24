// awem 是 AutoWonder Executor Manager 的唯一二进制入口：
// CLI 子命令、daemon、内嵌 Web 页同体（design.md §4）。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"awem/internal/cli"
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
				out, err := json.Marshal(map[string]string{"version": version.Version})
				if err != nil {
					return err
				}
				fmt.Println(string(out))
				return nil
			}
			fmt.Println("awem " + version.Version)
			return nil
		},
	})

	placeholders := []cli.Command{
		{Name: "add", Summary: "粘贴整条启动命令添加执行器", Usage: "awem add [--autostart] [--file <路径>]"},
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
		{Name: "service", Summary: "系统服务安装/卸载/状态", Usage: "awem service install|uninstall|status"},
		{Name: "reload", Summary: "手动触发配置重载", Usage: "awem reload"},
	}
	for i := range placeholders {
		r.Register(&placeholders[i])
	}

	return r
}
