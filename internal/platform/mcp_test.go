// mcp_test.go：MCP 只读客户端验收（T08-F1）。httptest 假 MCP 服务，
// 不依赖真实平台。
package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeMCP 是最小假 MCP 服务（Streamable HTTP JSON-RPC）。
type fakeMCP struct {
	mu      sync.Mutex
	server  *httptest.Server
	tools   map[string]func(args map[string]any) (string, error) // 工具名 → text 响应
	calls   []string                                             // 收到的工具调用名
	seenSID map[string]bool
	initOK  bool
	slow    bool // 模拟超时
}

func newFakeMCP(t *testing.T) *fakeMCP {
	t.Helper()
	f := &fakeMCP{
		tools: map[string]func(map[string]any) (string, error){},
	}
	mux := http.NewServeMux()
	// 两条路径同处理器："/mcp"（Client 直连测试用 f.url()）与真实平台
	// 风格的令牌路径（NewService 端点推导用 BaseURL 直连）
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.slow {
			time.Sleep(3 * time.Second)
		}
		var req struct {
			ID     int64          `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.calls = append(f.calls, req.Method)
		f.mu.Unlock()
		isInit := req.Method == "initialize"
		respond := func(result any) {
			w.Header().Set("Content-Type", "application/json")
			if isInit {
				w.Header().Set("Mcp-Session-Id", "sess-42")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID, "result": result,
			})
		}
		switch req.Method {
		case "initialize":
			respond(map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}})
		case "tools/call":
			name, _ := req.Params["name"].(string)
			args, _ := req.Params["arguments"].(map[string]any)
			f.mu.Lock()
			f.calls = append(f.calls, "tool:"+name)
			fn := f.tools[name]
			f.mu.Unlock()
			if fn == nil {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": req.ID,
					"error": map[string]any{"code": -32602, "message": "unknown tool " + name},
				})
				return
			}
			text, err := fn(args)
			if err != nil {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": req.ID,
					"error": map[string]any{"code": -32001, "message": err.Error()},
				})
				return
			}
			respond(map[string]any{
				"content": []map[string]any{{"type": "text", "text": text}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}
	mux.HandleFunc("/mcp", handler)
	mux.HandleFunc("/api/mcp/tok-1/", handler)
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeMCP) url() string { return f.server.URL + "/mcp" }

func (f *fakeMCP) callCount(tool string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == "tool:"+tool {
			n++
		}
	}
	return n
}

func TestMCP正常链路与会话头(t *testing.T) {
	f := newFakeMCP(t)
	f.tools["autowonder.list_projects"] = func(map[string]any) (string, error) {
		return `{"items":[{"id":10000,"name":"git-workspace"},{"id":10001,"name":"home-test"}]}`, nil
	}
	f.tools["autowonder.list_executors"] = func(args map[string]any) (string, error) {
		if args["workspaceId"].(float64) != 10001 {
			return `{"items":[]}`, nil
		}
		return `{"items":[{"id":10086,"name":"NO1快速全栈开发","agentId":10007,"agentName":"快速全栈开发","status":"ONLINE","lastHeartbeat":1792872206969}]}`, nil
	}
	c := NewClient(f.url(), "tok-1")

	ps, err := c.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list_projects 失败：%v", err)
	}
	if len(ps) != 2 || ps[0].ID != 10000 || ps[0].Name != "git-workspace" {
		t.Fatalf("空间解析：%+v", ps)
	}
	execs, err := c.ListExecutors(context.Background(), 10001)
	if err != nil {
		t.Fatalf("list_executors 失败：%v", err)
	}
	if len(execs) != 1 || execs[0].ID != 10086 || execs[0].Status != "ONLINE" {
		t.Fatalf("执行器解析：%+v", execs)
	}
	if execs[0].Name != "NO1快速全栈开发" || execs[0].AgentName != "快速全栈开发" || execs[0].AgentID != 10007 {
		t.Fatalf("归属展示字段解析：%+v", execs[0])
	}
	if execs[0].Heartbeat().IsZero() {
		t.Fatal("心跳时间未解析")
	}
	// 工具名点号前缀（附录 A）断言：假服务只认识点号前缀工具
	if f.callCount("autowonder.list_projects") != 1 || f.callCount("autowonder.list_executors") != 1 {
		t.Fatalf("工具调用记录：%v", f.calls)
	}
	// initialize 握手发生过且会话头在后续请求携带（假服务 seenSID 未强制——
	// 此处断言 initialize 次数为 1，即惰性一次）
	initCount := 0
	f.mu.Lock()
	for _, c := range f.calls {
		if c == "initialize" {
			initCount++
		}
	}
	f.mu.Unlock()
	if initCount != 1 {
		t.Fatalf("initialize 应恰好一次，实际 %d", initCount)
	}
}

func TestMCP错误响应与权限分类(t *testing.T) {
	f := newFakeMCP(t)
	f.tools["autowonder.list_projects"] = func(map[string]any) (string, error) {
		return "", staticErr("no permission for workspace")
	}
	c := NewClient(f.url(), "tok-1")
	// JSON-RPC 错误含 permission → IsPermissionErr（调用方跳过语义）
	if _, err := c.ListProjects(context.Background()); !IsPermissionErr(err) {
		t.Fatalf("应归类权限不足：%v", err)
	}
	// HTTP 401（错令牌）
	c2 := NewClient(f.url(), "wrong-token")
	_, err := c2.ListProjects(context.Background())
	if !IsPermissionErr(err) {
		t.Fatalf("HTTP 401 应归类权限不足：%v", err)
	}
	// 工具不存在 → 一般错误
	f.tools["autowonder.list_projects"] = nil
	delete(f.tools, "autowonder.list_projects")
	if _, err := c.ListProjects(context.Background()); err == nil || IsPermissionErr(err) {
		t.Fatalf("未知工具应为一般错误：%v", err)
	}
}

type staticErr string

func (e staticErr) Error() string { return string(e) }

func TestMCP超时(t *testing.T) {
	f := newFakeMCP(t)
	f.slow = true
	f.tools["autowonder.list_projects"] = func(map[string]any) (string, error) { return `{"items":[]}`, nil }
	c := NewClient(f.url(), "tok-1")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := c.ListProjects(ctx); err == nil {
		t.Fatal("超时应返回错误")
	}
}

func TestMCP_SSE响应形态(t *testing.T) {
	// Streamable HTTP 可能回 SSE；客户端应提取 data: 行 JSON
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// 原始字符串里的 \" 即 JSON 转义，inner 是 text 字段的内嵌 JSON
		inner := `{\"items\":[]}`
		data := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"` + inner + `"}]}}`
		_, _ = w.Write([]byte("event: message\ndata: " + data + "\n\n"))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok-1")
	if _, err := c.ListProjects(context.Background()); err != nil {
		t.Fatalf("SSE 形态应可解析：%v", err)
	}
}
