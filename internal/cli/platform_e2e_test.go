// platform_e2e_test.go：T08-F6 端到端验收。假平台（httptest MCP，按
// tools/call 的 name 分发）+ 真 Service 注入 cliEnv → list/status 显示
// 平台状态与漂移；doctor 三态；撤下联动回本地态。
package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"awem/internal/config"
	"awem/internal/launchcmd"
	"awem/internal/platform"
)

// fakePlatform 假 MCP（最小形态：list_projects / list_executors /
// get_executor_launch_config 三工具按 name 分发；状态与模型可变）。
type fakePlatform struct {
	srv    *httptest.Server
	mu     sync.Mutex
	status string
	model  string
}

func newFakePlatform(t *testing.T) *fakePlatform {
	t.Helper()
	fp := &fakePlatform{status: "ONLINE", model: "dfmodel"} // 与本地 qfmodel 不同 → 漂移
	mux := http.NewServeMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		text := func(s string) string {
			out, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]any{"content": []map[string]any{{"type": "text", "text": s}}},
			})
			return string(out)
		}
		fp.mu.Lock()
		st, model := fp.status, fp.model
		fp.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch req.Params.Name {
		case "autowonder.list_projects":
			_, _ = w.Write([]byte(text(`{"items":[{"id":10000,"name":"ws"}]}`)))
		case "autowonder.list_executors":
			_, _ = w.Write([]byte(text(`{"items":[{"id":10086,"name":"NO1快速全栈开发","agentId":10007,"agentName":"快速全栈开发","status":"` + st + `","lastHeartbeat":1792872206969,"squadNames":["标准自动化交付"]}]}`)))
		case "autowonder.get_executor_launch_config":
			_, _ = w.Write([]byte(text(`{"model":"` + model + `","reasoningEffort":"medium","contextWindow":"1000000"}`)))
		default:
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		}
	}
	mux.HandleFunc("/mcp", handler)
	mux.HandleFunc("/api/mcp/tok-1/", handler) // 真实平台令牌路径风格（NewService 端点推导）
	fp.srv = httptest.NewServer(mux)
	t.Cleanup(fp.srv.Close)
	return fp
}

func (fp *fakePlatform) setStatus(status string) {
	fp.mu.Lock()
	fp.status = status
	fp.mu.Unlock()
}

// attach 给 cliEnv 装配假平台联动并刷新一轮（返回 Service 供后续手动刷新）。
func (e *cliEnv) attach(t *testing.T, fp *fakePlatform) *platform.Service {
	t.Helper()
	var locals []platform.LocalExecutor
	for _, ex := range e.localExecs {
		if p, err := launchcmd.Parse(ex.Command); err == nil {
			locals = append(locals, platform.LocalExecutor{
				ExecutorID: p.ExecutorID, Model: p.Model, ReasoningEffort: p.ReasoningEffort,
				ContextWindow: p.ContextWindow, MemoryMode: p.MemoryMode, MaxTasks: p.MaxTasks,
			})
		}
	}
	svc := platform.NewService(platform.Options{
		Config:    &platform.SyncerConfig{BaseURL: fp.srv.URL, Token: "tok-1", Interval: time.Hour},
		CachePath: t.TempDir() + "/c.json",
		LocalIDs:  func() []platform.LocalExecutor { return locals },
	})
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("假平台刷新失败：%v", err)
	}
	e.setPlatform(t, svc)
	e.currentSvc = svc
	return svc
}

func Test端到端平台状态与漂移展示(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "pe", Command: validCommand}) // id 10086, model qfmodel
	fp := newFakePlatform(t)
	svc := e.attach(t, fp)

	// list：平台状态 + 漂移黄标
	if code := e.run(false, "list"); code != 0 {
		t.Fatalf("list 失败：%d %s", code, e.output())
	}
	out := e.output()
	for _, want := range []string{"pe", "ONLINE", "⚠1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list 应含 %q：\n%s", want, out)
		}
	}
	// status 详情：平台状态 + 漂移字段双侧值 + 文案
	if code := e.run(false, "status", "pe"); code != 0 {
		t.Fatalf("status 失败：%d", code)
	}
	out = e.output()
	for _, want := range []string{
		"平台状态：ONLINE",
		"配置漂移：⚠ 与平台不一致 1 项",
		`--model：本地 "qfmodel" vs 平台 "dfmodel"`,
		"需 restart 或平台配置已变",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status 详情应含 %q：\n%s", want, out)
		}
	}
	// --json：platform 与 drift 结构就位
	if code := e.run(true, "status", "pe"); code != 0 {
		t.Fatalf("status --json：%d", code)
	}
	out = e.output()
	for _, want := range []string{`"platform"`, `"drift"`, "dfmodel"} {
		if !strings.Contains(out, want) {
			t.Fatalf("--json 应含 %s：%s", want, out)
		}
	}
	// 平台状态变化 → 手动刷新后展示跟随
	fp.setStatus("BUSY")
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if code := e.run(false, "list"); code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(e.output(), "BUSY") {
		t.Fatalf("状态变化应反映：%s", e.output())
	}
}

func Test端到端撤下联动回本地态(t *testing.T) {
	withGreenDoctorEnv(t) // doctor 断言退出 0：预置 qoder 执行器需注入绿环境
	e := newCLIEnv(t, config.Executor{Name: "pe2", Command: validCommand})
	fp := newFakePlatform(t)
	e.attach(t, fp)
	if code := e.run(false, "list"); code != 0 || !strings.Contains(e.output(), "ONLINE") {
		t.Fatalf("联动应生效：%s", e.output())
	}
	// 撤下：Noop（等价未配置 [platform] 段）
	e.setPlatform(t, nil)
	if code := e.run(false, "list"); code != 0 {
		t.Fatal(code)
	}
	out := e.output()
	if !strings.Contains(out, "pe2") || strings.Contains(out, "ONLINE") {
		t.Fatalf("撤下后应只有本地态：\n%s", out)
	}
	// doctor 回到跳过态、全部通过
	if code := e.run(false, "doctor"); code != 0 {
		t.Fatalf("doctor 应通过：%d %s", code, e.output())
	}
	if !strings.Contains(e.output(), "— 平台连通：未配置") {
		t.Fatalf("doctor 平台项应回跳过态：%s", e.output())
	}
}

func Test端到端Doctor平台三态(t *testing.T) {
	withGreenDoctorEnv(t) // 同上：不依赖宿主 qoder
	e := newCLIEnv(t, config.Executor{Name: "pd", Command: validCommand})
	fp := newFakePlatform(t)
	e.attach(t, fp)
	// 配置了且可达 → ✅
	if code := e.run(false, "doctor"); code != 0 {
		t.Fatalf("doctor 应通过：%d\n%s", code, e.output())
	}
	if !strings.Contains(e.output(), "✅ 平台连通") {
		t.Fatalf("应显示平台可达：%s", e.output())
	}
	// 断服 → ❌ 且退出 1（Ping 打已关闭的端点）
	fp.srv.Close()
	if code := e.run(false, "doctor"); code != 1 {
		t.Fatalf("平台断连 doctor 应退出 1，实际 %d\n%s", code, e.output())
	}
	if !strings.Contains(e.output(), "❌ 平台连通") {
		t.Fatalf("应显示平台不可达：%s", e.output())
	}
}
