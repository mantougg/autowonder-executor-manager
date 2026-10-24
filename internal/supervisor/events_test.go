// events_test.go：T05-F6 事件联动收口验收（全生命周期事件序列 + 令牌安全）。
package supervisor

import (
	"encoding/json"
	"strings"
	"testing"
)

// eventEntry 是从 events.log 解析出的一条事件（按文件行序=时间序）。
type eventEntry struct {
	Type      string `json:"type"`
	Executor  string `json:"executor"`
	Detail    string `json:"detail"`
	Restart   *bool  `json:"restart"`
	LogFile   string `json:"log_file"`
	LogOffset int64  `json:"log_offset"`
}

// readEvents 按行序解析事件日志。
func (e *testEnv) readEvents(t *testing.T) []eventEntry {
	t.Helper()
	text := e.eventsText(t)
	var out []eventEntry
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		var ev eventEntry
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("事件行不是合法 JSON：%q（%v）", line, err)
		}
		out = append(out, ev)
	}
	return out
}

// 验收：冒烟场景（启/停/崩溃/拉起）后 events.log 事件序列完整且顺序合理，
// 详情含足够排障信息（退出码），且不含完整令牌。
func TestEventSequenceFullLifecycle(t *testing.T) {
	e := newTestEnv(t)
	e.setCfg(t, execWith("e1", "-fake-mode=crash -fake-after=30ms -fake-exit-code=7"))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	waitFor(t, "至少一次自动拉起", func() bool {
		return e.snapshotOf(t, "e1").Restarts >= 1
	})
	if err := e.sup.StopExecutor("e1"); err != nil {
		t.Fatalf("停止失败：%v", err)
	}
	// 退避中被停止：不再有新事件涌入，等状态落定
	waitFor(t, "状态 Stopped", func() bool { return e.snapshotOf(t, "e1").State == StateStopped })

	events := e.readEvents(t)
	// 抽取 e1 的事件类型序列
	var seq []string
	for _, ev := range events {
		if ev.Executor == "e1" {
			seq = append(seq, ev.Type)
		}
	}
	// 顺序断言：start → (crash → restart)+ → stop
	idx := func(want string, from int) int {
		for i := from; i < len(seq); i++ {
			if seq[i] == want {
				return i
			}
		}
		return -1
	}
	iStart := idx("executor_start", 0)
	iCrash := idx("executor_crash", iStart)
	iRestart := idx("executor_restart", iCrash)
	iStop := idx("executor_stop", iRestart)
	if iStart < 0 || iCrash < 0 || iRestart < 0 || iStop < 0 {
		t.Fatalf("事件序列不完整：%v", seq)
	}
	if !(iStart < iCrash && iCrash < iRestart && iRestart < iStop) {
		t.Fatalf("事件顺序不合理：%v", seq)
	}
	// 排障信息：崩溃事件含退出码；停止事件含退出描述
	crashDetail, stopDetail := "", ""
	var crashEvent *eventEntry
	for _, ev := range events {
		if ev.Executor != "e1" {
			continue
		}
		if ev.Type == "executor_crash" && crashDetail == "" {
			crashDetail = ev.Detail
			cp := ev
			crashEvent = &cp
		}
		if ev.Type == "executor_stop" {
			stopDetail = ev.Detail
		}
	}
	if !strings.Contains(crashDetail, "exit 7") {
		t.Fatalf("崩溃事件缺退出码：%q", crashDetail)
	}
	// 崩溃事件增量字段：自动拉起意图 + 日志定位（文件标识 + 字节偏移）。
	if crashEvent == nil || crashEvent.Restart == nil || !*crashEvent.Restart {
		t.Fatalf("崩溃事件应标记将自动拉起：%+v", crashEvent)
	}
	if crashEvent.LogFile != "e1.log" || crashEvent.LogOffset < 0 {
		t.Fatalf("崩溃事件应携带日志定位：%+v", crashEvent)
	}
	if !strings.Contains(stopDetail, "退出") && !strings.Contains(stopDetail, "取消拉起") {
		t.Fatalf("停止事件缺描述：%q", stopDetail)
	}
	// 令牌安全：事件日志不得出现完整执行器令牌（命令含 --token test-token-123456）
	if strings.Contains(e.eventsText(t), "test-token-123456") {
		t.Fatal("事件日志泄漏完整令牌")
	}
}

// 验收：启动失败事件详情含错误摘要（npx 缺失场景）。
func TestEventSpawnFailDetail(t *testing.T) {
	e := newTestEnv(t)
	e.sup.opts.ResolveNpx = func() (string, error) { return "", ErrNpxNotFound }
	e.setCfg(t, execWith("e1", ""))
	_ = e.sup.StartExecutor("e1")
	events := e.readEvents(t)
	if len(events) == 0 || events[len(events)-1].Type != "executor_spawn_fail" {
		t.Fatalf("应记录启动失败事件：%v", events)
	}
	if !strings.Contains(events[len(events)-1].Detail, "npx") {
		t.Fatalf("失败详情应含错误摘要：%q", events[len(events)-1].Detail)
	}
}

// 崩溃明确不拉起：事件 restart=false 且仍携带日志定位（时间带降级依据）。
func TestEventCrashNoRestartFields(t *testing.T) {
	e := newTestEnv(t)
	ex := execWith("e1", "-fake-mode=crash -fake-after=20ms -fake-exit-code=9")
	disabled := false
	ex.Restart.Enabled = &disabled
	e.setCfg(t, ex)
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	waitFor(t, "状态 Failed", func() bool { return e.snapshotOf(t, "e1").State == StateFailed })

	var crash *eventEntry
	for _, ev := range e.readEvents(t) {
		if ev.Type == "executor_crash" {
			cp := ev
			crash = &cp
		}
	}
	if crash == nil {
		t.Fatal("应记录崩溃事件")
	}
	if crash.Restart == nil || *crash.Restart {
		t.Fatalf("不拉起崩溃应记录 restart=false：%+v", crash)
	}
	if crash.LogFile != "e1.log" {
		t.Fatalf("崩溃事件应携带日志文件标识：%+v", crash)
	}
}
