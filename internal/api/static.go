package api

import (
	"net/http"

	"awem/web"
)

// IndexHandler 返回内嵌 Web 首页（web/dist/index.html）的最小 handler。
// 任意路径均回 index.html（SPA 形态）；token 鉴权与完整静态资源路由在 T06 实现。
func IndexHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := web.IndexHTML()
		if err != nil {
			http.Error(w, "内嵌页面缺失："+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}
