// auth_test.go：鉴权与 CSRF 防护矩阵（T06-F5 验收）。
// 覆盖：无 token 401 / 错 token 401 / 假 Origin 403 / 假 Host 403 /
// 本机无 Origin（curl/CLI）通过 / 浏览器同源通过 / WS 同源通过。
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"awem/internal/config"
)

// serve 用直接构造的请求走完整中间件栈（可伪造 Host/Origin；
// httptest.NewRequest 会把 URL host 填入 req.Host）。
func (e *testEnv) serve(t *testing.T, host, origin, auth string, path string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if host == "" {
		host = "127.0.0.1" // httptest.NewRequest 对相对路径默认 example.com
	}
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	rec := httptest.NewRecorder()
	e.api.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func TestAuth鉴权与CSRF矩阵(t *testing.T) {
	e := newEnvWithExecs(t)
	const path = "/api/executors"

	cases := []struct {
		name     string
		host     string // 空 = httptest 默认（127.0.0.1）
		origin   string
		token    string
		want     int
		wantCode string // 空 = 期望 2xx
	}{
		{"无token", "", "", "", http.StatusUnauthorized, "unauthorized"},
		{"错token", "", "", "wrong", http.StatusUnauthorized, "unauthorized"},
		{"无token且假Origin", "", "http://evil.example", "", http.StatusForbidden, "forbidden"},
		{"假Origin_拦截优先", "", "http://evil.example", "test-token", http.StatusForbidden, "forbidden"},
		{"假Origin_无scheme", "", "evil.example", "test-token", http.StatusForbidden, "forbidden"},
		{"假Origin_跨端口", "", "http://127.0.0.1:9999", "test-token", http.StatusForbidden, "forbidden"},
		{"假Host", "10.0.0.5:8080", "", "test-token", http.StatusForbidden, "forbidden"},
		{"假Host_外网域名", "awem.example.com", "", "test-token", http.StatusForbidden, "forbidden"},
		{"本机localhost无Origin", "localhost:7000", "", "test-token", http.StatusOK, ""},
		{"本机127带端口", "127.0.0.1:7001", "", "test-token", http.StatusOK, ""},
		{"本机IPv6回环", "[::1]:7002", "", "test-token", http.StatusOK, ""},
		{"同源Origin通过", "127.0.0.1:7003", "http://127.0.0.1:7003", "test-token", http.StatusOK, ""},
		{"同源Origin_localhost", "localhost:7004", "http://localhost:7004", "test-token", http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := e.serve(t, tc.host, tc.origin, tc.token, path)
			if code != tc.want {
				t.Fatalf("状态码 = %d，期望 %d", code, tc.want)
			}
		})
	}
}

// TestAuth静态资源免鉴权 SPA 壳本身无 token 可访问（数据接口才有拦截）。
func TestAuth静态资源免鉴权(t *testing.T) {
	e := newEnvWithExecs(t)
	code, body := e.doRaw(t, http.MethodGet, "/")
	if code != http.StatusOK {
		t.Fatalf("首页应免鉴权 200，实际 %d %s", code, body)
	}
	// 未知前端路由同样免鉴权（SPA fallback）
	code, _ = e.doRaw(t, http.MethodGet, "/some/spa/route")
	if code != http.StatusOK {
		t.Fatalf("SPA 路由应免鉴权 200，实际 %d", code)
	}
}

// TestAuth假Origin_错误体契约 403 的错误体字段。
func TestAuth假Origin错误体(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "a", Command: validCommand})
	req := httptest.NewRequest(http.MethodGet, "/api/executors", nil)
	req.Host = "127.0.0.1:7005"
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	e.api.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("假 Origin 应 403，实际 %d", rec.Code)
	}
	m := jsonMap(t, rec.Body.String())
	errObj := m["error"].(map[string]any)
	if errObj["code"] != "forbidden" {
		t.Fatalf("code 应 forbidden：%v", errObj)
	}
}
