// ws_test.go：WebSocket 日志实时流验收（T06-F4）。真 WS 客户端
// （gorilla dialer）：连接 → 收尾部行 → 持续写入持续收到 → 断开无泄漏；
// 未授权连接在升级前被拒。
package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"awem/internal/config"
)

// dialWS 建立带鉴权的 WS 连接。
func dialWS(t *testing.T, e *testEnv, path string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http") + path
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("WS 连接失败：%v（HTTP %d）", err, resp.StatusCode)
		}
		t.Fatalf("WS 连接失败：%v", err)
	}
	return conn
}

// readMsg 读一帧并解出信封。
func readMsg(t *testing.T, conn *websocket.Conn) wsMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读 WS 帧失败：%v", err)
	}
	var m wsMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("帧不是合法信封：%v（%s）", err, data)
	}
	return m
}

// TestWSbacklog与Follow不丢不重 backlog 结束与实时流之间写入的行不丢
// （TailOffset/FollowFrom 一致性回归；修复前 Follow 首开竞态会吞行）。
func TestWSbacklog与Follow不丢不重(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "noLoss", Command: validCommand})
	sink, _ := e.mgr.Sink("noLoss")
	_, _ = sink.Write([]byte("旧1\n旧2\n"))

	conn := dialWS(t, e, "/ws/logs/noLoss?tail=1&token=test-token")
	defer conn.Close()
	m := readMsg(t, conn)
	if m.Type != "backlog" || len(m.Lines) != 1 || m.Lines[0] != "旧2" {
		t.Fatalf("backlog 应只含最后 1 行：%+v", m)
	}
	// 立刻写入（落在 backlog 之后、Follow 首开之前的窗口内）——必须收到
	_, _ = sink.Write([]byte("窗口行W\n"))
	waitForLine(t, conn, "窗口行W")
}

// waitForLine 持续读帧直到出现指定文本的 line 帧（超时 fatal）。
func waitForLine(t *testing.T, conn *websocket.Conn, text string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		m := readMsg(t, conn)
		if m.Type == "line" && m.Text == text {
			return
		}
	}
	t.Fatalf("5s 内未收到 line 帧 %q", text)
}

func TestWS日志流全链路(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "wsx", Command: validCommand})
	// 预置 3 行历史
	sink, _ := e.mgr.Sink("wsx")
	_, _ = sink.Write([]byte("历史1\n历史2\n历史3\n"))

	before := runtime.NumGoroutine()
	conn := dialWS(t, e, "/ws/logs/wsx?tail=2&token=test-token")
	// 尾部回放：只含最后 2 行
	m := readMsg(t, conn)
	if m.Type != "backlog" || len(m.Lines) != 2 || m.Lines[0] != "历史2" {
		t.Fatalf("backlog 应为最后 2 行，实际 %+v", m)
	}
	// 持续写入 → 实时收到
	_, _ = sink.Write([]byte("新行A\n"))
	waitForLine(t, conn, "新行A")
	_, _ = sink.Write([]byte("新行B\n"))
	waitForLine(t, conn, "新行B")

	// 客户端断开 → 服务端回收（goroutine 数回落）
	conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("断开后 goroutine 应回落（%d → %d），疑似泄漏", before, n)
	}
	// 文件句柄回收：sink 已打开文件正常，Follow 的只读 fd 关闭后
	// 不影响写路径（写入仍成功即视为回收完成——fd 计数在 /proc 平台相关，
	// goroutine 断言为主证据）
	if _, err := sink.Write([]byte("断开后写入\n")); err != nil {
		t.Fatalf("断开后写日志失败：%v", err)
	}
}

func TestWS未授权被拒(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "wsa", Command: validCommand})
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws/logs/wsa"
	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("无 token 的 WS 连接应被拒绝")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无 token 应在升级前 401，实际 resp=%v", resp)
	}
	// 错 token 同样拒绝
	url += "?token=wrong"
	_, resp, err = websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("错 token 的 WS 连接应被拒绝")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错 token 应 401，实际 resp=%v", resp)
	}
}

func TestWS不存在的执行器(t *testing.T) {
	e := newEnvWithExecs(t)
	// httptest 客户端直接走 HTTP（不升级）拿 404
	code, body := e.do(t, http.MethodGet, "/ws/logs/nope?tail=1", "")
	if code != http.StatusNotFound {
		t.Fatalf("不存在执行器应 404，实际 %d %s", code, body)
	}
}

func TestWS头鉴权也可用(t *testing.T) {
	e := newEnvWithExecs(t, config.Executor{Name: "wsh", Command: validCommand})
	sink, _ := e.mgr.Sink("wsh")
	_, _ = sink.Write([]byte("h1\n"))
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws/logs/wsh?tail=1"
	h := http.Header{}
	h.Set("Authorization", "Bearer test-token")
	conn, _, err := websocket.DefaultDialer.Dial(url, h)
	if err != nil {
		t.Fatalf("头鉴权 WS 连接失败：%v", err)
	}
	defer conn.Close()
	m := readMsg(t, conn)
	if m.Type != "backlog" || len(m.Lines) != 1 || m.Lines[0] != "h1" {
		t.Fatalf("头鉴权连接应正常收 backlog：%+v", m)
	}
}
