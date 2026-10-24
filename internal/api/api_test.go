// api_test.go：REST API 验收测试（T06-F3）。假 npx = testdata/fakeproc
// 构建产物（与 supervisor 包同款），start/stop/restart/delete 走真子进程。
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"awem/internal/config"
	"awem/internal/logs"
	"awem/internal/platform"
	"awem/internal/supervisor"
)

// fakeNpxPath 由 TestMain 构建。
var fakeNpxPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "awem-api-fakeproc")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "fakeproc")
	build := exec.Command("go", "build", "-o", out, "./testdata/fakeproc")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(fmt.Sprintf("构建 fakeproc 失败：%v", err))
	}
	fakeNpxPath = out
	os.Exit(m.Run())
}

// newTestSupervisor 构造注入假 npx 的 supervisor（快退避参数）。
func newTestSupervisor(mgr *logs.Manager) *supervisor.Supervisor {
	return supervisor.New(supervisor.Options{
		Logs: mgr,
		Backoff: supervisor.BackoffParams{
			Initial: 20 * time.Millisecond, Max: 80 * time.Millisecond,
			StableReset: 150 * time.Millisecond, Grace: 2 * time.Second,
		},
		ResolveNpx: func() (string, error) { return fakeNpxPath, nil },
		HomeDir:    func() (string, error) { return "/tmp", nil },
	})
}

// testEnv 聚合一个可测的 API 环境（httptest.Server + 真落盘配置）。
type testEnv struct {
	srv  *httptest.Server
	api  *Server
	p    config.Paths
	mgr  *logs.Manager
	sup  *supervisor.Supervisor
}

// newEnvWithExecs 构造环境：预置 execs 到 config.toml 并发布。
func newEnvWithExecs(t *testing.T, execs ...config.Executor) *testEnv {
	t.Helper()
	env := &testEnv{}
	dir := t.TempDir()
	env.p = config.At(dir)
	cfg := config.Default()
	cfg.Server.Port = 0
	cfg.Executors = execs
	if err := cfg.Validate(); err != nil {
		t.Fatalf("测试配置不合法：%v", err)
	}
	if err := config.Save(env.p.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	env.mgr = logs.NewManager(env.p.ExecutorsLogDir, env.p.EventsLog, 0, 0, 0)
	t.Cleanup(env.mgr.Close)
	w := config.NewWatcher(env.p.ConfigFile, cfg)
	env.sup = newTestSupervisor(env.mgr)
	t.Cleanup(env.sup.Shutdown)
	// 配置发布走与 daemon 一致的路径（订阅者收到新配置即 ApplyConfig）
	w.Subscribe(func(c config.Config) {
		cc := c
		env.sup.ApplyConfig(&cc)
	})
	env.sup.ApplyConfig(&cfg)

	env.api = New(Options{
		Paths: env.p, Token: "test-token", Logs: env.mgr, Sup: env.sup,
		Watcher: w, Plat: platform.Noop{}, Port: 18765, Version: "test-1.2.3",
	})
	env.srv = httptest.NewServer(env.api.Handler())
	t.Cleanup(env.srv.Close)
	return env
}

// do 发带鉴权请求，返回状态码与 body。
func (e *testEnv) do(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, readAll(resp)
}

// doRaw 不带鉴权。
func (e *testEnv) doRaw(t *testing.T, method, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, readAll(resp)
}

func readAll(resp *http.Response) string {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}

// jsonMap 解析 JSON 为 map（简化断言）。
func jsonMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("响应不是合法 JSON：%v\n%s", err, s)
	}
	return m
}

const validCommand = "npx -y autowonder@0.3.22 connect --ws-url ws://127.0.0.1:1/ws " +
	"--token secret-token-abcdef123456 --executor-id 10086 --provider qoder --model qfmodel --reasoning-effort medium --context-window 1000000 --token-aware-enable"

// --- F3 验收：鉴权基础（完整 401/403 矩阵在 F5） ---

func TestAPI未带token401(t *testing.T) {
	e := newEnvWithExecs(t)
	for _, path := range []string{"/api/executors", "/api/status", "/api/settings"} {
		code, body := e.doRaw(t, http.MethodGet, path)
		if code != http.StatusUnauthorized {
			t.Fatalf("GET %s 无 token 应 401，实际 %d %s", path, code, body)
		}
		m := jsonMap(t, body)
		errObj := m["error"].(map[string]any)
		if errObj["code"] != "unauthorized" {
			t.Fatalf("错误 code 应为 unauthorized，实际 %v", errObj["code"])
		}
	}
}

