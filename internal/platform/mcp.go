// mcp.go：平台 MCP 只读客户端（T08-F1）。协议：Streamable HTTP JSON-RPC
// （POST <mcp-url>，附录 A）：initialize 握手（Mcp-Session-Id 会话头）→
// tools/call 调用点号前缀工具。
//
// 红线（design §2/§7.5）：对平台只读。本文件与 F5 共同保证：
//  1. 公开 API 只有 ListProjects / ListExecutors / GetExecutorLaunchConfig
//     三个类型化只读方法；
//  2. 底层 call 私有，且带工具名白名单校验——白名单外的工具名（一切
//     平台写操作）在客户端层直接拒绝，不发出任何 HTTP 请求。
//
// MCP 端点推导（2026-10-25 真实平台实测校准）：config.base_url +
// "/api/mcp/" + mcp_token + "/"——个人令牌编在 URL 路径中（推导在
// syncer.go NewService；Bearer 头平台兼容保留）。
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// readOnlyTools 是允许调用的全部平台工具（F5 白名单；只读三件套）。
// 红线来源 design §2"不做平台写操作"——任何写工具加入此表即违反红线。
var readOnlyTools = map[string]bool{
	"autowonder.list_projects":              true,
	"autowonder.list_executors":             true,
	"autowonder.get_executor_launch_config": true,
}

// Client 是 MCP 只读客户端。并发安全（会话头初始化有锁）。
type Client struct {
	url       string // MCP 端点完整 URL
	token     string
	http      *http.Client
	sessMu    chan struct{} // 会话初始化互斥（容量 1）
	sessionID string
}

// NewClient 创建客户端（url 为完整 MCP 端点；token 个人 MCP 令牌）。
func NewClient(url, token string) *Client {
	return &Client{
		url:    url,
		token:  token,
		http:   &http.Client{Timeout: 15 * time.Second},
		sessMu: make(chan struct{}, 1),
	}
}

// Project 是 list_projects 的一条空间信息（真实平台字段：id/name，
// 外层 {"items":[…]} 包裹——2026-10-25 实测校准）。
type Project struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ExecutorRemote 是 list_executors 的一条平台侧执行器（字段名按真实
// 平台 2026-10-25 实测校准：name=执行器名（通常为接入时的中文名）、
// agentId/agentName=所属数字工人、lastHeartbeat=Unix 毫秒）。
type ExecutorRemote struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	AgentID        int64    `json:"agentId"`
	AgentName      string   `json:"agentName"`
	Status         string   `json:"status"` // ONLINE / BUSY / OFFLINE
	LastHeartbeatMS int64   `json:"lastHeartbeat"` // Unix 毫秒
	SquadNames     []string `json:"squadNames"`
}

// Heartbeat 把毫秒时间戳转 time.Time（0/缺失返回零值）。
func (e ExecutorRemote) Heartbeat() time.Time {
	if e.LastHeartbeatMS <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(e.LastHeartbeatMS)
}

// Squad 把小队名列表拼为展示串（顿号分隔；空为 ""）。
func (e ExecutorRemote) Squad() string {
	return strings.Join(e.SquadNames, "、")
}

// LaunchConfig 是 get_executor_launch_config 的平台侧启动配置
// （2026-10-25 实测：contextWindow 为字符串，如 "1000000"）。
type LaunchConfig struct {
	Model                   string `json:"model"`
	ReasoningEffort         string `json:"reasoningEffort"`
	ContextWindow           string `json:"contextWindow"`
	MemoryMode              string `json:"memoryMode"`
	MaxConcurrentDispatches int64  `json:"maxConcurrentDispatches"`
}

// ListProjects 列出可见空间（READ 及以上权限，由平台侧按令牌过滤）。
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	raw, err := c.call(ctx, "autowonder.list_projects", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []Project `json:"items"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析 list_projects 响应失败：%w", err)
	}
	return out.Items, nil
}

// ListExecutors 列出某空间的执行器（含实时状态与所属数字工人）。
func (c *Client) ListExecutors(ctx context.Context, workspaceID int64) ([]ExecutorRemote, error) {
	raw, err := c.call(ctx, "autowonder.list_executors", map[string]any{
		"workspaceId": workspaceID,
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []ExecutorRemote `json:"items"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析 list_executors 响应失败：%w", err)
	}
	return out.Items, nil
}

