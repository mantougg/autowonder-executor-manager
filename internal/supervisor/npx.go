// npx.go：npx 可执行文件解析策略（T05-F1，跨平台差异在 npx_unix/windows.go）。
// Linux/macOS：exec.LookPath("npx")，失败回退登录 shell 的 PATH（`sh -lc
// 'command -v npx'`）——覆盖 GUI/系统服务启动场景（profile 注入的 nvm/mise
// PATH 不在 daemon 环境里）。Windows：LookPath("npx.cmd"→"npx.exe"→"npx")。
// 全部失败返回 ErrNpxNotFound（中文、指向 awem doctor）。
package supervisor

import "errors"

// NpxResolver 解析 npx 可执行文件绝对路径；失败应返回包装了 ErrNpxNotFound
// 的错误（便于 CLI/API 层归类）。测试注入假 npx 路径用。
type NpxResolver func() (string, error)

// ErrNpxNotFound 表示本机找不到 npx（环境问题，非配置问题）。
var ErrNpxNotFound = errors.New("找不到 npx：请先安装 Node.js（≥ 20），再运行 awem doctor 检查本机环境")

// DefaultNpxResolver 返回当前平台的默认 npx 解析策略。
func DefaultNpxResolver() NpxResolver { return defaultNpxResolver() }
