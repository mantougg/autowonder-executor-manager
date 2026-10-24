// doctor_test.go：awem doctor 验收（T07-F9）。
package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"awem/internal/config"
	"awem/internal/exitcode"
)

// withDoctorEnv 临时替换体检环境（测试后还原）。
func withDoctorEnv(t *testing.T, dd doctorDeps) {
	t.Helper()
	old := doctorEnv
	doctorEnv = dd
	t.Cleanup(func() { doctorEnv = old })
}

func TestDoctor本机全项输出(t *testing.T) {
	e := newCLIEnv(t) // 无执行器 → provider CLI 走跳过分支
	if code := e.run(false, "doctor"); code != exitcode.Success {
		t.Fatalf("本机体检应通过（Node/npx/磁盘），实际 %d：%s", code, exitcodeOrOutput(e))
	}
	out := e.output()
	for _, want := range []string{"Node.js ≥ 20", "npx", "provider CLI", "磁盘剩余", "平台连通"} {
		if !strings.Contains(out, want) {
			t.Fatalf("应含检查项 %q：\n%s", want, out)
		}
	}
	if !strings.Contains(out, "环境体检全部通过") {
		t.Fatalf("结尾应报全部通过：%s", out)
	}
	// 平台连通占位：— 且不误报失败
	if !strings.Contains(out, "— 平台连通：未配置") {
		t.Fatalf("平台连通应为跳过占位：%s", out)
	}
}

func TestDoctor缺失npx退出码1(t *testing.T) {
	e := newCLIEnv(t)
	withDoctorEnv(t, doctorDeps{
		lookupPath: func(name string) (string, error) { return "", errors.New("not found") },
		runVersion: func(cmd string, args ...string) (string, error) { return "", errors.New("no exec") },
		statFree:   func(string) (uint64, error) { return 99 << 30, nil },
	})
	if code := e.run(false, "doctor"); code != exitcode.General {
		t.Fatalf("缺失 node/npx 应退出码 1，实际 %d", code)
	}
	out := e.output()
	if !strings.Contains(out, "❌") || !strings.Contains(out, "建议：") {
		t.Fatalf("应有 ❌ 与修复建议：%s", out)
	}
}

func TestDoctorNode版本过低(t *testing.T) {
	e := newCLIEnv(t)
	withDoctorEnv(t, doctorDeps{
		lookupPath: func(string) (string, error) { return "/fake/node", nil },
		runVersion: func(cmd string, args ...string) (string, error) {
			if cmd == "node" {
				return "v16.20.0", nil
			}
			return "", errors.New("no exec")
		},
		statFree: func(string) (uint64, error) { return 99 << 30, nil },
	})
	if code := e.run(false, "doctor"); code != exitcode.General {
		t.Fatalf("Node 16 应不通过退出 1，实际 %d", code)
	}
	if !strings.Contains(e.output(), "版本过低（v16.20.0）") {
		t.Fatalf("应报版本过低：%s", e.output())
	}
}

func TestDoctor按Provider检查CLI(t *testing.T) {
	// 配置了 qoder 执行器 → 检查 qoder CLI（注入缺失）
	e := newCLIEnv(t, config.Executor{Name: "dq", Command: validCommand})
	withDoctorEnv(t, doctorDeps{
		lookupPath: func(name string) (string, error) {
			if name == "qoder" {
				return "", errors.New("not found")
			}
			return "/fake/" + name, nil
		},
		runVersion: func(cmd string, args ...string) (string, error) { return "v22.0.0", nil },
		statFree:   func(string) (uint64, error) { return 99 << 30, nil },
	})
	if code := e.run(false, "doctor"); code != exitcode.General {
		t.Fatalf("缺 qoder CLI 应退出 1，实际 %d：%s", code, e.output())
	}
	if !strings.Contains(e.output(), "缺少 qoder") {
		t.Fatalf("应报缺少 qoder：%s", e.output())
	}
}

func TestDoctorJSON输出(t *testing.T) {
	e := newCLIEnv(t)
	if code := e.run(true, "doctor"); code != exitcode.Success {
		t.Fatalf("doctor --json 失败：%d", code)
	}
	// 人读行也会输出（doctor 的逐项 ✅/— 与 --json 摘要并存；JSON 是最后一行）
	lines := strings.Split(strings.TrimSpace(e.output()), "\n")
	var m []checkItem
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &m); err != nil {
		t.Fatalf("最后一行应为 JSON 数组：%v\n%s", err, e.output())
	}
	if len(m) != 5 {
		t.Fatalf("应 5 项检查：%d", len(m))
	}
	// 平台连通恒为跳过占位且不误报（OK 字段无关紧要，Skipped=true）
	foundPlatformSkip := false
	for _, it := range m {
		if it.Skipped && strings.Contains(it.Name, "平台连通") {
			foundPlatformSkip = true
		}
	}
	if !foundPlatformSkip {
		t.Fatalf("平台连通应为跳过占位：%+v", m)
	}
}

// exitcodeOrOutput 调试辅助。
func exitcodeOrOutput(e *cliEnv) string { return e.output() }
