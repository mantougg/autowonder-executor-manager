// stats_test.go：运行时间带 API（timeline）、日志按位读取与平台小队数组契约。
package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"awem/internal/config"
	"awem/internal/logs"
	"awem/internal/platform"
)

func utc(h, m int) time.Time { return time.Date(2026, 10, 25, h, m, 0, 0, time.UTC) }

func cfgExec(name string) config.Executor {
	return config.Executor{
		Name: name, Command: validCommand,
		Restart: config.RestartPolicy{Enabled: config.BoolPtr(true), Backoff: "exponential"},
	}
}

// recordEvents 写入事件日志（时间可控）。
func recordEvents(t *testing.T, e *testEnv, evs ...logs.Event) {
	t.Helper()
	el, err := e.mgr.EventLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range evs {
		if err := el.RecordEvent(ev); err != nil {
			t.Fatal(err)
		}
	}
}

func timelineURL(name string, from, to time.Time) string {
	q := url.Values{}
	q.Set("from", from.Format(time.RFC3339))
	q.Set("to", to.Format(time.RFC3339))
	return "/api/stats/executors/" + url.PathEscape(name) + "/timeline?" + q.Encode()
}

// timeline 端点：鉴权、404/400、重放语义、崩溃定位标识。
func TestTimelineAPI(t *testing.T) {
	e := newEnvWithExecs(t, cfgExec("e1"))
	auto := true
	recordEvents(t, e,
		logs.Event{Time: utc(10, 0), Type: logs.EventExecutorStart, Executor: "e1"},
		logs.Event{Time: utc(10, 30), Type: logs.EventExecutorCrash, Executor: "e1",
			Detail: "崩溃退出（exit 1）", Restart: &auto, LogFile: "e1.log", LogOffset: 10},
		logs.Event{Time: utc(10, 31), Type: logs.EventExecutorRestart, Executor: "e1"},
	)
	if err := os.MkdirAll(e.p.ExecutorsLogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.p.ExecutorsLogDir, "e1.log"), []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, body := e.do(t, http.MethodGet, timelineURL("e1", utc(10, 0), utc(11, 0)), "")
	if code != http.StatusOK {
		t.Fatalf("timeline 应 200，实际 %d %s", code, body)
	}
	m := jsonMap(t, body)
	if m["name"] != "e1" {
		t.Fatalf("name 不符：%v", m["name"])
	}
	if m["data_start"] == nil {
		t.Fatalf("应返回数据起点：%v", m)
	}
	segs := m["segments"].([]any)
	if len(segs) != 3 {
		t.Fatalf("应返回 3 段状态区间，实际 %d：%v", len(segs), segs)
	}
	first := segs[0].(map[string]any)
	if first["state"] != "running" {
		t.Fatalf("首段应为 running：%v", first)
	}
	crashes := m["crashes"].([]any)
	if len(crashes) != 1 {
		t.Fatalf("应返回 1 个崩溃点：%v", crashes)
	}
	c := crashes[0].(map[string]any)
	if c["log_file"] != "e1.log" || c["locatable"] != true {
		t.Fatalf("崩溃定位标识不符：%v", c)
	}

	// 未知执行器 404
	code, body = e.do(t, http.MethodGet, timelineURL("nope", utc(10, 0), utc(11, 0)), "")
	if code != http.StatusNotFound || jsonMap(t, body)["error"].(map[string]any)["code"] != "not_found" {
		t.Fatalf("未知执行器应 404 not_found，实际 %d %s", code, body)
	}

	// 非法/缺失时间参数 400
	for _, path := range []string{
		"/api/stats/executors/e1/timeline",
		"/api/stats/executors/e1/timeline?from=bad&to=2026-10-25T11:00:00Z",
		"/api/stats/executors/e1/timeline?from=2026-10-25T12:00:00Z&to=2026-10-25T11:00:00Z",
	} {
		code, body = e.do(t, http.MethodGet, path, "")
		if code != http.StatusBadRequest {
			t.Fatalf("%s 应 400，实际 %d %s", path, code, body)
		}
		if jsonMap(t, body)["error"].(map[string]any)["code"] != "bad_request" {
			t.Fatalf("%s 错误 code 应为 bad_request：%s", path, body)
		}
	}

	// 鉴权：无 token 401
	if code, body := e.doRaw(t, http.MethodGet, timelineURL("e1", utc(10, 0), utc(11, 0))); code != http.StatusUnauthorized {
		t.Fatalf("timeline 无 token 应 401，实际 %d %s", code, body)
	}
}