func TestAPI错token401(t *testing.T) {
	e := newEnvWithExecs(t)
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/executors", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错 token 应 401，实际 %d", resp.StatusCode)
	}
}

// --- F3 验收：列表/单条/掩码 ---

func TestAPI列表与token掩码(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{
		Name: "e1", Command: validCommand,
		Restart: config.RestartPolicy{Enabled: config.BoolPtr(true), Backoff: "exponential"},
	})
	code, body := e.do(t, http.MethodGet, "/api/executors", "")
	if code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d %s", code, body)
	}
	m := jsonMap(t, body)
	execs := m["executors"].([]any)
	if len(execs) != 1 {
		t.Fatalf("应返回 1 个执行器，实际 %d", len(execs))
	}
	ev := execs[0].(map[string]any)
	if ev["name"] != "e1" {
		t.Fatalf("name 应为 e1：%v", ev["name"])
	}
	parsed := ev["parsed"].(map[string]any)
	tok, _ := parsed["token"].(string)
	if tok == "" || strings.Contains(tok, "secret-token-abcdef123456") || !strings.Contains(tok, "***") {
		t.Fatalf("token 应为掩码形式，实际 %q", tok)
	}
	if ev["platform"] != nil {
		t.Fatalf("平台状态应为 null 占位，实际 %v", ev["platform"])
	}
	local := ev["local"].(map[string]any)
	if local["state"] != "stopped" {
		t.Fatalf("初始 state 应 stopped，实际 %v", local["state"])
	}
}

func TestAPI单条404(t *testing.T) {
	e := newEnvWithExecs(t)
	code, body := e.do(t, http.MethodGet, "/api/executors/nope", "")
	if code != http.StatusNotFound {
		t.Fatalf("不存在应 404，实际 %d %s", code, body)
	}
	if jsonMap(t, body)["error"].(map[string]any)["code"] != "not_found" {
		t.Fatal("错误 code 应为 not_found")
	}
}

// --- F3 验收：add 全链路 + 错误路径 ---

func TestAPIadd全链路(t *testing.T) {
	e := newEnvWithExecs(t)
	// 正常：解析→校验→落盘→发布→start
	body := `{"name":"new-exec.1","command":"` + validCommand + `","autostart":false,"start":true}`
	code, resp := e.do(t, http.MethodPost, "/api/executors", body)
	if code != http.StatusCreated {
		t.Fatalf("add 应 201，实际 %d %s", code, resp)
	}
	ev := jsonMap(t, resp)["executor"].(map[string]any)
	if ev["name"] != "new-exec.1" {
		t.Fatalf("返回视图 name 错误：%v", ev["name"])
	}
	// 落盘断言
	onDisk, err := config.Load(e.p.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.FindExecutor("new-exec.1") == nil {
		t.Fatal("add 后 config.toml 应包含新执行器")
	}
	// start=true → 假子进程运行中
	local := ev["local"].(map[string]any)
	if local["state"] != "running" {
		t.Fatalf("start=true 后 state 应 running，实际 %v（%s）", local["state"], resp)
	}
	// 幂等停止收口
	if _, resp2 := e.do(t, http.MethodPost, "/api/executors/new-exec.1/stop", ""); true {
		_ = resp2
	}
}

func TestAPIadd已存在409(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "dup", Command: validCommand})
	body := `{"name":"dup","command":"` + validCommand + `"}`
	code, resp := e.do(t, http.MethodPost, "/api/executors", body)
	if code != http.StatusConflict {
		t.Fatalf("重复 add 应 409，实际 %d %s", code, resp)
	}
	errObj := jsonMap(t, resp)["error"].(map[string]any)
	if errObj["code"] != "already_exists" || errObj["field"] != "name" {
		t.Fatalf("错误体不合契约：%v", errObj)
	}
}

