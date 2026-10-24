package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestManager 建一个临时目录的 Manager。
func newTestManager(t *testing.T, maxBytes int64) *Manager {
	t.Helper()
	base := t.TempDir()
	m := NewManager(filepath.Join(base, "executors"), filepath.Join(base, "events.log"), maxBytes, 5, 7)
	t.Cleanup(m.Close)
	return m
}

// TestTail尾部N行 写 K 行 → Tail(N<K) 返回最后 N 行。
func TestTail尾部N行(t *testing.T) {
	m := newTestManager(t, 0)
	s, err := m.Sink("e1")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(s, "line-%03d\n", i)
	}
	got, err := m.Tail("e1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 || got[0] != "line-091" || got[9] != "line-100" {
		t.Fatalf("Tail 结果不符：%v", got)
	}
	// N 超过总数 → 返回全部
	all, _ := m.Tail("e1", 500)
	if len(all) != 100 {
		t.Fatalf("Tail(500) 应返回 100 行，实际 %d", len(all))
	}
}

// TestTail文件不存在空态 文件不存在不报错、返回空。
func TestTail文件不存在空态(t *testing.T) {
	m := newTestManager(t, 0)
	got, err := m.Tail("不存在的执行器", 10)
	if err != nil {
		t.Fatalf("文件不存在不应报错：%v", err)
	}
	if len(got) != 0 {
		t.Fatalf("应返回空态，实际 %v", got)
	}
	// Follow 同样不报错
	sub, err := m.Follow("不存在的执行器")
	if err != nil {
		t.Fatalf("Follow 不应报错：%v", err)
	}
	sub.Close()
}

// TestReadLines历史分页 按行偏移分页读取。
func TestReadLines历史分页(t *testing.T) {
	m := newTestManager(t, 0)
	s, _ := m.Sink("e1")
	for i := 1; i <= 50; i++ {
		fmt.Fprintf(s, "row-%02d\n", i)
	}
	page1, total, err := m.ReadLines("e1", 0, 20)
	if err != nil || total != 50 {
		t.Fatalf("total=%d err=%v", total, err)
	}
	if len(page1) != 20 || page1[0] != "row-01" || page1[19] != "row-20" {
		t.Fatalf("第 1 页不符：%v", page1)
	}
	page3, _, _ := m.ReadLines("e1", 40, 20)
	if len(page3) != 10 || page3[0] != "row-41" || page3[9] != "row-50" {
		t.Fatalf("末页不符：%v", page3)
	}
	// 越界 → 空、总数不变
	over, total, _ := m.ReadLines("e1", 50, 20)
	if len(over) != 0 || total != 50 {
		t.Fatalf("越界应空：%v total=%d", over, total)
	}
}

// TestFollow跨滚动无漏行 订阅后持续写入并触发滚动 → 收到全部新行。
func TestFollow跨滚动无漏行(t *testing.T) {
	m := newTestManager(t, 200) // 小阈值，写入过程中多次滚动
	s, err := m.Sink("e1")
	if err != nil {
		t.Fatal(err)
	}
	// 先写一些历史（订阅前，不应推送）
	for i := 0; i < 5; i++ {
		fmt.Fprint(s, "history\n")
	}

	sub, err := m.Follow("e1")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	time.Sleep(250 * time.Millisecond) // 等订阅就位（读到当前末尾）

	const newLines = 60
	go func() {
		for i := 1; i <= newLines; i++ {
			fmt.Fprintf(s, "new-%03d\n", i)
		}
	}()

	var got []string
	deadline := time.Now().Add(10 * time.Second)
	for len(got) < newLines && time.Now().Before(deadline) {
		select {
		case line := <-sub.C:
			got = append(got, line)
		case <-time.After(200 * time.Millisecond):
		}
	}
	if len(got) != newLines {
		t.Fatalf("Follow 收到 %d 行，期望 %d", len(got), newLines)
	}
	for i, want := range got {
		if want != fmt.Sprintf("new-%03d", i+1) {
			t.Fatalf("第 %d 行不符：%q（期望 new-%03d）——漏行或乱序", i+1, want, i+1)
		}
	}
	// 历史行不应被推送
	for _, l := range got {
		if l == "history" {
			t.Fatal("订阅前历史被推送")
		}
	}
}

// TestPurge删除全部日志文件。
func TestPurge删除全部日志文件(t *testing.T) {
	m := newTestManager(t, 100)
	s, _ := m.Sink("e1")
	for i := 0; i < 30; i++ {
		fmt.Fprint(s, "0123456789012345678901234567890123456789\n") // 触发滚动
	}
	if len(m.ListRotated("e1")) == 0 {
		t.Fatal("应已产生滚动文件")
	}
	if err := m.Purge("e1"); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(m.executorsDir, "e1.log*"))
	if len(matches) != 0 {
		t.Fatalf("Purge 后仍有残留：%v", matches)
	}
	// purge 后再取 sink → 新文件可写
	s2, err := m.Sink("e1")
	if err != nil {
		t.Fatalf("purge 后重开失败：%v", err)
	}
	fmt.Fprint(s2, "after-purge\n")
	lines, _ := m.Tail("e1", 10)
	if len(lines) != 1 || lines[0] != "after-purge" {
		t.Fatalf("purge 后写入读取不符：%v", lines)
	}
}

// TestCleanup联动事件 清理产生事件记录（读回 events.log 验证）。
func TestCleanup联动事件(t *testing.T) {
	m := newTestManager(t, 0)
	now := time.Now()
	// 构造过期滚动文件
	if err := os.MkdirAll(m.executorsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	expired := filepath.Join(m.executorsDir, "e1.log.1")
	os.WriteFile(expired, []byte("x"), 0o644)
	os.Chtimes(expired, now.AddDate(0, 0, -30), now.AddDate(0, 0, -30))

	n, err := m.Cleanup(now)
	if err != nil || n != 1 {
		t.Fatalf("Cleanup 应删 1 个：%d err=%v", n, err)
	}
	// 事件已写入 events.log
	data, err := os.ReadFile(m.eventsPath)
	if err != nil {
		t.Fatalf("读取事件日志失败：%v", err)
	}
	if !strings.Contains(string(data), "log_cleanup") || !strings.Contains(string(data), "1 个过期日志文件") {
		t.Fatalf("清理事件缺失：%s", data)
	}
}
