package stats

import (
	"testing"
	"time"

	"awem/internal/logs"
)

func ts(h, m int) time.Time { return time.Date(2026, 10, 25, h, m, 0, 0, time.UTC) }

func ev(t time.Time, typ logs.EventType, executor, detail string) logs.Event {
	return logs.Event{Time: t, Type: typ, Executor: executor, Detail: detail}
}

func boolPtr(v bool) *bool { return &v }

func wantSegments(t *testing.T, tl Timeline, want []Segment) {
	t.Helper()
	if len(tl.Segments) != len(want) {
		t.Fatalf("区间数量 = %d，期望 %d：%+v", len(tl.Segments), len(want), tl.Segments)
	}
	for i, w := range want {
		g := tl.Segments[i]
		if !g.From.Equal(w.From) || !g.To.Equal(w.To) || g.State != w.State {
			t.Fatalf("区间 #%d = {%v → %v %s}，期望 {%v → %v %s}",
				i, g.From.Format(time.RFC3339), g.To.Format(time.RFC3339), g.State,
				w.From.Format(time.RFC3339), w.To.Format(time.RFC3339), w.State)
		}
	}
}

// 启停：数据起点前无数据（斜纹），start→running，stop→stopped 延续到窗口末。
func TestReplayStartStop(t *testing.T) {
	events := []logs.Event{
		ev(ts(10, 0), logs.EventExecutorStart, "e1", ""),
		ev(ts(11, 0), logs.EventExecutorStop, "e1", "人为停止"),
	}
	tl := Replay(events, "e1", ts(9, 0), ts(12, 0))
	if tl.DataStart == nil || !tl.DataStart.Equal(ts(10, 0)) {
		t.Fatalf("数据起点 = %v，期望 10:00", tl.DataStart)
	}
	if tl.DataEnd == nil || !tl.DataEnd.Equal(ts(11, 0)) {
		t.Fatalf("数据终点 = %v，期望 11:00", tl.DataEnd)
	}
	wantSegments(t, tl, []Segment{
		{From: ts(9, 0), To: ts(10, 0), State: StateNoData},
		{From: ts(10, 0), To: ts(11, 0), State: StateRunning},
		{From: ts(11, 0), To: ts(12, 0), State: StateStopped},
	})
	if len(tl.Crashes) != 0 {
		t.Fatalf("不应有崩溃点：%+v", tl.Crashes)
	}
}

// 崩溃自动拉起：running → backoff → running；崩溃点携带日志定位。
func TestReplayCrashAutoRestart(t *testing.T) {
	auto := true
	crash := ev(ts(10, 30), logs.EventExecutorCrash, "e1", "崩溃退出（exit 1）")
	crash.Restart = &auto
	crash.LogFile = "e1.log"
	crash.LogOffset = 4096
	events := []logs.Event{
		ev(ts(10, 0), logs.EventExecutorStart, "e1", ""),
		crash,
		ev(ts(10, 31), logs.EventExecutorRestart, "e1", "第 1 次自动拉起"),
		ev(ts(11, 0), logs.EventExecutorStop, "e1", "人为停止"),
	}
	tl := Replay(events, "e1", ts(10, 0), ts(12, 0))
	wantSegments(t, tl, []Segment{
		{From: ts(10, 0), To: ts(10, 30), State: StateRunning},
		{From: ts(10, 30), To: ts(10, 31), State: StateBackoff},
		{From: ts(10, 31), To: ts(11, 0), State: StateRunning},
		{From: ts(11, 0), To: ts(12, 0), State: StateStopped},
	})
	if len(tl.Crashes) != 1 {
		t.Fatalf("应有一个崩溃点：%+v", tl.Crashes)
	}
	c := tl.Crashes[0]
	if !c.At.Equal(ts(10, 30)) || !c.AutoRestart || c.LogFile != "e1.log" || c.LogOffset != 4096 {
		t.Fatalf("崩溃点字段不符：%+v", c)
	}
}

// 崩溃明确不拉起 → stopped；旧事件无 restart 字段时按 detail 兼容。
func TestReplayCrashNoRestartAndLegacyDetail(t *testing.T) {
	no := false
	crash := ev(ts(10, 30), logs.EventExecutorCrash, "e1", "崩溃退出（exit 1），restart.enabled=false 不拉起")
	crash.Restart = &no
	legacy := ev(ts(10, 45), logs.EventExecutorCrash, "e1", "崩溃退出（exit 1），restart.enabled=false 不拉起")
	events := []logs.Event{
		ev(ts(10, 0), logs.EventExecutorStart, "e1", ""),
		crash,
		legacy,
	}
	tl := Replay(events, "e1", ts(10, 0), ts(11, 0))
	wantSegments(t, tl, []Segment{
		{From: ts(10, 0), To: ts(10, 30), State: StateRunning},
		{From: ts(10, 30), To: ts(11, 0), State: StateStopped},
	})
	if len(tl.Crashes) != 2 || tl.Crashes[0].AutoRestart || tl.Crashes[1].AutoRestart {
		t.Fatalf("两次崩溃都应判定为不拉起：%+v", tl.Crashes)
	}
}