// GetExecutorLaunchConfig 查平台侧启动配置（漂移检测用）。
func (c *Client) GetExecutorLaunchConfig(ctx context.Context, workspaceID, executorID int64) (*LaunchConfig, error) {
	raw, err := c.call(ctx, "autowonder.get_executor_launch_config", map[string]any{
		"workspaceId": workspaceID,
		"executorId":  executorID,
	})
	if err != nil {
		return nil, err
	}
	var out LaunchConfig
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析 get_executor_launch_config 响应失败：%w", err)
	}
	return &out, nil
}

// errPermission 权限不足（令牌看不到目标资源）——调用方按 design §7.4 跳过。
type errPermission struct{ detail string }

func (e *errPermission) Error() string { return "权限不足：" + e.detail }

// IsPermissionErr 报告错误是否为权限不足（跳过语义）。
func IsPermissionErr(err error) bool {
	if ep, ok := err.(*errPermission); ok {
		return ep != nil
	}
	return false
}

// rpcRequest / rpcResponse 是 JSON-RPC 2.0 信封。
type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int64          `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// call 执行一次 tools/call（私有：白名单外工具名直接拒绝，无 HTTP 发出）。
func (c *Client) call(ctx context.Context, tool string, args map[string]any) (json.RawMessage, error) {
	if !readOnlyTools[tool] {
		return nil, fmt.Errorf("工具 %s 不在只读白名单（红线：不做平台写操作），已拒绝", tool)
	}
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	return c.post(ctx, "tools/call", map[string]any{
		"name":      tool,
		"arguments": args,
	})
}

// ensureSession 惰性 initialize 握手（失败下次重试）。
func (c *Client) ensureSession(ctx context.Context) error {
	select {
	case c.sessMu <- struct{}{}:
		defer func() { <-c.sessMu }()
	default:
		return nil // 已有并发初始化在进行；直接尝试带当前会话头发请求
	}
	if c.sessionID != "" {
		return nil
	}
	_, err := c.post(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "awem", "version": "1"},
	})
	return err
}

// post 发一次 JSON-RPC 请求并解包 result.content[0].text。
func (c *Client) post(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	body, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MCP 请求失败（%s %s）：%w", method, c.url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 MCP 响应失败：%w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, &errPermission{fmt.Sprintf("HTTP %d", resp.StatusCode)}
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("MCP HTTP %d：%s", resp.StatusCode, truncate(string(data), 200))
	}
	// 会话头记录（initialize 响应携带）
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.sessionID = sid
	}
	// Streamable HTTP 可能返回 SSE（text/event-stream）——取 data: 行的 JSON
	payload := extractJSON(data)

	var r rpcResponse
	if err := json.Unmarshal(payload, &r); err != nil {
		return nil, fmt.Errorf("MCP 响应非 JSON-RPC：%w", err)
	}
	if r.Error != nil {
		// MCP 常见权限类错误码按权限不足处理（调用方跳过）
		if r.Error.Code == -32001 || strings.Contains(r.Error.Message, "permission") ||
			strings.Contains(r.Error.Message, "权限") || strings.Contains(r.Error.Message, "forbidden") {
			return nil, &errPermission{r.Error.Message}
		}
		return nil, fmt.Errorf("MCP 错误（%s code=%d）：%s", method, r.Error.Code, r.Error.Message)
	}
	// tools/call 结果：result.content[0].text（文本里是资源 JSON）
	if method == "tools/call" {
		var tr struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(r.Result, &tr); err != nil || len(tr.Content) == 0 {
			return nil, fmt.Errorf("MCP 工具响应缺少 content：%s", truncate(string(r.Result), 200))
		}
		return json.RawMessage(tr.Content[0].Text), nil
	}
	return r.Result, nil
}

// extractJSON 从可能的 SSE 响应体提取 JSON（非 SSE 原样返回）。
func extractJSON(data []byte) []byte {
	s := string(data)
	if !strings.Contains(s, "data:") {
		return data
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			return []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return data
}

// truncate 截断长文本。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
