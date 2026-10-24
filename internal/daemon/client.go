// client.go：CLI 侧统一连接器（T06-F6）。流程：
//	读 daemon.json → 探活（任意 HTTP 响应即活）→ 未运行则后台拉起
//	`awem daemon`（detach，stderr 落 data/logs/daemon.stderr 便于排障）→
//	轮询等就绪（默认 10s）→ 返回带 token 的 API 客户端。
// 拉起失败或超时 → 退出码 5 语义错误（exitcode.DaemonNotRunning）+ 中文指引。
// 并发竞态：多个 CLI 同时拉起时，锁保证只有一个 daemon 存活，其余实例
// 立即退出，各方轮询端口最终都能拿到可用服务。
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"awem/internal/config"
	"awem/internal/exitcode"
)

// DefaultReadyTimeout 是拉起后等待端口就绪的超时。
const DefaultReadyTimeout = 10 * time.Second

// ClientOptions 是 Connect 的可注入项（测试替换拉起与探活）。
type ClientOptions struct {
	Paths        config.Paths
	ReadyTimeout time.Duration   // 就绪等待（0=默认 10s）
	Launch       func() error    // 拉起 daemon（nil=默认 spawn 自身）
}

// Client 是本地 API 的 CLI 侧客户端（自动带 token 头）。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Connect 返回可用客户端；daemon 未运行时自动拉起。
// 探活失败的瞬时错误（daemon 正在退出）按未运行处理重拉。
func Connect(ctx context.Context, opts ClientOptions) (*Client, error) {
	if opts.ReadyTimeout <= 0 {
		opts.ReadyTimeout = DefaultReadyTimeout
	}
	info, alive, err := config.ReadDaemonInfo(opts.Paths)
	if err != nil {
		return nil, exitcode.Wrap(exitcode.General, err)
	}
	if alive && probeAlive(info.Port) {
		return newClient(info.Port, opts.Paths), nil
	}
	// 未运行（或 daemon.json 缺失/stale）：拉起
	launch := opts.Launch
	if launch == nil {
		launch = func() error { return spawnDetachedDaemon(opts.Paths) }
	}
	if err := launch(); err != nil {
		return nil, exitcode.DaemonNotRunningf(
			"daemon 未运行且自动拉起失败（%v）；可先执行 `awem daemon` 前台运行查看错误，或 `awem service install` 注册系统服务", err)
	}
	// 轮询等就绪（端口就绪即 daemon.json 可用）
	deadline := time.Now().Add(opts.ReadyTimeout)
	for {
		info, alive, err := config.ReadDaemonInfo(opts.Paths)
		if err == nil && alive && probeAlive(info.Port) {
			return newClient(info.Port, opts.Paths), nil
		}
		select {
		case <-ctx.Done():
			return nil, exitcode.DaemonNotRunningf(
				"daemon 自动拉起后 %s 内未就绪；请执行 `awem daemon` 前台运行排查（stderr 见 data/logs/daemon.stderr）", opts.ReadyTimeout)
		case <-time.After(100 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return nil, exitcode.DaemonNotRunningf(
				"daemon 自动拉起后 %s 内未就绪；请执行 `awem daemon` 前台运行排查（stderr 见 data/logs/daemon.stderr）", opts.ReadyTimeout)
		}
	}
}

// newClient 组装客户端（token 取自 data/api-token；文件缺失时重新生成，
// 与 daemon 侧约定一致）。
func newClient(port int, p config.Paths) *Client {
	token, err := config.APIToken(p)
	if err != nil {
		token = ""
	}
	return &Client{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError 是服务端按错误 schema 返回的结构化错误。
type APIError struct {
	HTTPStatus int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Field      string `json:"field"`
}

func (e *APIError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s（field=%s）：%s", e.Code, e.Field, e.Message)
	}
	return fmt.Sprintf("%s：%s", e.Code, e.Message)
}

// ExitCode 把 API 错误映射为 CLI 退出码（conventions §4）。
// 用户输入的命令/参数问题（invalid_command/invalid_name/bad_request/
// invalid_config）按用法错误（2）；not_found→3；already_exists→4；
// 其余→1（unauthorized/forbidden 在同机同用户场景不应出现）。
func (e *APIError) ExitCode() int {
	switch e.Code {
	case "invalid_command", "invalid_name", "bad_request", "invalid_config":
		return exitcode.Usage
	case "not_found":
		return exitcode.NotFound
	case "already_exists":
		return exitcode.AlreadyExists
	default:
		return exitcode.General
	}
}

// NewClientAt 返回直连指定 base URL 的客户端（测试直连 httptest 假
// daemon 用；生产路径走 Connect 惰性拉起）。
func NewClientAt(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Do 发起带鉴权请求；2xx 返回 body；4xx/5xx 返回 *APIError。
func (c *Client) Do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return nil, exitcode.Wrap(exitcode.General, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, exitcode.Wrap(exitcode.General, fmt.Errorf("请求本地 API 失败：%w", err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, exitcode.Wrap(exitcode.General, err)
	}
	if resp.StatusCode >= 400 {
		var eb struct {
			Error APIError `json:"error"`
		}
		if json.Unmarshal(data, &eb) == nil && eb.Error.Code != "" {
			eb.Error.HTTPStatus = resp.StatusCode
			return nil, &eb.Error
		}
		return nil, exitcode.Generalf("本地 API 返回 %d：%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// GetJSON 便捷 GET。
func (c *Client) GetJSON(ctx context.Context, path string) ([]byte, error) {
	return c.Do(ctx, http.MethodGet, path, nil)
}

// probeAlive 探活：目标端口能返回任意 HTTP 响应（401 也算活——探活不带 token）。
func probeAlive(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/status", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// spawnDetachedDaemon 后台拉起 daemon：spawn 自身二进制 `awem daemon`，
// 与终端脱钩（Unix setsid / Windows 分离进程组），stderr 落
// data/logs/daemon.stderr（排障），stdout 丢弃。
func spawnDetachedDaemon(p config.Paths) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位 awem 二进制失败：%w", err)
	}
	if err := os.MkdirAll(p.LogsDir, 0o700); err != nil {
		return fmt.Errorf("创建日志目录失败：%w", err)
	}
	stderrPath := p.LogsDir + string(os.PathSeparator) + "daemon.stderr"
	stderrFile, err := os.OpenFile(stderrPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("打开 daemon.stderr 失败：%w", err)
	}
	defer stderrFile.Close()
	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("打开空设备失败：%w", err)
	}
	defer devNull.Close()

	cmd := exec.Command(self, "daemon")
	cmd.Stdout = devNull
	cmd.Stderr = stderrFile
	detachDaemon(cmd)
	return cmd.Start()
}
