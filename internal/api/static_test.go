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
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Fatalf("GET %s Cache-Control = %q，期望 no-cache（HTML 壳每次重校验）", path, cc)
		}
	}
}

// TestCacheControlOf 资源缓存策略：assets/ 内容哈希长缓存，其余稳定文件名 no-cache。
func TestCacheControlOf(t *testing.T) {
	for name, want := range map[string]string{
		"index.html":            "no-cache",
		"favicon.svg":           "no-cache",
		"assets/index-Ab12.js":  "public, max-age=31536000, immutable",
		"assets/index-Cd34.css": "public, max-age=31536000, immutable",
	} {
		if got := cacheControlOf(name); got != want {
			t.Fatalf("cacheControlOf(%q) = %q，期望 %q", name, got, want)
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
