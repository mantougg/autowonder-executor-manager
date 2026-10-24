// connstate.go：连接状态解析（T05-F5）。从 connect 日志行解析连接状态，
// 关键字实标自 autowonder@0.3.22 vendored daemon 二进制（任务文档变更记录）：
//   connected    ← `ws connected url=… executorId=…`
//   reconnecting ← `ws disconnected, reconnecting executorId=… retryIn=…` /
//                  `ws disconnected before stable interval, reconnecting …` /
//                  `runtime is restarting; retry after reconnect`
// 大小写不敏感子串匹配（真实日志行带时间戳前缀）；重连关键字优先于连接
// （一行同时命中时以断线为准）。选子串匹配而非正则：开销可控（design F5）。
package supervisor

import "strings"

// ParseConnLine 从一行 connect 日志解析连接状态；未命中任何关键字返回
// (ConnUnknown, false)。
func ParseConnLine(line string) (ConnState, bool) {
	l := strings.ToLower(line)
	if strings.Contains(l, "ws disconnected") ||
		strings.Contains(l, "reconnecting") ||
		strings.Contains(l, "retry after reconnect") {
		return ConnReconnecting, true
	}
	if strings.Contains(l, "ws connected") {
		return ConnConnected, true
	}
	return ConnUnknown, false
}
