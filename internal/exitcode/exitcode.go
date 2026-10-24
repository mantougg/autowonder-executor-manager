// Package exitcode 定义 awem 的退出码常量（design.md §8.2）与携带
// 退出码的错误类型。所有子命令与内部逻辑的报错只允许用本包类型包装，
// 由 CLI 分发层统一映射为退出码并格式化输出（中文文本 / --json 结构）。
// 独立成包（而非放 internal/cli）是为了让 internal/api 等非 CLI 层
// 也能构造带退出码语义的错误而不反向依赖 CLI 框架。
package exitcode

import (
	"errors"
	"fmt"
)

// 退出码常量，取值与 design.md §8.2 表完全一致，不可变更。
const (
	Success          = 0 // 成功
	General          = 1 // 一般错误
	Usage            = 2 // 用法错误
	NotFound         = 3 // 未找到
	AlreadyExists    = 4 // 已存在
	DaemonNotRunning = 5 // daemon 未运行（且无法自动拉起）
)

// kindName 是各退出码的机器可读类型名，作为 --json 错误输出 error.type
// 字段（schema 稳定，只增不改）。
var kindName = map[int]string{
	Success:          "success",
	General:          "general",
	Usage:            "usage",
	NotFound:         "not_found",
	AlreadyExists:    "already_exists",
	DaemonNotRunning: "daemon_not_running",
}

// KindName 返回退出码的类型名；未知码回退 "general"。
func KindName(code int) string {
	if name, ok := kindName[code]; ok {
		return name
	}
	return kindName[General]
}

// Error 是携带退出码的错误。实现 error 与 Unwrap，
// message 由构造时格式化生成。
type Error struct {
	Code int   // 退出码
	msg  string // 中文错误信息
	Err  error // 底层原因，可为 nil
}

// Error 实现 error 接口。
func (e *Error) Error() string { return e.msg }

// Unwrap 支持 errors.Is / errors.As 穿透到底层错误。
func (e *Error) Unwrap() error { return e.Err }

// New 构造带退出码的错误。
func New(code int, format string, args ...any) *Error {
	return &Error{Code: code, msg: fmt.Sprintf(format, args...)}
}

// Usagef 构造用法错误（退出码 2）。
func Usagef(format string, args ...any) *Error { return New(Usage, format, args...) }

// NotFoundf 构造未找到错误（退出码 3）。
func NotFoundf(format string, args ...any) *Error { return New(NotFound, format, args...) }

// AlreadyExistsf 构造已存在错误（退出码 4）。
func AlreadyExistsf(format string, args ...any) *Error { return New(AlreadyExists, format, args...) }

// DaemonNotRunningf 构造 daemon 未运行错误（退出码 5）。
func DaemonNotRunningf(format string, args ...any) *Error {
	return New(DaemonNotRunning, format, args...)
}

// Generalf 构造一般错误（退出码 1）。
func Generalf(format string, args ...any) *Error { return New(General, format, args...) }

// Wrap 用指定退出码包装底层错误（信息为底层错误原文）。
func Wrap(code int, err error) *Error {
	return &Error{Code: code, msg: err.Error(), Err: err}
}

// Code 从错误链中提取退出码；非本包错误按一般错误（1）处理。
func Code(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return General
}

// Type 返回错误对应的类型名（与 KindName(Code(err)) 一致）。
func Type(err error) string { return KindName(Code(err)) }
