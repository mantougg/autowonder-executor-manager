// ws.go：WebSocket 日志实时流（T06-F4）。
//	GET /ws/logs/{name}?tail=N&token=<api-token>
// 升级成功后：先推送尾部 N 行（backlog），再持续推送新行（复用 T04-F6
// Follow 订阅）；客户端断开即取消订阅并回收（goroutine/文件句柄）。
//
// 消息格式（服务端→客户端，JSON 文本帧；契约记入变更记录）：
//	{"type":"backlog","lines":[...]}   连接后一次性尾部回放
//	{"type":"line","text":"..."}       此后逐行实时推送
//	{"type":"error","message":"..."}   出错后随连接关闭
//
// 状态推送通道 v1 不做（前端轮询 /api/executors；WS 状态通道留接口不实现，
// 记入变更记录）。鉴权同 F5：token 头之外，WS 因浏览器无法自定义请求头，
// 额外接受 query 参数 ?token=（仅 /ws/* 生效，避免 token 进 REST 访问日志）。
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

// wsUpgrader 升级器：仅本机回环来源（Origin 校验在 F5 中间件统一做，
// 此处再限制 ReadBufferSize）；不压缩。
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Origin 检查交给鉴权中间件（F5）；不使用库默认的同源拒绝，
	// 以便 CLI/测试带自定义 Origin 头诊断
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsHandler 返回 WS 子路由（/ws/logs/{name}，F4）。
func (s *Server) wsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/logs/{name}", s.handleWSLogs)
	return mux
}

// wsMessage 是 WS 文本帧的统一信封。
type wsMessage struct {
	Type    string   `json:"type"`           // backlog / line / error
	Text    string   `json:"text,omitempty"` // type=line 的行内容
	Lines   []string `json:"lines,omitempty"` // type=backlog 的尾部回放
	Message string   `json:"message,omitempty"`
}

// handleWSLogs 升级连接并进入读循环（客户端消息一概忽略，仅用于断开感知）。
func (s *Server) handleWSLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg := s.currentConfig()
	if cfg.FindExecutor(name) == nil {
		writeAPIError(w, notFound("执行器 "+name+" 不存在"))
		return
	}
	tail := 100
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err != nil || n < 0 {
			writeAPIError(w, badRequest("tail 必须为非负整数"))
			return
		} else {
			tail = n
		}
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade 已写错误响应
	}
	defer conn.Close()

	// 尾部回放：TailOffset 拿行与读取结束偏移（tail=0 时仍需偏移，保证接续）
	lines, offset, err := s.opts.Logs.TailOffset(name, tail)
	if err != nil {
		s.wsWrite(conn, wsMessage{Type: "error", Message: err.Error()})
		return
	}
	if tail > 0 {
		if lines == nil {
			lines = []string{}
		}
		if !s.wsWrite(conn, wsMessage{Type: "backlog", Lines: lines}) {
			return
		}
	}

	// 实时订阅：从 backlog 读取结束的偏移续订（不丢不重：backlog 之后
	// 写入的字节恰好落在 [offset, ∞)；Follow 首次打开的竞态窗口由此消除）
	sub, err := s.opts.Logs.FollowFrom(name, offset)
	if err != nil {
		s.wsWrite(conn, wsMessage{Type: "error", Message: err.Error()})
		return
	}
	defer sub.Close()

	// 写泵：订阅行 → WS 帧；done 后收口
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for line := range sub.C {
			if !s.wsWrite(conn, wsMessage{Type: "line", Text: line}) {
				return
			}
		}
	}()

	// 读泵：客户端消息忽略，仅感知断开（ReadMessage 返回错误即断开）
	go func() {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				// 断开：关闭订阅让写泵退出（sub.Close 幂等，defer 再兜底）
				sub.Close()
				return
			}
		}
	}()

	select {
	case <-writeDone: // 写泵结束（订阅关闭或写失败）
	case <-r.Context().Done(): // HTTP 连接断开
	}
	conn.Close()
	<-writeDone // 等写泵彻底退出（无 goroutine 泄漏）
}

// wsWrite 写一帧 JSON 文本；失败（客户端已断开/超时）返回 false。
func (s *Server) wsWrite(conn *websocket.Conn, msg wsMessage) bool {
	data, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, data) == nil
}
