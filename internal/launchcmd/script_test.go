package launchcmd

import (
	"os"
	"strings"
	"testing"
)

// TestParseScript真实模板Fixture 用仓库 testdata/ 的真实模板文件做单测。
func TestParseScript真实模板Fixture(t *testing.T) {
	data, err := os.ReadFile("testdata/executor-manager.sh")
	if err != nil {
		t.Fatalf("读取 fixture 失败：%v", err)
	}
	items, err := ParseScript(string(data))
	if err != nil {
		t.Fatalf("解析真实模板失败：%v", err)
	}
	if len(items) != 1 {
		t.Fatalf("模板 EXECUTORS 应恰有 1 条示例条目，实际 %d", len(items))
	}
	it := items[0]
	if it.Name != "NO1示例数字人" {
		t.Fatalf("名称 = %q，期望模板示例名", it.Name)
	}
	if it.ParseErr == nil || !strings.Contains(it.ParseErr.Error(), "占位符") {
		t.Fatalf("含 <runtimeVersion> 的示例行应标记占位符错误，实际：%v", it.ParseErr)
	}
	if it.NeedsRename {
		t.Fatal("命令本身有占位符错误时不走改名流程（ParseErr 优先）")
	}
}

// TestParseScript合成脚本 覆盖注释/空行/合法条目/中文名改名/坏条目。
func TestParseScript合成脚本(t *testing.T) {
	script := `#!/usr/bin/env bash
# 注释行应被跳过
EXECUTORS=(
  # 数组内注释也跳过

  "cli.build|npx -y autowonder@0.3.22 connect --ws-url ws://h/ws --token tk --executor-id 10086 --provider qoder --model qfmodel --token-aware-enable"
  "需求澄清|npx -y autowonder@0.3.22 connect --ws-url ws://h/ws --token tk2 --executor-id 10087 --provider qoder"
  '单引号条目|npx -y autowonder@0.1.0 connect --ws-url ws://x --token t --executor-id 20001 --provider codex'
  "没有分隔符的行"
  "坏命令|xecutor start --all"
)
`
	items, err := ParseScript(script)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(items) != 5 {
		t.Fatalf("应解析出 5 条，实际 %d：%+v", len(items), items)
	}

	// 1) 合法名 + 合法命令：直接可导入
	if items[0].Name != "cli.build" || items[0].NeedsRename || items[0].ParseErr != nil {
		t.Fatalf("合法条目不应有问题：%+v", items[0])
	}
	// 2) 中文名：命令合法 → 建议名 exec-<executor-id>
	if !items[1].NeedsRename || items[1].SuggestedName != "exec-10087" {
		t.Fatalf("中文名应建议改名 exec-10087：%+v", items[1])
	}
	if items[1].ParseErr != nil {
		t.Fatalf("命令合法时不应有 ParseErr：%v", items[1].ParseErr)
	}
	// 3) 单引号条目同样解析
	if items[2].Name != "单引号条目" || items[2].SuggestedName != "exec-20001" {
		t.Fatalf("单引号条目解析不符：%+v", items[2])
	}
	// 4) 无分隔符：报错不静默
	if items[3].ParseErr == nil || !strings.Contains(items[3].ParseErr.Error(), "分隔符") {
		t.Fatalf("无分隔符应报错：%+v", items[3])
	}
	// 5) 坏命令：报不支持的命令
	if items[4].ParseErr == nil || !strings.Contains(items[4].ParseErr.Error(), "不支持的命令") {
		t.Fatalf("坏命令应报错：%+v", items[4])
	}
}

// TestParseScript建议名去重 相同 executor-id 的两条中文名不撞名。
func TestParseScript建议名去重(t *testing.T) {
	script := `EXECUTORS=(
  "甲|npx -y autowonder@1.0.0 connect --ws-url ws://h --token t --executor-id 5 --provider qoder"
  "乙|npx -y autowonder@1.0.0 connect --ws-url ws://h --token t --executor-id 5 --provider qoder"
  "丙|npx -y autowonder@1.0.0 connect --ws-url ws://h --token t --provider qoder --executor-id 6"
)
`
	// 第三条 executor-id 在末尾：解析正常
	items, err := ParseScript(script)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].SuggestedName != "exec-5" || items[1].SuggestedName != "exec-5-2" {
		t.Fatalf("建议名应去重：%+v", items)
	}
	if items[2].SuggestedName != "exec-6" {
		t.Fatalf("第三条建议名 = %q，期望 exec-6", items[2].SuggestedName)
	}
}

// TestParseScript无数组报错。
func TestParseScript无数组报错(t *testing.T) {
	if _, err := ParseScript("echo hello"); err == nil {
		t.Fatal("无 EXECUTORS 数组应报错")
	}
}

// TestParseScript空数组。
func TestParseScript空数组(t *testing.T) {
	items, err := ParseScript("EXECUTORS=(\n)\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("空数组应返回 0 条，实际 %d", len(items))
	}
}
