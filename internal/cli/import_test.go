// import_test.go：awem import 验收（T07-F8）。用真实模板形态样例：
// 中文名（走改名）、坏行（跳过并报告）、占位符行（跳过）；--with-logs
// 归档后日志出现在新位置；汇总与落盘一致。
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awem/internal/config"
	"awem/internal/exitcode"
)

// importScript 测试脚本样例（真实模板的 EXECUTORS 形态）。
const importScript = `#!/usr/bin/env bash
EXECUTORS=(
  "NO1需求澄清|npx -y autowonder@0.3.22 connect --ws-url ws://127.0.0.1:1/ws --token import-tok-123456 --executor-id 30001 --provider qoder --model qfmodel --reasoning-effort medium"
  "ascii-exec|npx -y autowonder@0.3.22 connect --ws-url ws://127.0.0.1:1/ws --token import-tok-123456 --executor-id 30002 --provider qoder --model qfmodel"
  "NO3模板示例|npx -y autowonder@<runtimeVersion> connect --ws-url ws://<内网地址>/ws --token <t>"
)
`

func writeImportScript(t *testing.T, dir string) string {
	t.Helper()
	f := filepath.Join(dir, "executor-manager.sh")
	if err := os.WriteFile(f, []byte(importScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestImport预览确认与落盘(t *testing.T) {
	e := newCLIEnv(t)
	f := writeImportScript(t, t.TempDir())

	// 拒绝 → 不导入
	e.setAnswers("n")
	if code := e.run(false, "import", "--file="+f); code != exitcode.Success {
		t.Fatalf("拒绝应取消成功：%d %s", code, e.output())
	}
	onDisk, _ := config.Load(e.p.ConfigFile)
	if len(onDisk.Executors) != 0 {
		t.Fatal("取消后不应有导入")
	}

	// 确认 → 导入 2 条（中文名改名为 exec-30001；占位符行跳过）
	e.setAnswers("y")
	if code := e.run(false, "import", "--file="+f); code != exitcode.Success {
		t.Fatalf("导入失败：%d %s", code, e.output())
	}
	out := e.output()
	for _, want := range []string{
		"1 条可导入、1 条需改名、1 条有问题",
		"NO1需求澄清 → exec-30001",
		"ascii-exec",
		"占位符", // 坏行报告
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("预览应含 %q：\n%s", want, out)
		}
	}
	onDisk, _ = config.Load(e.p.ConfigFile)
	if onDisk.FindExecutor("exec-30001") == nil || onDisk.FindExecutor("ascii-exec") == nil {
		t.Fatalf("导入应落盘 2 条（改名 + 原名），实际：%+v", onDisk.Executors)
	}
	if len(onDisk.Executors) != 2 {
		t.Fatalf("占位符行不应导入，实际 %d 条", len(onDisk.Executors))
	}
}

func TestImportWithLogs归档(t *testing.T) {
	e := newCLIEnv(t)
	dir := t.TempDir()
	f := writeImportScript(t, dir)
	// 预置旧布局日志：~/.autowonder-executor/<旧名>.log（home 已注入临时目录）
	oldDir := filepath.Join(e.home, ".autowonder-executor")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "NO1需求澄清.log"), []byte("旧日志行A\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e.setAnswers("y")
	if code := e.run(false, "import", "--file="+f, "--with-logs"); code != exitcode.Success {
		t.Fatalf("导入失败：%d %s", code, e.output())
	}
	// 日志出现在新位置（按改名后的新名）
	newLog := filepath.Join(e.p.ExecutorsLogDir, "exec-30001.log")
	data, err := os.ReadFile(newLog)
	if err != nil {
		t.Fatalf("归档日志应出现在新位置 %s：%v", newLog, err)
	}
	if !strings.Contains(string(data), "旧日志行A") {
		t.Fatalf("归档内容应一致：%s", data)
	}
	if !strings.Contains(e.output(), "日志归档完成：拷贝 1 条") {
		t.Fatalf("归档汇总应报 1 条拷贝（ascii-exec 无旧日志跳过）：%s", e.output())
	}
}

func TestImportJSON汇总(t *testing.T) {
	e := newCLIEnv(t)
	f := writeImportScript(t, t.TempDir())
	if code := e.run(true, "import", "--file="+f); code != exitcode.Usage {
		t.Fatalf("--json 无 --yes 应要求确认（用法错），实际 %d", code)
	}
	if code := e.run(true, "import", "--file="+f, "--yes"); code != exitcode.Success {
		t.Fatalf("--json 导入失败：%d %s", code, e.output())
	}
	var m struct {
		Script  string `json:"script"`
		Results []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(e.output()), &m); err != nil {
		t.Fatalf("非法 JSON：%v\n%s", err, e.output())
	}
	if len(m.Results) != 2 {
		t.Fatalf("应 2 条结果：%+v", m.Results)
	}
	byName := map[string]string{}
	for _, r := range m.Results {
		byName[r.Name+"|"+r.Status] = r.Detail
	}
	if _, ok := byName["exec-30001|renamed"]; !ok {
		t.Fatalf("中文名应为 renamed：%+v", m.Results)
	}
	if _, ok := byName["ascii-exec|imported"]; !ok {
		t.Fatalf("ASCII 名应为 imported：%+v", m.Results)
	}
}

func TestImport找不到脚本(t *testing.T) {
	e := newCLIEnv(t)
	// 工作目录与注入家目录都没有 executor-manager.sh
	// 用法错误文案走 stderr（writeError 统一），此处断言退出码
	if code := e.run(false, "import"); code != exitcode.Usage {
		t.Fatalf("找不到脚本应 2（用法指引），实际 %d", code)
	}
}