// 旧崩溃事件（无 restart 字段、detail 无“不拉起”）按自动拉起处理。
func TestReplayLegacyCrashDefaultsToAutoRestart(t *testing.T) {
	events := []logs.Event{
		ev(ts(10, 0), logs.EventExecutorStart, "e1", ""),
		ev(ts(10, 30), logs.EventExecutorCrash, "e1", "崩溃退出（exit 1）"),
		ev(ts(10, 31), logs.EventExecutorRestart, "e1", "第 1 次自动拉起"),
	}
	tl := Replay(events, "e1", ts(10, 0), ts(11, 0))
	if len(tl.Crashes) != 1 || !tl.Crashes[0].AutoRestart {
		t.Fatalf("旧崩溃事件应按自动拉起处理：%+v", tl.Crashes)
	}
	wantSegments(t, tl, []Segment{
		{From: ts(10, 0), To: ts(10, 30), State: StateRunning},
		{From: ts(10, 30), To: ts(10, 31), State: StateBackoff},
		{From: ts(10, 31), To: ts(11, 0), State: StateRunning},
	})
}

// daemon 优雅停止到下次启动之间为无数据；daemon_start 后到下一执行器事件前为 stopped。
func TestReplayDaemonStopGap(t *testing.T) {
	events := []logs.Event{
		ev(ts(10, 0), logs.EventExecutorStart, "e1", ""),
		ev(ts(10, 30), logs.EventExecutorStop, "e1", "人为停止"),
		ev(ts(10, 40), logs.EventDaemonStop, "", "收到退出信号"),
		ev(ts(11, 0), logs.EventDaemonStart, "", "awem 启动"),
		ev(ts(11, 10), logs.EventExecutorStart, "e1", ""),
	}
	tl := Replay(events, "e1", ts(9, 0), ts(12, 0))
	wantSegments(t, tl, []Segment{
		{From: ts(9, 0), To: ts(10, 0), State: StateNoData},
		{From: ts(10, 0), To: ts(10, 30), State: StateRunning},
		{From: ts(10, 30), To: ts(10, 40), State: StateStopped},
		{From: ts(10, 40), To: ts(11, 0), State: StateNoData},
		{From: ts(11, 0), To: ts(11, 10), State: StateStopped},
		{From: ts(11, 10), To: ts(12, 0), State: StateRunning},
	})
}

// 窗口裁剪：区间与崩溃点按 [from,to] 裁剪，窗口外崩溃不返回。
func TestReplayWindowClip(t *testing.T) {
	auto := true
	crash := ev(ts(10, 30), logs.EventExecutorCrash, "e1", "崩溃退出（exit 1）")
	crash.Restart = &auto
	events := []logs.Event{
		ev(ts(10, 0), logs.EventExecutorStart, "e1", ""),
		crash,
		ev(ts(10, 40), logs.EventExecutorRestart, "e1", "第 1 次自动拉起"),
		ev(ts(11, 0), logs.EventExecutorStop, "e1", "人为停止"),
	}
	tl := Replay(events, "e1", ts(10, 15), ts(10, 50))
	wantSegments(t, tl, []Segment{
		{From: ts(10, 15), To: ts(10, 30), State: StateRunning},
		{From: ts(10, 30), To: ts(10, 40), State: StateBackoff},
		{From: ts(10, 40), To: ts(10, 50), State: StateRunning},
	})
	if len(tl.Crashes) != 1 || !tl.Crashes[0].At.Equal(ts(10, 30)) {
		t.Fatalf("窗口内崩溃点不符：%+v", tl.Crashes)
	}
	later := Replay(events, "e1", ts(10, 45), ts(10, 50))
	if len(later.Crashes) != 0 {
		t.Fatalf("窗口外崩溃点不应返回：%+v", later.Crashes)
	}
}

// 无事件：数据起止为空，区间为空（页面显示“暂无事件数据”）。
func TestReplayNoEvents(t *testing.T) {
	tl := Replay(nil, "e1", ts(9, 0), ts(10, 0))
	if tl.DataStart != nil || tl.DataEnd != nil {
		t.Fatalf("无事件时数据起止应为空：%+v", tl)
	}
	if len(tl.Segments) != 0 || len(tl.Crashes) != 0 {
		t.Fatalf("无事件时区间/崩溃应为空：%+v", tl)
	}
}

// 仅有其他执行器事件：数据边界来自全局链；目标执行器按 stopped 延续。
func TestReplayOtherExecutorEvents(t *testing.T) {
	events := []logs.Event{
		ev(ts(10, 0), logs.EventExecutorStart, "other", ""),
		ev(ts(10, 30), logs.EventExecutorStop, "other", ""),
	}
	tl := Replay(events, "e1", ts(10, 0), ts(11, 0))
	wantSegments(t, tl, []Segment{{From: ts(10, 0), To: ts(11, 0), State: StateStopped}})
}

// 乱序输入按时间稳定排序；连续同态事件合并为一段。
func TestReplayUnsortedAndMerge(t *testing.T) {
	events := []logs.Event{
		ev(ts(10, 20), logs.EventExecutorStart, "e1", ""),
		ev(ts(10, 0), logs.EventExecutorStart, "e1", ""),
		ev(ts(10, 40), logs.EventExecutorStart, "e1", ""),
	}
	tl := Replay(events, "e1", ts(10, 0), ts(11, 0))
	wantSegments(t, tl, []Segment{{From: ts(10, 0), To: ts(11, 0), State: StateRunning}})
}

// 窗口整体早于数据起点：整窗无数据。
func TestReplayWindowBeforeData(t *testing.T) {
	events := []logs.Event{ev(ts(10, 0), logs.EventExecutorStart, "e1", "")}
	tl := Replay(events, "e1", ts(8, 0), ts(9, 0))
	wantSegments(t, tl, []Segment{{From: ts(8, 0), To: ts(9, 0), State: StateNoData}})
}
