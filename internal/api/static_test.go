package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestIndexHandler返回内嵌页 验证任意路径都能拿到 200 与嵌入的占位页内容。
func TestIndexHandler返回内嵌页(t *testing.T) {
	h := IndexHandler()
	for _, path := range []string{"/", "/index.html", "/任意未定义路径"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s 状态码 = %d，期望 200", path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "awem") {
			t.Fatalf("GET %s 响应缺少 awem 字样：%s", path, body)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("GET %s Content-Type = %q，期望 text/html", path, ct)
		}
	}
}
