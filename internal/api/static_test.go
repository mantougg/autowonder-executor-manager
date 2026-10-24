package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awem/internal/config"
	"awem/internal/logs"
	"awem/internal/platform"
)

// TestServeStatic返回内嵌页 任意前端路径 → 200 与内嵌 SPA 页（F7 完整
// fallback 语义在此验证：静态命中与未命中均可用）。
func TestServeStatic返回内嵌页(t *testing.T) {
	s := New(testOptions(t, nil))
	for _, path := range []string{"/", "/index.html", "/任意未定义路由"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
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

// testOptions 构造测试用 Options（静态测试无执行器）。
func testOptions(t *testing.T, execs []config.Executor) Options {
	t.Helper()
	dir := t.TempDir()
	p := config.At(dir)
	cfg := config.Default()
	cfg.Executors = execs
	if err := cfg.Validate(); err != nil {
		t.Fatalf("测试配置不合法：%v", err)
	}
	if err := config.Save(p.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	mgr := logs.NewManager(p.ExecutorsLogDir, p.EventsLog, 0, 0, 0)
	t.Cleanup(mgr.Close)
	w := config.NewWatcher(p.ConfigFile, cfg)
	sup := newTestSupervisor(mgr)
	t.Cleanup(sup.Shutdown)
	return Options{
		Paths: p, Token: "test-token", Logs: mgr, Sup: sup,
		Watcher: w, Plat: platform.Noop{}, Port: 0,
	}
}
