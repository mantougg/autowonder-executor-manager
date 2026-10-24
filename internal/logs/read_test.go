package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 事件滚动链按旧→新读取并按时间排序；坏行跳过；增量字段可解析。
func TestReadEventsChain(t *testing.T) {
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "events.log")
	writeFile(t, eventsPath+".2", `{"time":"2026-10-25T09:00:00Z","type":"config_loaded"}`+"\n")
	writeFile(t, eventsPath+".1", `{"time":"2026-10-25T10:00:00Z","type":"executor_start","executor":"e1"}`+"\nnot-json\n")
	writeFile(t, eventsPath, `{"time":"2026-10-25T10:30:00Z","type":"executor_crash","executor":"e1","detail":"崩溃退出","restart":false,"log_file":"e1.log","log_offset":42}`+"\n")

	m := NewManager(filepath.Join(dir, "executors"), eventsPath, 0, 0, 0)
	evs, err := m.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 {
		t.Fatalf("应读取 3 条有效事件（坏行跳过），实际 %d：%+v", len(evs), evs)
	}
	if !evs[0].Time.Equal(time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("事件未按时间排序：%+v", evs)
	}
	last := evs[len(evs)-1]
	if last.Type != EventExecutorCrash || last.Restart == nil || *last.Restart || last.LogFile != "e1.log" || last.LogOffset != 42 {
		t.Fatalf("崩溃事件增量字段解析失败：%+v", last)
	}
}

// 按位定位：跨滚动链向前补齐，返回时间顺序、首行位置与 hasMore。
func TestReadBeforeAcrossChain(t *testing.T) {
	dir := t.TempDir()
	execDir := filepath.Join(dir, "executors")
	m := NewManager(execDir, filepath.Join(dir, "events.log"), 0, 0, 0)
	old := "old1\nold2\nold3\n"
	writeFile(t, filepath.Join(execDir, "e1.log.1"), old)
	cur := "cur1\ncur2\ncur3\n"
	writeFile(t, filepath.Join(execDir, "e1.log"), cur)

	before := int64(len("cur1\ncur2\n"))
	lines, startFile, startOffset, hasMore, err := m.ReadBefore("e1", "e1.log", before, 3)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(lines, "|") != "old3|cur1|cur2" {
		t.Fatalf("定位结果不符：%v", lines)
	}
	if startFile != "e1.log.1" || startOffset != int64(len("old1\nold2\n")) {
		t.Fatalf("首行位置不符：file=%s offset=%d", startFile, startOffset)
	}
	if !hasMore {
		t.Fatal("前面仍有更早日志，hasMore 应为 true")
	}

	lines, _, _, hasMore, err = m.ReadBefore("e1", "e1.log", before, 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(lines, "|") != "old1|old2|old3|cur1|cur2" {
		t.Fatalf("补齐全部历史不符：%v", lines)
	}
	if hasMore {
		t.Fatal("已到链首，hasMore 应为 false")
	}

	lines, _, _, hasMore, err = m.ReadBefore("e1", "e1.log.1", int64(len(old)), 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(lines, "|") != "old1|old2|old3" || hasMore {
		t.Fatalf("旧文件整读不符：%v hasMore=%v", lines, hasMore)
	}
}

// 路径穿越与跨执行器文件必须拒绝。
func TestReadBeforeRejectsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	execDir := filepath.Join(dir, "executors")
	m := NewManager(execDir, filepath.Join(dir, "events.log"), 0, 0, 0)
	writeFile(t, filepath.Join(execDir, "e1.log"), "x\n")
	writeFile(t, filepath.Join(execDir, "other.log"), "y\n")
	writeFile(t, filepath.Join(dir, "events.log"), "{}\n")

	for _, bad := range []string{
		"", "../events.log", filepath.Join(dir, "events.log"), "e1.log.0",
		"e1.log.x", "other.log", "e1.log.1", "..", "e1.log/../e1.log",
	} {
		if m.ValidExecutorLogFile("e1", bad) {
			t.Fatalf("非法文件被放行：%q", bad)
		}
		if _, _, _, _, err := m.ReadBefore("e1", bad, 10, 10); err == nil {
			t.Fatalf("ReadBefore 应拒绝非法文件：%q", bad)
		}
	}
	for _, ok := range []string{"e1.log"} {
		if !m.ValidExecutorLogFile("e1", ok) {
			t.Fatalf("合法文件被拒绝：%q", ok)
		}
	}
}

// Position 返回当前日志文件基名与字节大小。
func TestPosition(t *testing.T) {
	dir := t.TempDir()
	execDir := filepath.Join(dir, "executors")
	m := NewManager(execDir, filepath.Join(dir, "events.log"), 0, 0, 0)
	writeFile(t, filepath.Join(execDir, "e1.log"), "abc\n")
	file, size := m.Position("e1")
	if file != "e1.log" || size != 4 {
		t.Fatalf("Position = %q,%d，期望 e1.log,4", file, size)
	}
	if file, size := m.Position("missing"); file != "" || size != 0 {
		t.Fatalf("不存在执行器应为空位置：%q,%d", file, size)
	}
}
