// commands.go：CLI 子命令集的共享装配（T07）。全部数据操作走本地 API
// （红线：不绕过 API 直改状态），依赖经 Deps 注入（生产真连 daemon，
// 测试直连 httptest 假 daemon）。
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"awem/internal/config"
	"awem/internal/daemon"
	"awem/internal/exitcode"
)

// Deps 是子命令集的可注入依赖。
type Deps struct {
	Layout  func() (config.Paths, error)                      // 目录布局（默认 config.DefaultLayout）
	Connect func(ctx context.Context) (*daemon.Client, error) // API 连接（默认 daemon.Connect 惰性拉起）
	In      io.Reader                                         // 确认交互输入（默认 os.Stdin）
	Out     io.Writer                                         // 正常输出（默认 os.Stdout）
	HomeDir func() (string, error)                            // 家目录（import 归档旧布局用；默认 os.UserHomeDir）
}

// DefaultDeps 返回生产依赖。
func DefaultDeps() Deps {
	return Deps{
		Layout:  config.DefaultLayout,
		Connect: func(ctx context.Context) (*daemon.Client, error) { return nil, nil }, // RegisterCommands 内包装
		In:      os.Stdin,
		Out:     os.Stdout,
	}
}

// RegisterCommands 注册全部 API 依赖型子命令（add/list/status/start/stop/
// restart/edit/remove/log/import/doctor/reload）。d 零值字段取默认。
func RegisterCommands(r *Registry, d Deps) {
	if d.Layout == nil {
		d.Layout = config.DefaultLayout
	}
	if d.Connect == nil {
		d.Connect = func(ctx context.Context) (*daemon.Client, error) {
			p, err := config.DefaultLayout()
			if err != nil {
				return nil, exitcode.Wrap(exitcode.General, err)
			}
			return daemon.Connect(ctx, daemon.ClientOptions{Paths: p})
		}
	}
	if d.In == nil {
		d.In = os.Stdin
	}
	if d.Out == nil {
		d.Out = os.Stdout
	}
	if d.HomeDir == nil {
		d.HomeDir = os.UserHomeDir
	}
	reg := func(c *Command) { r.Register(c) }
	reg(cmdAdd(d))
	reg(cmdList(d))
	reg(cmdStatus(d))
	reg(cmdAction(d, "start"))
	reg(cmdAction(d, "stop"))
	reg(cmdAction(d, "restart"))
	reg(cmdEdit(d))
	reg(cmdRemove(d))
	reg(cmdLog(d))
	reg(cmdImport(d))
	reg(cmdDoctor(d))
	// 后续功能点逐个注册：reload（main.go 已注册，F10 收口统一）
}

// printf 向 Deps.Out 输出。
func (d Deps) printf(format string, args ...any) {
	fmt.Fprintf(d.Out, format, args...)
}

// out 返回输出目标（tabwriter 等需要 io.Writer 的场合）。
func (d Deps) out() io.Writer { return d.Out }

// usagef/generalf 是 exitcode 构造器的薄包装（子命令文件统一入口）。
func usagef(format string, args ...any) error   { return exitcode.Usagef(format, args...) }
func generalf(format string, args ...any) error { return exitcode.Generalf(format, args...) }

// confirm 交互确认：prompt 以"…[y/N]"结尾时输入 y/Y/是 才算确认。
// 测试通过注入 d.In 驱动。
func (d Deps) confirm(prompt string) bool {
	d.printf("%s [y/N]：", prompt)
	var answer string
	if _, err := fmt.Fscanln(d.In, &answer); err != nil && answer == "" {
		return false // EOF/空输入按拒绝
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes" || answer == "是"
}

// wrapAPIErr 把 daemon.APIError 转为带退出码的 CLI 错误；其余错误原样。
func wrapAPIErr(err error) error {
	var ae *daemon.APIError
	if ok := asAPIError(err, &ae); ok {
		return exitcode.Wrap(ae.ExitCode(), err)
	}
	return err
}

// asAPIErr 是 errors.As 的薄包装（减少 cli 包对 errors 的散用）。
func asAPIError(err error, target **daemon.APIError) bool {
	if ae, ok := err.(*daemon.APIError); ok {
		*target = ae
		return true
	}
	return false
}

// printJSON 输出缩进 JSON（--json 模式）。
func (d Deps) printJSON(data []byte) {
	var buf bytes.Buffer
	if json.Indent(&buf, data, "", "  ") == nil {
		d.printf("%s\n", buf.String())
		return
	}
	d.printf("%s\n", data)
}
