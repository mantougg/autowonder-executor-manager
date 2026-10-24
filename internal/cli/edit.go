// edit.go：`awem edit <名称>`（T07-F5）。结构化字段直改（--model 等）→
// API PUT fields（服务端重建整条命令，passthrough 不丢）；--command 整条
// 替换；运行中的执行器提示"需 restart 生效"。
package cli

import (
	"context"
	"encoding/json"
	"strings"
)

// editFlagNames 是可直改的结构化字段 → API 字段名映射（值型）。
var editFlagNames = []struct {
	flag string // CLI flag（不带 --）
	api  string // API fields 字段名
}{
	{"model", "model"},
	{"reasoning-effort", "reasoning_effort"},
	{"context-window", "context_window"},
	{"memory-mode", "memory_mode"},
	{"max-tasks", "max_tasks"},
	{"ws-url", "ws_url"},
	{"executor-id", "executor_id"},
	{"provider", "provider"},
	{"runtime-version", "runtime_version"},
	{"token", "token"},
}

func cmdEdit(d Deps) *Command {
	return &Command{
		Name:    "edit",
		Summary: "结构化字段编辑（--model 等；--command 整条替换）",
		Usage:   "awem edit <名称> [--model m] [--reasoning-effort r] [--context-window n] [--memory-mode m] [--max-tasks n] [--token t] [--ws-url u] [--executor-id i] [--provider p] [--runtime-version v] [--autostart[=bool]] [--restart[=bool]] [--debug[=bool]] [--workspace-root 路径] [--command \"<新命令>\"]",
		Run: func(g GlobalFlags, args []string) error {
			if len(args) < 1 || args[0] == "" {
				return usagef("用法：awem edit <名称> [--model …]（至少给一个修改项）")
			}
			name := args[0]
			fields := map[string]any{}
			var autostart, restart, debug *bool
			var workspaceRoot *string
			var command string
			rest := args[1:]
			for i := 0; i < len(rest); i++ {
				a := rest[i]
				takeValue := func(flag string) (string, error) {
					if i+1 >= len(rest) {
						return "", usagef("--%s 缺少值", flag)
					}
					i++
					return rest[i], nil
				}
				switch {
				case a == "--command":
					v, err := takeValue("command")
					if err != nil {
						return err
					}
					command = v
				case strings.HasPrefix(a, "--command="):
					command = strings.TrimPrefix(a, "--command=")
				case a == "--autostart":
					autostart = boolPtr(true)
				case a == "--no-autostart":
					autostart = boolPtr(false)
				case strings.HasPrefix(a, "--autostart="):
					b, err := parseBoolFlag(a, "--autostart=")
					if err != nil {
						return err
					}
					autostart = boolPtr(b)
				case a == "--restart":
					restart = boolPtr(true)
				case a == "--no-restart":
					restart = boolPtr(false)
				case strings.HasPrefix(a, "--restart="):
					b, err := parseBoolFlag(a, "--restart=")
					if err != nil {
						return err
					}
					restart = boolPtr(b)
				case a == "--debug":
					debug = boolPtr(true)
				case a == "--no-debug":
					debug = boolPtr(false)
				case strings.HasPrefix(a, "--debug="):
					b, err := parseBoolFlag(a, "--debug=")
					if err != nil {
						return err
					}
					debug = boolPtr(b)
				case a == "--workspace-root":
					v, err := takeValue("workspace-root")
					if err != nil {
						return err
					}
					workspaceRoot = &v
				case strings.HasPrefix(a, "--workspace-root="):
					v := strings.TrimPrefix(a, "--workspace-root=")
					workspaceRoot = &v
				default:
					matched := false
					for _, f := range editFlagNames {
						if a == "--"+f.flag {
							v, err := takeValue(f.flag)
							if err != nil {
								return err
							}
							fields[f.api] = v
							matched = true
							break
						}
						if strings.HasPrefix(a, "--"+f.flag+"=") {
							fields[f.api] = strings.TrimPrefix(a, "--"+f.flag+"=")
							matched = true
							break
						}
					}
					if !matched {
						return usagef("edit 不认识参数 %s", a)
					}
				}
			}
			if len(fields) == 0 && autostart == nil && restart == nil && debug == nil &&
				workspaceRoot == nil && command == "" {
				return usagef("至少给一个修改项（--model 等，或 --command）")
			}

			body := map[string]any{}
			if command != "" {
				body["command"] = command
			}
			if len(fields) > 0 {
				body["fields"] = fields
				// 布尔字段单独塞进 fields（API 契约：TokenAware/Debug *bool）
				if debug != nil {
					f := body["fields"].(map[string]any)
					f["debug"] = *debug
				}
			} else if debug != nil {
				body["fields"] = map[string]any{"debug": *debug}
			}
			if autostart != nil {
				body["autostart"] = *autostart
			}
			if restart != nil {
				body["restart_enabled"] = *restart
			}
			if workspaceRoot != nil {
				body["workspace_root"] = *workspaceRoot
			}
			raw, _ := json.Marshal(body)

			ctx := context.Background()
			cl, err := d.Connect(ctx)
			if err != nil {
				return err
			}
			data, err := cl.Do(ctx, "PUT", "/api/executors/"+name, raw)
			if err != nil {
				return wrapAPIErr(err)
			}
			if g.JSON {
				d.printJSON(data)
				return nil
			}
			var out struct {
				Executor struct {
					Command string `json:"command"`
					Local   struct {
						State string `json:"state"`
					} `json:"local"`
				} `json:"executor"`
			}
			_ = json.Unmarshal(data, &out)
			d.printf("已更新 %s\n  新命令：%s\n", name, out.Executor.Command)
			if st := out.Executor.Local.State; st == "running" || st == "backoff" || st == "stopping" {
				d.printf("提示：%s 正在运行，修改将在下次 restart 后生效（awem restart %s）\n", name, name)
			}
			return nil
		},
	}
}

// eq2 拆 --flag=value。
func eq2(a string) (string, string, bool) {
	if k, v, ok := strings.Cut(a, "="); ok && strings.HasPrefix(k, "--") {
		return k[2:], v, true
	}
	return "", "", false
}

// parseBoolFlag 解析 --xxx=true|false 形态。
func parseBoolFlag(a, prefix string) (bool, error) {
	switch strings.TrimPrefix(a, prefix) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, usagef("%s 只接受 =true|=false", prefix)
}
