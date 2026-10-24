// stats.go：管理事件重放（一期：单执行器运行时间带）。
package stats

import (
	"sort"
	"strings"
	"time"

	"awem/internal/logs"
)

// 状态区间取值（与前端时间带颜色一一对应）。
const (
	StateRunning = "running"
	StateBackoff = "backoff"
	StateStopped = "stopped"
	StateNoData  = "nodata"
)

// Segment 是一段状态区间（窗口裁剪后，from < to）。
type Segment struct {
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
	State string    `json:"state"`
}

// Crash 是一个崩溃点及其日志定位信息。
type Crash struct {
	At          time.Time `json:"at"`
	LogFile     string    `json:"log_file,omitempty"`
	LogOffset   int64     `json:"log_offset,omitempty"`
	AutoRestart bool      `json:"auto_restart"`
	Locatable   bool      `json:"locatable"` // 由 API 层按日志链存在性填充
}

// Timeline 是窗口内的重放结果；DataStart/DataEnd 为 events.log 链的数据边界。
type Timeline struct {
	DataStart *time.Time `json:"data_start"`
	DataEnd   *time.Time `json:"data_end"`
	Segments  []Segment  `json:"segments"`
	Crashes   []Crash    `json:"crashes"`
}

// Replay 重放事件序列，返回指定执行器在 [from, to] 窗口内的状态区间与崩溃点。
// events 允许乱序（内部按时间稳定排序）；executor 为空时只重放 daemon 全局事件。
//
// 语义（design §9）：
//   - executor_start / executor_restart → running；executor_stop → stopped；
//   - executor_crash → 崩溃点；将自动拉起 → backoff，否则 → stopped；
//   - daemon_stop → 无数据（直到 daemon_start）；daemon_start 后到下一执行器事件前按 stopped；
//   - events.log 滚动起点之前、窗口整体早于数据起点 → 无数据；
//   - 最近事件之后到窗口末按最后状态延续（daemon 非优雅停机无法识别，已知限制）。
func Replay(events []logs.Event, executor string, from, to time.Time) Timeline {
	tl := Timeline{Segments: []Segment{}, Crashes: []Crash{}}
	if !from.Before(to) || len(events) == 0 {
		return tl
	}
	evs := make([]logs.Event, len(events))
	copy(evs, events)
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].Time.Before(evs[j].Time) })

	dataStart, dataEnd := evs[0].Time, evs[len(evs)-1].Time
	tl.DataStart = &dataStart
	tl.DataEnd = &dataEnd
	if dataStart.After(to) {
		tl.Segments = append(tl.Segments, Segment{From: from, To: to, State: StateNoData})
		return tl
	}

	state := StateStopped
	cur := dataStart
	var raw []Segment
	closeAt := func(end time.Time) {
		if end.After(cur) {
			raw = append(raw, Segment{From: cur, To: end, State: state})
		}
	}
	for _, e := range evs {
		if e.Time.After(to) {
			break
		}
		next := ""
		switch e.Type {
		case logs.EventDaemonStart:
			next = StateStopped // daemon 重启后旧子进程已不在，按 stopped 起算
		case logs.EventDaemonStop:
			next = StateNoData
		case logs.EventExecutorStart, logs.EventExecutorRestart:
			if e.Executor == executor {
				next = StateRunning
			}
		case logs.EventExecutorStop:
			if e.Executor == executor {
				next = StateStopped
			}
		case logs.EventExecutorCrash:
			if e.Executor == executor {
				auto := autoRestart(e)
				tl.Crashes = append(tl.Crashes, Crash{
					At: e.Time, LogFile: e.LogFile, LogOffset: e.LogOffset, AutoRestart: auto,
				})
				if auto {
					next = StateBackoff
				} else {
					next = StateStopped
				}
			}
		}
		if next == "" || next == state {
			continue
		}
		closeAt(e.Time)
		cur = e.Time
		state = next
	}
	closeAt(to)

	// 窗口前缀：滚动起点之前为无数据（斜纹底）。
	if from.Before(dataStart) {
		tl.Segments = append(tl.Segments, Segment{From: from, To: dataStart, State: StateNoData})
	}
	for _, s := range raw {
		start := s.From
		if start.Before(from) {
			start = from
		}
		end := s.To
		if end.After(to) {
			end = to
		}
		if !end.After(start) {
			continue
		}
		// 与上一段同态且首尾相接 → 合并（窗口裁剪后仍保持连续）
		if n := len(tl.Segments); n > 0 {
			last := &tl.Segments[n-1]
			if last.State == s.State && last.To.Equal(start) {
				last.To = end
				continue
			}
		}
		tl.Segments = append(tl.Segments, Segment{From: start, To: end, State: s.State})
	}

	crashes := tl.Crashes[:0]
	for _, c := range tl.Crashes {
		if c.At.Before(from) || c.At.After(to) {
			continue
		}
		crashes = append(crashes, c)
	}
	tl.Crashes = crashes
	return tl
}

// autoRestart 判定崩溃后是否将自动拉起：新事件用 restart 字段；
// 旧事件（无该字段）按 detail 兼容——含“不拉起”视为不拉起。
func autoRestart(e logs.Event) bool {
	if e.Restart != nil {
		return *e.Restart
	}
	return !strings.Contains(e.Detail, "不拉起")
}
