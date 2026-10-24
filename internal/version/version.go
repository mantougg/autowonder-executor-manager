// Package version 保存 awem 版本号。
// 默认值 0.0.0-dev，正式构建通过 -ldflags "-X awem/internal/version.Version=…"
// 注入（见 Makefile）。
package version

// Version 是当前二进制版本号，构建时由 ldflags 注入。
var Version = "0.0.0-dev"
