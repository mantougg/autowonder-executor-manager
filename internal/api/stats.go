// stats.go：运行统计端点（一期：单执行器运行时间带）。
package api

import (
	"net/http"
	"time"

	"awem/internal/stats"
)

// registerStatsRoutes 注册统计路由（挂在 /api/ 鉴权中间件下）。
func (s *Server) registerStatsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/stats/executors/{name}/timeline", s.handleTimeline)
}

// timelineView 是 timeline 响应（窗口、数据起止、区间、崩溃点与定位标识）。
type timelineView struct {
	Name      string          `json:"name"`
	From      time.Time       `json:"from"`
	To        time.Time       `json:"to"`
	DataStart *time.Time      `json:"data_start"`
	DataEnd   *time.Time      `json:"data_end"`
	Segments  []stats.Segment `json:"segments"`
	Crashes   []stats.Crash   `json:"crashes"`
}

// handleTimeline GET /api/stats/executors/{name}/timeline?from=&to=
func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg := s.currentConfig()
	if cfg.FindExecutor(name) == nil {
		handleAPIError(w, notFound("执行器 "+name+" 不存在"))
		return
	}
	from, to, err := parseTimelineWindow(r)
	if err != nil {
		handleAPIError(w, err)
		return
	}
	events, err := s.opts.Logs.ReadEvents()
	if err != nil {
		handleAPIError(w, internalErr(err))
		return
	}
	tl := stats.Replay(events, name, from, to)
	for i := range tl.Crashes {
		tl.Crashes[i].Locatable = s.opts.Logs.ValidExecutorLogFile(name, tl.Crashes[i].LogFile)
	}
	writeJSON(w, http.StatusOK, timelineView{
		Name: name, From: from, To: to,
		DataStart: tl.DataStart, DataEnd: tl.DataEnd,
		Segments: tl.Segments, Crashes: tl.Crashes,
	})
}

// parseTimelineWindow 解析并校验 from/to（RFC3339，from 必须早于 to）。
func parseTimelineWindow(r *http.Request) (time.Time, time.Time, error) {
	q := r.URL.Query()
	fromS, toS := q.Get("from"), q.Get("to")
	if fromS == "" || toS == "" {
		return time.Time{}, time.Time{}, badRequest("from/to 必填（RFC3339 时间）")
	}
	from, err := time.Parse(time.RFC3339Nano, fromS)
	if err != nil {
		return time.Time{}, time.Time{}, badRequest("from 不是合法的 RFC3339 时间：" + fromS)
	}
	to, err := time.Parse(time.RFC3339Nano, toS)
	if err != nil {
		return time.Time{}, time.Time{}, badRequest("to 不是合法的 RFC3339 时间：" + toS)
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, badRequest("from 必须早于 to")
	}
	return from, to, nil
}
