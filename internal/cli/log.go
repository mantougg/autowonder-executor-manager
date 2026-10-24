// log.go：`awem log <名称> [-f] [--lines N]`（T07-F7）。默认尾部 N 行
// （N 默认 100）；-f 经 WS 跟随（Ctrl-C 干净退出）；输出原始日志行。
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"awem/internal/daemon"
)

// defaultLogLines 是 --lines 缺省值（决策记录于任务文档）。
const defaultLogLines = 100

func cmdLog(d Deps) *Command {
	return &Command{
		Name:    "log",
		Summary: "查看执行器日志（-f 跟随）",
		Usage:   "awem log <名称> [-f] [--lines N]",
		Run: func(g GlobalFlags, args []string) error {
			var name string
			follow := false
			lines := defaultLogLines
			rest := args
			if len(rest) == 0 || rest[0] == "" {
				return usagef("用法：awem log <名称> [-f] [--lines N]")
			}
			name = rest[0]
			for _, a := range rest[1:] {
				switch {
				case a == "-f" || a == "--follow":
					follow = true
				case a == "--lines" || a == "-n":
					return usagef("--lines 请用 --lines=N 形式")
				default:
					if n, ok := cutPrefix(a, "--lines="); ok {
						v, err := parsePositive(n)
						if err != nil {
							return usagef("--lines=%s：%v", n, err)
						}
						lines = v
						continue
					}
					return usagef("log 不认识参数 %s", a)
				}
			}

			ctx := context.Background()
			cl, err := d.Connect(ctx)
			if err != nil {
				return err
			}
			if follow {
				return d.followLogs(ctx, cl, name, lines)
			}

			// 非流式：tail 拉取
			data, err := cl.GetJSON(ctx, "/api/executors/"+name+"/log?tail="+itoa(lines))
			if err != nil {
				return wrapAPIErr(err) // 3
			}
			if g.JSON {
				d.printJSON(data)
				return nil
			}
			var out struct {
				Lines []string `json:"lines"`
			}
			if err := json.Unmarshal(data, &out); err != nil {
				return generalf("解析日志响应失败：%v", err)
			}
			if len(out.Lines) == 0 {
				d.printf("（%s 暂无日志；启动后产生：awem start %s）\n", name, name)
				return nil
			}
			for _, line := range out.Lines {
				d.printf("%s\n", line)
			}
			return nil
		},
	}
}

// followLogs -f 模式：backlog + 实时行；Ctrl-C 干净退出。
func (d Deps) followLogs(ctx context.Context, cl *daemon.Client, name string, tail int) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	d.printf("（跟随 %s 日志，Ctrl-C 退出）\n", name)
	return cl.FollowLogs(ctx, name, tail, func(ev daemon.LogEvent) bool {
		switch ev.Type {
		case "backlog":
			for _, line := range ev.Lines {
				d.printf("%s\n", line)
			}
		case "line":
			d.printf("%s\n", ev.Text)
		case "error":
			d.printf("日志流出错：%s\n", ev.Message)
			return false
		}
		return true
	})
}

// cutPrefix 拆前缀。
func cutPrefix(s, prefix string) (string, bool) {
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return "", false
}

// parsePositive 正整数解析。
func parsePositive(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("须为正整数")
	}
	return n, nil
}

func itoa(n int) string { return strconv.Itoa(n) }
