// add.go：`awem add`（T07-F1）。整条粘贴启动命令为一等入口；名称默认
// `exec-<executor-id>`（可用 --name 覆盖，决策记录于任务文档）；
// 添加后默认不启动（--autostart 表示随 daemon 拉起，--start 表示立即启动）。
package cli

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"awem/internal/exitcode"
	"awem/internal/launchcmd"
)

// addRequest 与 API POST /api/executors 请求体一致（F3 契约）。
type addRequest struct {
	Name           string `json:"name"`
	Command        string `json:"command"`
	Autostart      bool   `json:"autostart"`
	Start          bool   `json:"start"`
	WorkspaceRoot  string `json:"workspace_root"`
	RestartEnabled *bool  `json:"restart_enabled"`
}

func cmdAdd(d Deps) *Command {
	return &Command{
		Name:    "add",
		Summary: "粘贴整条启动命令添加执行器",
		Usage:   "awem add \"<整条启动命令>\" [--name 名称] [--file 路径] [--autostart|--no-autostart] [--start] [--restart|--no-restart] [--debug 附加] [--workspace-root 路径]",
		Run: func(g GlobalFlags, args []string) error {
			opts, err := parseAddArgs(args)
			if err != nil {
				return err // 已是用法错误（2）
			}
			command := opts.command
			if opts.file != "" {
				data, err := os.ReadFile(opts.file)
				if err != nil {
					return exitcode.Usagef("读取 --file 失败：%v", err)
				}
				command = strings.TrimSpace(string(data))
			}
			if command == "" {
				return exitcode.Usagef("缺少启动命令：位置参数粘贴整条命令，或 --file <路径>")
			}

			// 本地先解析一次：确定默认名（exec-<id>）并提前给出逐字段错误（不走网络）
			parsed, perr := launchcmd.Parse(command)
			var name string
			if perr != nil {
				if opts.name == "" {
					return exitcode.Usagef("%v", perr)
				}
			} else if verr := parsed.Validate(); verr != nil {
				if opts.name == "" {
					return exitcode.Usagef("%v", verr)
				}
			}
			name = opts.name
			if name == "" && parsed != nil {
				name = "exec-" + parsed.ExecutorID
			}
			if !launchcmd.ValidExecutorName(name) {
				return exitcode.Usagef("名称 %q 不合规（只允许 [a-zA-Z0-9._-]），用 --name 指定", name)
			}

			req := addRequest{
				Name: name, Command: command,
				Autostart: opts.autostart, Start: opts.start,
				WorkspaceRoot: opts.workspaceRoot,
			}
			if opts.restart != nil {
				req.RestartEnabled = opts.restart
			}
			// --debug：布尔 flag 追加进命令（design §11.3 添加时可选附加）
			if opts.debug {
				req.Command = command + " --debug"
			}
			body, _ := json.Marshal(req)

			ctx := context.Background()
			cl, err := d.Connect(ctx)
			if err != nil {
				return err
			}
			data, err := cl.Do(ctx, "POST", "/api/executors", body)
			if err != nil {
				return wrapAPIErr(err)
			}
			if g.JSON {
				d.printJSON(data)
				return nil
			}
			return d.printAddSummary(data, name)
		},
	}
}

// printAddSummary 输出人读摘要（token 掩码由 API 视图保证）。
func (d Deps) printAddSummary(data []byte, name string) error {
	var out struct {
		Executor struct {
			Name      string `json:"name"`
			Autostart bool   `json:"autostart"`
			Parsed    *struct {
				ExecutorID string `json:"executor_id"`
				Provider   string `json:"provider"`
				Model      string `json:"model"`
				Token      string `json:"token"`
				RuntimeVer string `json:"runtime_version"`
			} `json:"parsed"`
			Local struct {
				State string `json:"state"`
			} `json:"local"`
		} `json:"executor"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return exitcode.Generalf("解析 add 响应失败：%v", err)
	}
	e := out.Executor
	d.printf("已添加执行器 %s\n", e.Name)
	if e.Parsed != nil {
		d.printf("  执行器 ID：%s\n", e.Parsed.ExecutorID)
		d.printf("  provider：%s    模型：%s\n", e.Parsed.Provider, e.Parsed.Model)
		d.printf("  runtime 版本：autowonder@%s\n", e.Parsed.RuntimeVer)
		d.printf("  token：%s（掩码显示，明文已写入配置）\n", e.Parsed.Token)
	}
	d.printf("  autostart：%s\n", boolCN(e.Autostart))
	if e.Local.State == "running" {
		d.printf("  已启动（--start）\n")
	}
	d.printf("提示：awem start %s 可启动；awem log %s -f 跟随日志\n", e.Name, e.Name)
	return nil
}

// addOptions 是 add 的参数集。
type addOptions struct {
	command       string
	file          string
	name          string
	autostart     bool
	start         bool
	debug         bool
	workspaceRoot string
	restart       *bool
}

// parseAddArgs 解析 add 参数：首个非 flag 参数为命令（支持带引号粘贴）。
func parseAddArgs(args []string) (*addOptions, error) {
	o := &addOptions{}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--file":
			if i+1 >= len(args) {
				return nil, exitcode.Usagef("--file 缺少路径")
			}
			i++
			o.file = args[i]
		case strings.HasPrefix(a, "--file="):
			o.file = strings.TrimPrefix(a, "--file=")
		case a == "--name":
			if i+1 >= len(args) {
				return nil, exitcode.Usagef("--name 缺少值")
			}
			i++
			o.name = args[i]
		case strings.HasPrefix(a, "--name="):
			o.name = strings.TrimPrefix(a, "--name=")
		case a == "--autostart":
			o.autostart = true
		case a == "--no-autostart":
			o.autostart = false
		case a == "--start":
			o.start = true
		case a == "--restart":
			o.restart = boolPtr(true)
		case a == "--no-restart":
			o.restart = boolPtr(false)
		case a == "--debug":
			o.debug = true
		case a == "--workspace-root":
			if i+1 >= len(args) {
				return nil, exitcode.Usagef("--workspace-root 缺少路径")
			}
			i++
			o.workspaceRoot = args[i]
		case strings.HasPrefix(a, "--workspace-root="):
			o.workspaceRoot = strings.TrimPrefix(a, "--workspace-root=")
		case strings.HasPrefix(a, "-"):
			return nil, exitcode.Usagef("add 不认识参数 %s", a)
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 1 {
		return nil, exitcode.Usagef("add 只接受一条命令参数（多余：%v）", rest[1:])
	}
	if len(rest) == 1 {
		o.command = rest[0]
	}
	return o, nil
}

func boolPtr(b bool) *bool { return &b }

func boolCN(b bool) string {
	if b {
		return "开"
	}
	return "关"
}
