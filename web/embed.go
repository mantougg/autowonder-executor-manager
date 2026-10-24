// Package web 内嵌 Web 前端构建产物（web/dist/），供 internal/api 提供
// 静态资源服务。dist/ 约定"构建时生成、仓库忽略"（见 Makefile 的
// web-dist 目标：缺失时从 web/placeholder/ 复制占位页），
// 本文件是 web/ 下唯一入库的 Go 源码，embed 路径固定为 dist。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS 返回以 web/dist/ 内容为根的只读文件系统。
func DistFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist 与本文件同包编译，目录必然存在，仅防御性处理
		panic("embed 目录 dist 不存在：" + err.Error())
	}
	return sub
}

// IndexHTML 返回内嵌的 SPA 入口页 index.html 内容。
func IndexHTML() ([]byte, error) {
	return fs.ReadFile(DistFS(), "index.html")
}
