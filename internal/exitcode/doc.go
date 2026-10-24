// Package exitcode 定义 awem 的进程退出码契约与对应错误类型：
// 0 成功 / 2 用法错误 / 3 未找到 / 4 已存在 / 5 daemon 未运行 / 1 其他。
//
// 新错误一律经 New / Usagef / NotFoundf / AlreadyExistsf /
// DaemonNotRunningf / Generalf 构造；进程退出前用 Code(err) 取码，
// 禁止绕过本包 os.Exit 裸数字。
package exitcode
