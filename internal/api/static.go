// static.go：内嵌 SPA 静态资源服务（T01-F5 骨架，T06 增强）。
// / 与未知前端路由 → index.html（SPA fallback）；其余命中 web/dist 内文件
// 原样返回。完整 fallback 语义（/api、/ws 除外）在 F7 收口。
package api

import (
	"io/fs"
	"net/http"
	"strings"

	"awem/web"
)

// serveStatic 服务内嵌静态资源：先按静态文件查找，未命中回 index.html。
// 免鉴权（SPA 壳本身无数据；数据接口在 /api、/ws 下全拦）。
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	dist := web.DistFS()
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if data, err := fs.ReadFile(dist, name); err == nil {
		w.Header().Set("Content-Type", mimeOf(name))
		_, _ = w.Write(data)
		return
	}
	// SPA fallback：未知前端路由回首页（客户端路由接管）
	data, err := web.IndexHTML()
	if err != nil {
		http.Error(w, "内嵌页面缺失："+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// mimeOf 按扩展名给 Content-Type（构建产物只含常见 Web 资源类型）。
func mimeOf(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}
