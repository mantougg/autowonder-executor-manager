package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"awem/internal/exitcode"
)

// GlobalFlags 是从命令行任意位置抽出的全局 flag，透传给所有子命令。
type GlobalFlags struct {
	JSON bool // --json：输出稳定 schema 的 JSON（供 AI/脚本消费）
	Yes  bool // --yes：跳过破坏性操作确认
}

// Command 描述一个可注册的子命令。Run 为 nil 表示"占位注册、尚未实现"，
// 调用时统一报"未实现"并退出码 1。
type Command struct {
	Name    string                                  // 子命令名（如 version）
	Summary string                                  // 一句话中文说明（用于 usage）
	Usage   string                                  // 用法行（如 awem log <名称> [-f] [--lines N]）
	Run     func(g GlobalFlags, args []string) error // 实际执行逻辑；报错须用 internal/exitcode 类型包装
}

// Registry 管理全部子命令的注册与分发。
type Registry struct {
	commands map[string]*Command
	order    []string // 保持注册顺序，usage 按此列出
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]*Command)}
}

// Register 注册一个子命令；重名后者覆盖前者。
func (r *Registry) Register(c *Command) {
	if _, exists := r.commands[c.Name]; !exists {
		r.order = append(r.order, c.Name)
	}
	r.commands[c.Name] = c
}

// extractGlobalFlags 从 argv 中抽出全局 flag（--json / --yes，
// 支持 --flag 与 --flag=true|false 两种形式），返回剩余参数。
// 约定全局 flag 可出现在任意位置（子命令前后均可），便于 AI 与脚本调用；
// 若某子命令自身参数值恰好等于全局 flag 名，用 = 形式传值可避免被吞。
func extractGlobalFlags(argv []string) (GlobalFlags, []string) {
	var g GlobalFlags
	rest := make([]string, 0, len(argv))
	for _, arg := range argv {
		switch {
		case arg == "--json":
			g.JSON = true
		case strings.HasPrefix(arg, "--json="):
			g.JSON = arg == "--json=true"
		case arg == "--yes":
			g.Yes = true
		case strings.HasPrefix(arg, "--yes="):
			g.Yes = arg == "--yes=true"
		default:
			rest = append(rest, arg)
		}
	}
	return g, rest
}

// Usage 输出中文用法说明。
func (r *Registry) Usage() string {
	var b strings.Builder
	b.WriteString("awem - AutoWonder 执行器管理器\n\n")
	b.WriteString("用法：awem [全局 flag] <子命令> [参数]\n\n")
	b.WriteString("全局 flag：\n")
	b.WriteString("  --json   以稳定 schema 的 JSON 输出（供 AI/脚本消费）\n")
	b.WriteString("  --yes    跳过破坏性操作确认\n\n")
	b.WriteString("子命令：\n")
	for _, name := range r.order {
		c := r.commands[name]
		usage := c.Usage
		if usage == "" {
			usage = "awem " + c.Name
		}
		fmt.Fprintf(&b, "  %-34s %s\n", usage, c.Summary)
	}
	b.WriteString("\n帮助：awem help、awem <子命令> --help（由各子命令自行实现）\n")
	return b.String()
}

// Dispatch 解析 argv 并执行对应子命令，返回退出码
// （internal/exitcode：0 成功 / 1 一般 / 2 用法 / 3 未找到 / 4 已存在 / 5 daemon 未运行）。
// 文本模式诊断到 stderr、正常输出到 stdout；--json 模式错误结构到 stdout。
func (r *Registry) Dispatch(argv []string) int {
	g, rest := extractGlobalFlags(argv)

	if len(rest) == 0 || rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		if len(rest) == 0 {
			writeError(g.JSON, exitcode.Usagef("缺少子命令"))
			if !g.JSON {
				fmt.Fprint(os.Stderr, r.Usage())
			}
			return exitcode.Usage
		}
		if !g.JSON {
			fmt.Fprint(os.Stderr, r.Usage())
		}
		return exitcode.Success
	}

	name := rest[0]
	cmd, ok := r.commands[name]
	if !ok {
		writeError(g.JSON, exitcode.Usagef("未知子命令：%s", name))
		if !g.JSON {
			fmt.Fprint(os.Stderr, "\n"+r.Usage())
		}
		return exitcode.Usage
	}

	if cmd.Run == nil {
		err := exitcode.Generalf("子命令 %q 尚未实现", name)
		writeError(g.JSON, err)
		return exitcode.Code(err)
	}

	if err := cmd.Run(g, rest[1:]); err != nil {
		writeError(g.JSON, err)
		return exitcode.Code(err)
	}
	return exitcode.Success
}

// writeError 统一输出错误。--json 模式输出
// {"error":{"code":N,"type":"…","message":"…"}}（合法 JSON、schema 稳定）到
// stdout；文本模式输出"错误：…"到 stderr。
func writeError(jsonMode bool, err error) {
	if jsonMode {
		out, mErr := json.Marshal(map[string]any{
			"error": map[string]any{
				"code":    exitcode.Code(err),
				"type":    exitcode.Type(err),
				"message": err.Error(),
			},
		})
		if mErr != nil {
			fmt.Fprintf(os.Stderr, "错误：%v\n", err)
			return
		}
		fmt.Println(string(out))
		return
	}
	fmt.Fprintln(os.Stderr, "错误："+err.Error())
}