// 崩溃缺少定位信息 / 文件已清理 → locatable=false。
func TestTimelineCrashNotLocatable(t *testing.T) {
	e := newEnvWithExecs(t, cfgExec("e1"))
	auto := true
	recordEvents(t, e,
		logs.Event{Time: utc(10, 0), Type: logs.EventExecutorStart, Executor: "e1"},
		logs.Event{Time: utc(10, 30), Type: logs.EventExecutorCrash, Executor: "e1", Restart: &auto},
		logs.Event{Time: utc(10, 31), Type: logs.EventExecutorRestart, Executor: "e1"},
		logs.Event{Time: utc(10, 40), Type: logs.EventExecutorCrash, Executor: "e1", Restart: &auto,
			LogFile: "e1.log", LogOffset: 3}, // 文件不存在（已清理）
	)
	code, body := e.do(t, http.MethodGet, timelineURL("e1", utc(10, 0), utc(11, 0)), "")
	if code != http.StatusOK {
		t.Fatalf("timeline 应 200，实际 %d %s", code, body)
	}
	crashes := jsonMap(t, body)["crashes"].([]any)
	if len(crashes) != 2 {
		t.Fatalf("应有 2 个崩溃点：%v", crashes)
	}
	for _, raw := range crashes {
		if c := raw.(map[string]any); c["locatable"] != false {
			t.Fatalf("不可定位崩溃应 locatable=false：%v", c)
		}
	}
}

// 日志按位读取：file/before/limit 返回定位页；非法 file 400；旧 tail 行为保持。
func TestLogPositionAPI(t *testing.T) {
	e := newEnvWithExecs(t, cfgExec("e1"))
	if err := os.MkdirAll(e.p.ExecutorsLogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.p.ExecutorsLogDir, "e1.log"), []byte("aaaa\nbbbb\ncccc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, body := e.do(t, http.MethodGet, "/api/executors/e1/log?file=e1.log&before=10&limit=2", "")
	if code != http.StatusOK {
		t.Fatalf("按位读取应 200，实际 %d %s", code, body)
	}
	m := jsonMap(t, body)
	lines := m["lines"].([]any)
	if len(lines) != 2 || lines[0] != "aaaa" || lines[1] != "bbbb" {
		t.Fatalf("定位行不符：%v", lines)
	}
	if m["start_file"] != "e1.log" || m["start_offset"].(float64) != 0 {
		t.Fatalf("首行位置不符：%v", m)
	}

	for _, path := range []string{
		"/api/executors/e1/log?file=../events.log&before=10",
		"/api/executors/e1/log?file=other.log&before=10",
		"/api/executors/e1/log?before=10",
		"/api/executors/e1/log?file=e1.log&before=-1",
		"/api/executors/e1/log?file=e1.log&before=10&limit=abc",
	} {
		code, body = e.do(t, http.MethodGet, path, "")
		if code != http.StatusBadRequest {
			t.Fatalf("%s 应 400，实际 %d %s", path, code, body)
		}
	}

	// 旧 tail 契约不变
	code, body = e.do(t, http.MethodGet, "/api/executors/e1/log?tail=2", "")
	if code != http.StatusOK {
		t.Fatalf("tail 应 200，实际 %d %s", code, body)
	}
	m = jsonMap(t, body)
	if m["tail"].(float64) != 2 || len(m["lines"].([]any)) != 2 {
		t.Fatalf("tail 契约变化：%v", m)
	}
}

// 平台视图增量 squad_names（数组），squad 拼接串保持兼容。
func TestAPI平台小队数组字段(t *testing.T) {
	e := newEnvWithExecs(t, cfgExec("e1"))
	e.api = New(Options{
		Paths: e.p, Token: "test-token", Logs: e.mgr, Sup: e.sup,
		Watcher: e.w,
		Plat: fakeSyncer{"10086": platform.Status{
			State: "ONLINE", Name: "NO1", AgentName: "全栈",
			Project: "项目甲", Squad: "小队一、小队二", SquadNames: []string{"小队一", "小队二"},
		}}, Port: 18765, Version: "test-1.2.3",
	})
	e.srv = httptest.NewServer(e.api.Handler())
	t.Cleanup(e.srv.Close)

	code, body := e.do(t, http.MethodGet, "/api/executors/e1", "")
	if code != http.StatusOK {
		t.Fatalf("应 200，实际 %d %s", code, body)
	}
	p := jsonMap(t, body)["executor"].(map[string]any)["platform"].(map[string]any)
	if p["squad"] != "小队一、小队二" {
		t.Fatalf("squad 拼接串应保持兼容：%v", p["squad"])
	}
	names, ok := p["squad_names"].([]any)
	if !ok || len(names) != 2 || names[0] != "小队一" || names[1] != "小队二" {
		t.Fatalf("squad_names 应为字符串数组：%v", p["squad_names"])
	}
}
