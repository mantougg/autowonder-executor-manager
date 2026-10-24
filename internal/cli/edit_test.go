// edit_test.go：awem edit 验收（T07-F5）。
package cli

import (
	"strings"
	"testing"

	"awem/internal/config"
	"awem/internal/exitcode"
)

func TestEdit单字段与命令重建(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "ed1", Command: validCommand})
	if code := e.run(false, "edit", "ed1", "--model", "dfmodel", "--reasoning-effort", "low"); code != exitcode.Success {
		t.Fatalf("edit 失败：%d %s", code, e.output())
	}
	out := e.output()
	if !strings.Contains(out, "已更新 ed1") || !strings.Contains(out, "--model dfmodel") {
		t.Fatalf("edit 输出不合预期：%s", out)
	}
	// 落盘：命令段变化、passthrough（--token-aware-enable）与原 token 不丢
	onDisk, _ := config.Load(e.p.ConfigFile)
	cmd := onDisk.FindExecutor("ed1").Command
	if !strings.Contains(cmd, "--model dfmodel") || !strings.Contains(cmd, "--reasoning-effort low") {
		t.Fatalf("命令未按新字段重建：%s", cmd)
	}
	if !strings.Contains(cmd, "--token-aware-enable") {
		t.Fatalf("未知/布尔 flag 丢失：%s", cmd)
	}
	if !strings.Contains(cmd, "secret-token-abcdef123456") {
		t.Fatalf("未提交的 token 不应丢失：%s", cmd)
	}
}

func TestEdit布尔与autostart(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "ed2", Command: validCommand})
	if code := e.run(false, "edit", "ed2", "--debug", "--no-autostart"); code != exitcode.Success {
		t.Fatalf("edit 失败：%d %s", code, e.output())
	}
	onDisk, _ := config.Load(e.p.ConfigFile)
	ex := onDisk.FindExecutor("ed2")
	if !strings.Contains(ex.Command, "--debug") {
		t.Fatalf("--debug 未落命令：%s", ex.Command)
	}
	if ex.Autostart {
		t.Fatal("--no-autostart 未生效")
	}
	// =false 形态移除 debug（布尔开关来回切）
	if code := e.run(false, "edit", "ed2", "--debug=false"); code != exitcode.Success {
		t.Fatalf("edit --debug=false 失败：%d %s", code, e.output())
	}
	onDisk, _ = config.Load(e.p.ConfigFile)
	if strings.Contains(onDisk.FindExecutor("ed2").Command, "--debug") {
		t.Fatalf("--debug=false 应移除 flag：%s", onDisk.FindExecutor("ed2").Command)
	}
}

func TestEdit整条替换与坏命令(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "ed3", Command: validCommand})
	// 整条替换（换版本与模型）
	newCmd := strings.Replace(strings.Replace(validCommand, "qfmodel", "other-m", 1), "0.3.22", "0.4.0", 1)
	if code := e.run(false, "edit", "ed3", "--command", newCmd); code != exitcode.Success {
		t.Fatalf("整条替换失败：%d %s", code, e.output())
	}
	onDisk, _ := config.Load(e.p.ConfigFile)
	if !strings.Contains(onDisk.FindExecutor("ed3").Command, "0.4.0") {
		t.Fatal("整条替换未生效")
	}
	// 坏命令 → 2 且原配置不变
	before := onDisk.FindExecutor("ed3").Command
	if code := e.run(false, "edit", "ed3", "--command", "not-a-command"); code != exitcode.Usage {
		t.Fatalf("坏命令应退出码 2，实际 %d：%s", code, e.output())
	}
	onDisk, _ = config.Load(e.p.ConfigFile)
	if onDisk.FindExecutor("ed3").Command != before {
		t.Fatal("坏命令不应改动原配置")
	}
	// 未找到 → 3
	if code := e.run(false, "edit", "nope", "--model", "x"); code != exitcode.NotFound {
		t.Fatalf("不存在应 3，实际 %d", code)
	}
	// 无修改项 → 2
	if code := e.run(false, "edit", "ed3"); code != exitcode.Usage {
		t.Fatalf("无修改项应 2，实际 %d", code)
	}
}

func TestEdit运行中提示restart(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "ed4", Command: validCommand})
	if err := e.sup.StartExecutor("ed4"); err != nil {
		t.Fatal(err)
	}
	if code := e.run(false, "edit", "ed4", "--model", "dfmodel"); code != exitcode.Success {
		t.Fatalf("edit 失败：%d %s", code, e.output())
	}
	if !strings.Contains(e.output(), "下次 restart 后生效") {
		t.Fatalf("运行中 edit 应提示需 restart：%s", e.output())
	}
}
