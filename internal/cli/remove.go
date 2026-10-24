// remove.go：`awem remove <名称>`（T07-F6）。破坏性操作必须确认（--yes
// 跳过）；运行中先停（API stop=true，确认文案说明）；--purge 同时删日志。
package cli

import (
	"context"
	"encoding/json"
)

func cmdRemove(d Deps) *Command {
	return &Command{
		Name:    "remove",
		Summary: "删除执行器配置（--purge 同时删本地日志，需确认）",
		Usage:   "awem remove <名称> [--purge] [--yes]",
		Run: func(g GlobalFlags, args []string) error {
			if len(args) < 1 || args[0] == "" {
				return usagef("用法：awem remove <名称> [--purge]")
			}
			name := args[0]
			purge := false
			for _, a := range args[1:] {
				switch a {
				case "--purge":
					purge = true
				default:
					return usagef("remove 不认识参数 %s", a)
				}
			}

			ctx := context.Background()
			cl, err := d.Connect(ctx)
			if err != nil {
				return err
			}
			// 取当前状态：存在性 + 运行态（确认文案用）
			data, err := cl.GetJSON(ctx, "/api/executors/"+name)
			if err != nil {
				return wrapAPIErr(err) // 3
			}
			var cur struct {
				Executor struct {
					Autostart bool `json:"autostart"`
					Local     struct {
						State string `json:"state"`
					} `json:"local"`
				} `json:"executor"`
			}
			_ = json.Unmarshal(data, &cur)
			running := cur.Executor.Local.State == "running" ||
				cur.Executor.Local.State == "backoff" || cur.Executor.Local.State == "stopping"

			if !g.Yes {
				prompt := "删除执行器 " + name
				if running {
					prompt += "（正在运行，将先停止其进程）"
				}
				if purge {
					prompt += " 并删除其全部本地日志"
				}
				if !d.confirm(prompt + "？此操作不可恢复") {
					d.printf("已取消\n")
					return nil
				}
			}

			path := "/api/executors/" + name + "?purge=" + boolQuery(purge)
			if running {
				path += "&stop=true"
			}
			data, err = cl.Do(ctx, "DELETE", path, nil)
			if err != nil {
				return wrapAPIErr(err)
			}
			if g.JSON {
				d.printJSON(data)
				return nil
			}
			var out struct {
				Stopped bool `json:"stopped"`
				Purged  bool `json:"purged"`
			}
			_ = json.Unmarshal(data, &out)
			d.printf("已删除 %s", name)
			if out.Stopped {
				d.printf("（已停止进程）")
			}
			if out.Purged {
				d.printf("（日志已清除）")
			}
			d.printf("\n")
			return nil
		},
	}
}

func boolQuery(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