func TestAPIadd命令逐字段错误(t *testing.T) {
	e := newEnvWithExecs(t)
	// executor-id 非数字 + token 缺失
	badCmd := "npx -y autowonder@0.3.22 connect --ws-url ws://127.0.0.1:1/ws --executor-id abc --provider qoder"
	code, resp := e.do(t, http.MethodPost, "/api/executors", `{"name":"x","command":"`+badCmd+`"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("坏命令应 400，实际 %d %s", code, resp)
	}
	errObj := jsonMap(t, resp)["error"].(map[string]any)
	if errObj["code"] != "invalid_command" {
		t.Fatalf("code 应 invalid_command：%v", errObj)
	}
	field, _ := errObj["field"].(string)
	if !strings.Contains(field, "executor-id") || !strings.Contains(field, "token") {
		t.Fatalf("field 应逐字段列出 executor-id 与 token：%q", field)
	}
	// 非法名称
	code, resp = e.do(t, http.MethodPost, "/api/executors", `{"name":"中文名","command":"`+validCommand+`"}`)
	if code != http.StatusBadRequest || jsonMap(t, resp)["error"].(map[string]any)["code"] != "invalid_name" {
		t.Fatalf("非法 name 应 400 invalid_name，实际 %d %s", code, resp)
	}
	// 非法 JSON
	code, resp = e.do(t, http.MethodPost, "/api/executors", `{坏json`)
	if code != http.StatusBadRequest || jsonMap(t, resp)["error"].(map[string]any)["code"] != "bad_request" {
		t.Fatalf("坏 JSON 应 400 bad_request，实际 %d %s", code, resp)
	}
}

// --- F3 验收：PUT 结构化编辑 ---

func TestAPIedit字段与命令替换(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "e1", Command: validCommand})
	// 字段级：改 model；token 提交掩码 → 保留
	code, resp := e.do(t, http.MethodPut, "/api/executors/e1",
		`{"fields":{"model":"dfmodel"},"autostart":true}`)
	if code != http.StatusOK {
		t.Fatalf("edit 应 200，实际 %d %s", code, resp)
	}
	ev := jsonMap(t, resp)["executor"].(map[string]any)
	if ev["autostart"] != true {
		t.Fatalf("autostart 应更新为 true：%v", ev["autostart"])
	}
	// 落盘命令含新 model，token 原值保留（掩码语义），未知 flag 不丢
	onDisk, _ := config.Load(e.p.ConfigFile)
	cmd := onDisk.FindExecutor("e1").Command
	if !strings.Contains(cmd, "--model dfmodel") {
		t.Fatalf("命令应含 --model dfmodel：%s", cmd)
	}
	if !strings.Contains(cmd, "secret-token-abcdef123456") {
		t.Fatalf("未提交 token 时原令牌不应丢失：%s", cmd)
	}
	if !strings.Contains(cmd, "--token-aware-enable") {
		t.Fatalf("布尔 flag 不应丢失：%s", cmd)
	}
	// 整条替换
	newCmd := strings.Replace(validCommand, "qfmodel", "other-model", 1)
	code, resp = e.do(t, http.MethodPut, "/api/executors/e1", `{"command":"`+newCmd+`"}`)
	if code != http.StatusOK {
		t.Fatalf("整条替换应 200，实际 %d %s", code, resp)
	}
	onDisk, _ = config.Load(e.p.ConfigFile)
	if !strings.Contains(onDisk.FindExecutor("e1").Command, "other-model") {
		t.Fatal("整条替换应生效")
	}
	// 404
	code, _ = e.do(t, http.MethodPut, "/api/executors/nope", `{"fields":{"model":"m"}}`)
	if code != http.StatusNotFound {
		t.Fatalf("编辑不存在应 404，实际 %d", code)
	}
}

func TestAPIedit掩码token不覆盖(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "e1", Command: validCommand})
	// 先取当前掩码，再原样提交
	_, body := e.do(t, http.MethodGet, "/api/executors/e1", "")
	masked := jsonMap(t, body)["executor"].(map[string]any)["parsed"].(map[string]any)["token"].(string)
	code, resp := e.do(t, http.MethodPut, "/api/executors/e1",
		`{"fields":{"token":"`+masked+`","model":"dfmodel"}}`)
	if code != http.StatusOK {
		t.Fatalf("掩码提交应成功，实际 %d %s", code, resp)
	}
	onDisk, _ := config.Load(e.p.ConfigFile)
	if !strings.Contains(onDisk.FindExecutor("e1").Command, "secret-token-abcdef123456") {
		t.Fatalf("掩码 token 不应覆盖原令牌：%s", onDisk.FindExecutor("e1").Command)
	}
}

// --- F3 验收：动作路由（真子进程） ---

func TestAPI动作startStopRestart(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "act", Command: validCommand})
	// start → running
	code, body := e.do(t, http.MethodPost, "/api/executors/act/start", "")
	if code != http.StatusOK {
		t.Fatalf("start 应 200，实际 %d %s", code, body)
	}
	if st := jsonMap(t, body)["executor"].(map[string]any)["local"].(map[string]any)["state"]; st != "running" {
		t.Fatalf("start 后应 running，实际 %v", st)
	}
	// start 幂等（已运行 no-op 成功）
	code, _ = e.do(t, http.MethodPost, "/api/executors/act/start", "")
	if code != http.StatusOK {
		t.Fatalf("重复 start 应幂等 200，实际 %d", code)
	}
	// restart → 仍 running
	code, body = e.do(t, http.MethodPost, "/api/executors/act/restart", "")
	if code != http.StatusOK {
		t.Fatalf("restart 应 200，实际 %d %s", code, body)
	}
	if st := jsonMap(t, body)["executor"].(map[string]any)["local"].(map[string]any)["state"]; st != "running" {
		t.Fatalf("restart 后应 running，实际 %v", st)
	}
	// stop → stopped
	code, body = e.do(t, http.MethodPost, "/api/executors/act/stop", "")
	if code != http.StatusOK {
		t.Fatalf("stop 应 200，实际 %d %s", code, body)
	}
	if st := jsonMap(t, body)["executor"].(map[string]any)["local"].(map[string]any)["state"]; st != "stopped" {
		t.Fatalf("stop 后应 stopped，实际 %v", st)
	}
	// 不存在 → 404
	code, _ = e.do(t, http.MethodPost, "/api/executors/nope/start", "")
	if code != http.StatusNotFound {
		t.Fatalf("start 不存在应 404，实际 %d", code)
	}
}

// --- F3 验收：DELETE ---

func TestAPIremove运行中409与stop参数(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "del", Command: validCommand})
	if _, body := e.do(t, http.MethodPost, "/api/executors/del/start", ""); body == "" {
		t.Fatal("start 失败")
	}
	// 运行中直接删 → 409 code=running
	code, resp := e.do(t, http.MethodDelete, "/api/executors/del", "")
	if code != http.StatusConflict {
		t.Fatalf("运行中删除应 409，实际 %d %s", code, resp)
	}
	if jsonMap(t, resp)["error"].(map[string]any)["code"] != "running" {
		t.Fatal("code 应为 running")
	}
	// stop=true → 停止并删除
	code, resp = e.do(t, http.MethodDelete, "/api/executors/del?stop=true", "")
	if code != http.StatusOK {
		t.Fatalf("stop=true 删除应 200，实际 %d %s", code, resp)
	}
	m := jsonMap(t, resp)
	if m["removed"] != "del" || m["stopped"] != true {
		t.Fatalf("删除响应不合契约：%v", m)
	}
	onDisk, _ := config.Load(e.p.ConfigFile)
	if onDisk.FindExecutor("del") != nil {
		t.Fatal("删除后配置不应再含 del")
	}
}

func TestAPIremovePurge删日志(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "purge-me", Command: validCommand})
	// 造一份日志文件
	sink, err := e.mgr.Sink("purge-me")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = sink.Write([]byte("line1\n"))
	logPath := filepath.Join(e.p.ExecutorsLogDir, "purge-me.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatal(err)
	}
	code, resp := e.do(t, http.MethodDelete, "/api/executors/purge-me?purge=true", "")
	if code != http.StatusOK {
		t.Fatalf("purge 删除应 200，实际 %d %s", code, resp)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("purge 应删除日志文件，err=%v", err)
	}
}

// --- F3 验收：log 拉取 ---

func TestAPIlog尾部拉取(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "lg", Command: validCommand})
	sink, _ := e.mgr.Sink("lg")
	for i := 1; i <= 10; i++ {
		_, _ = sink.Write([]byte(fmt.Sprintf("第%d行\n", i)))
	}
	code, body := e.do(t, http.MethodGet, "/api/executors/lg/log?tail=3", "")
	if code != http.StatusOK {
		t.Fatalf("log 应 200，实际 %d %s", code, body)
	}
	m := jsonMap(t, body)
	lines := m["lines"].([]any)
	if len(lines) != 3 || lines[0] != "第8行" {
		t.Fatalf("tail=3 应返回最后 3 行，实际 %v", lines)
	}
	// tail 非法
	code, body = e.do(t, http.MethodGet, "/api/executors/lg/log?tail=abc", "")
	if code != http.StatusBadRequest {
		t.Fatalf("tail 非法应 400，实际 %d", code)
	}
}

// --- F3 验收：status / settings / reload ---

func TestAPIstatus(t *testing.T) {
	e := newEnvWithExecs(t)
	code, body := e.do(t, http.MethodGet, "/api/status", "")
	if code != http.StatusOK {
		t.Fatalf("status 应 200，实际 %d", code)
	}
	m := jsonMap(t, body)
	if m["version"] != "test-1.2.3" || m["port"] != float64(18765) {
		t.Fatalf("status 字段不合预期：%v", m)
	}
	if m["platform"].(map[string]any)["configured"] != false {
		t.Fatalf("未配置平台时 configured 应 false：%v", m["platform"])
	}
}

func TestAPIsettings读写与掩码保留(t *testing.T) {
	e := newEnvWithExecs(t)
	// PUT 配置平台段
	code, body := e.do(t, http.MethodPut, "/api/settings",
		`{"platform":{"base_url":"http://10.0.0.9:7001","mcp_token":"mcp-secret-token-9876","poll_interval":"30s"},"log":{"max_size_mb":20,"retention_days":3}}`)
	if code != http.StatusOK {
		t.Fatalf("settings PUT 应 200，实际 %d %s", code, body)
	}
	m := jsonMap(t, body)
	pl := m["platform"].(map[string]any)
	if tok, _ := pl["mcp_token"].(string); !strings.Contains(tok, "***") || strings.Contains(tok, "mcp-secret-token-9876") {
		t.Fatalf("mcp_token 应掩码回显：%q", tok)
	}
	// GET 回读 + 掩码
	code, body = e.do(t, http.MethodGet, "/api/settings", "")
	if code != http.StatusOK {
		t.Fatal("settings GET 失败")
	}
	m = jsonMap(t, body)
	if m["log"].(map[string]any)["max_size_mb"] != float64(20) {
		t.Fatalf("log 设置未生效：%v", m["log"])
	}
	// 落盘明文（0600 明文落盘约定）
	onDisk, _ := config.Load(e.p.ConfigFile)
	if onDisk.Platform == nil || onDisk.Platform.MCPToken != "mcp-secret-token-9876" {
		t.Fatalf("落盘应保留明文令牌：%+v", onDisk.Platform)
	}
	// 掩码原样提交 → 保留原值；base_url 更新
	masked := jsonMap(t, body)["platform"].(map[string]any)["mcp_token"].(string)
	code, _ = e.do(t, http.MethodPut, "/api/settings",
		`{"platform":{"base_url":"http://10.0.0.8:7001","mcp_token":"`+masked+`"}}`)
	if code != http.StatusOK {
		t.Fatalf("掩码提交应成功：%d", code)
	}
	onDisk, _ = config.Load(e.p.ConfigFile)
	if onDisk.Platform.MCPToken != "mcp-secret-token-9876" || onDisk.Platform.BaseURL != "http://10.0.0.8:7001" {
		t.Fatalf("掩码提交不应覆盖令牌：%+v", onDisk.Platform)
	}
	// platform null → 删除段
	code, _ = e.do(t, http.MethodPut, "/api/settings", `{"platform":null}`)
	if code != http.StatusOK {
		t.Fatalf("删除平台段应成功：%d", code)
	}
	onDisk, _ = config.Load(e.p.ConfigFile)
	if onDisk.Platform != nil {
		t.Fatalf("platform=null 应删除平台段：%+v", onDisk.Platform)
	}
	// 非法时长
	code, _ = e.do(t, http.MethodPut, "/api/settings", `{"platform":{"base_url":"x","mcp_token":"t","poll_interval":"负数"}}`)
	if code != http.StatusBadRequest {
		t.Fatalf("非法 poll_interval 应 400，实际 %d", code)
	}
}

func TestAPIreload生效(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "r1", Command: validCommand})
	// 手改磁盘配置（绕过 API）
	onDisk, _ := config.Load(e.p.ConfigFile)
	onDisk.Executors = append(onDisk.Executors, config.Executor{Name: "r2", Command: validCommand})
	if err := config.Save(e.p.ConfigFile, onDisk); err != nil {
		t.Fatal(err)
	}
	code, body := e.do(t, http.MethodPost, "/api/reload", "")
	if code != http.StatusOK {
		t.Fatalf("reload 应 200，实际 %d %s", code, body)
	}
	// 后续命令看到新配置
	code, body = e.do(t, http.MethodGet, "/api/executors/r2", "")
	if code != http.StatusOK {
		t.Fatalf("reload 后新增执行器应可见，实际 %d %s", code, body)
	}
}

// --- 并发写保护冒烟 ---

func TestAPI并发写不丢配置(t *testing.T) {
	e := newEnvWithExecs(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("c%d", i)
			body := `{"name":"` + name + `","command":"` + validCommand + `"}`
			code, resp := e.do(t, http.MethodPost, "/api/executors", body)
			if code != http.StatusCreated {
				t.Errorf("并发 add %s 失败：%d %s", name, code, resp)
			}
		}(i)
	}
	wg.Wait()
	onDisk, err := config.Load(e.p.ConfigFile)
	if err != nil {
		t.Fatalf("并发写后配置损坏：%v", err)
	}
	if len(onDisk.Executors) != 8 {
		t.Fatalf("并发 add 应全部落盘（8 个），实际 %d", len(onDisk.Executors))
	}
}
