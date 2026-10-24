package launchcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupOldLogs 在临时目录准备两个旧日志文件，返回 (srcDir, 内容映射)。
func setupOldLogs(t *testing.T) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	contents := map[string]string{
		"需求澄清": "旧日志-需求澄清\nline2\n",
		"cli.build": "old log for cli.build\n",
	}
	for name, content := range contents {
		if err := os.WriteFile(filepath.Join(dir, name+".log"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, contents
}

func TestArchiveLogs三种情况(t *testing.T) {
	srcDir, contents := setupOldLogs(t)
	dstDir := filepath.Join(t.TempDir(), "executors")

	// 改名映射：中文名 → 确认后的新名；cli.build 同名迁移；不存在的条目
	renames := map[string]string{
		"需求澄清":  "exec-10086",  // 改名映射
		"cli.build": "cli.build", // 同名迁移
		"不存在者":    "exec-99999", // 源缺失
	}
	results, err := ArchiveLogs(srcDir, dstDir, renames)
	if err != nil {
		t.Fatalf("ArchiveLogs 失败：%v", err)
	}
	if len(results) != 3 {
		t.Fatalf("应返回 3 条结果，实际 %d", len(results))
	}

	byDest := map[string]ArchiveResult{}
	for _, r := range results {
		byDest[filepath.Base(r.Dest)] = r
	}

	// 改名映射：内容一致
	r1 := byDest["exec-10086.log"]
	if !r1.Copied {
		t.Fatalf("改名映射应拷贝：%+v", r1)
	}
	data, err := os.ReadFile(filepath.Join(dstDir, "exec-10086.log"))
	if err != nil || string(data) != contents["需求澄清"] {
		t.Fatalf("改名拷贝内容不符：%q err=%v", data, err)
	}

	// 同名迁移
	if r2 := byDest["cli.build.log"]; !r2.Copied {
		t.Fatalf("同名迁移应拷贝：%+v", r2)
	}

	// 源缺失：跳过不报错
	r3 := byDest["exec-99999.log"]
	if r3.Copied || !strings.Contains(r3.Reason, "不存在") {
		t.Fatalf("源缺失应跳过并说明原因：%+v", r3)
	}
}

func TestArchiveLogs无副作用于源目录(t *testing.T) {
	srcDir, contents := setupOldLogs(t)
	dstDir := filepath.Join(t.TempDir(), "executors")

	_, err := ArchiveLogs(srcDir, dstDir, map[string]string{"需求澄清": "exec-10086"})
	if err != nil {
		t.Fatal(err)
	}

	// 源文件仍存在且内容不变；源目录没有新增文件
	for name, content := range contents {
		data, err := os.ReadFile(filepath.Join(srcDir, name+".log"))
		if err != nil {
			t.Fatalf("源文件 %s 被删除：%v", name, err)
		}
		if string(data) != content {
			t.Fatalf("源文件 %s 内容被修改", name)
		}
	}
	entries, _ := os.ReadDir(srcDir)
	if len(entries) != len(contents) {
		t.Fatalf("源目录文件数应不变（%d），实际 %d", len(contents), len(entries))
	}
	// 目标目录只包含 1 个新文件
	dstEntries, _ := os.ReadDir(dstDir)
	if len(dstEntries) != 1 {
		t.Fatalf("目标目录应只有 1 个文件，实际 %d", len(dstEntries))
	}
}

func TestArchiveLogs空映射(t *testing.T) {
	results, err := ArchiveLogs(t.TempDir(), filepath.Join(t.TempDir(), "dst"), map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("空映射应返回 0 条，实际 %d", len(results))
	}
}
