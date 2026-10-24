// wsclient.go：daemon.Client 的 WebSocket 日志跟随（T07-F7 的 -f 实现；
// 复用 /ws/logs/{name} 端点与消息契约，CLI 与 Web 同一数据面）。
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// LogEvent 是 WS 日志流的一条事件（信封同 API ws.go 契约）。
type LogEvent struct {
	Type    string   // backlog / line / error
	Text    string   // type=line 的行内容
	Lines   []string // type=backlog 的尾部回放
	Message string   // type=error
}

// FollowLogs 连接 /ws/logs/{name}?tail=N 并持续回调事件；ctx 取消或连接
// 断开即返回。客户端用 query token 鉴权（与浏览器同规则）。
func (c *Client) FollowLogs(ctx context.Context, name string, tail int, onEvent func(LogEvent) bool) error {
	url := strings.Replace(c.baseURL, "http", "ws", 1) +
		fmt.Sprintf("/ws/logs/%s?tail=%d&token=%s", name, tail, c.token)
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			var ae *APIError
			data := readRespBody(resp)
			var eb struct {
				Error APIError `json:"error"`
			}
			if json.Unmarshal(data, &eb) == nil && eb.Error.Code != "" {
				ae = &eb.Error
				ae.HTTPStatus = resp.StatusCode
				return ae
			}
			return &APIError{HTTPStatus: resp.StatusCode, Code: "not_found", Message: "执行器 " + name + " 不存在"}
		}
		return fmt.Errorf("连接日志流失败：%w", err)
	}
	defer conn.Close()

	// 读泵：帧 → 事件回调；onEvent 返回 false 主动停止
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(24 * time.Hour))
		_, data, err := conn.ReadMessage()
		if err != nil {
			// 正常关闭（服务端收口或 ctx 取消引发的本地关闭）
			return nil
		}
		var ev LogEvent
		if json.Unmarshal(data, &ev) != nil {
			continue
		}
		if !onEvent(ev) {
			return nil
		}
	}
}

// readRespBody 读尽响应体（错误解析用）。
func readRespBody(resp *http.Response) []byte {
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 512)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf
}
