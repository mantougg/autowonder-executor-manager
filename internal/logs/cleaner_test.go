package logs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeFile 在 dir 创建文件并注入 mtime。
func makeFile(t *testing.T, dir, name string, mtime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// Test清理只删过期 mtime 用注入时钟控制新旧。
func Test清理只删过期(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.Local)
	old := now.AddDate(0, 0, -8)  // 超过 7 天
	recent := now.AddDate(0, 0, -1)

	makeFile(t, dir, "e1.log.1", old)     // 过期 → 删
	makeFile(t, dir, "e1.log.2", old)     // 过期 → 删
	makeFile(t, dir, "e1.log.3", recent)  // 未过期 → 留
	makeFile(t, dir, "e1.log", recent)    // 活动文件 → 留
	makeFile(t, dir, "e2.log.1", recent)  // 未过期 → 留
	makeFile(t, dir, "events.log.1", old) // 过期 → 删
	makeFile(t, dir, "readme.txt", old)   // 非滚动文件 → 留

	var events []string
	removed, err := CleanupExpired(dir, 7, now, func(p string) { events = append(events, p) })
	if err != nil {
		t.Fatalf("CleanupExpired 失败：%v", err)
	}
	if len(removed) != 3 || len(events) != 3 {
		t.Fatalf("应删 3 个过期滚动文件，实际 %d（事件 %d）：%v", len(removed), len(events), removed)
	}
	for _, p := range removed {
		base := filepath.Base(p)
		if base != "e1.log.1" && base != "e1.log.2" && base != "events.log.1" {
			t.Fatalf("误删：%s", p)
		}
	}
	// 留存的文件
	for _, keep := range []string{"e1.log.3", "e1.log", "e2.log.1", "readme.txt"} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Fatalf("不应删除 %s：%v", keep, err)
		}
	}
	// 清理产生事件记录（回调）
	if len(events) != len(removed) {
		t.Fatalf("事件数 %d 与删除数 %d 不一致", len(events), len(removed))
	}
}

// Test清理目录不存在与默认天数。
func Test清理目录不存在与默认天数(t *testing.T) {
	if removed, err := CleanupExpired(filepath.Join(t.TempDir(), "不存在"), 7, time.Now(), nil); err != nil || len(removed) != 0 {
		t.Fatalf("目录不存在应静默返回空：%v %v", removed, err)
	}
	// retentionDays<=0 → 默认 7 天
	dir := t.TempDir()
	now := time.Now()
	makeFile(t, dir, "e.log.1", now.AddDate(0, 0, -30))
	removed, err := CleanupExpired(dir, 0, now, nil)
	if err != nil || len(removed) != 1 {
		t.Fatalf("默认 7 天应删除 30 天前文件：%v %v", removed, err)
	}
}
