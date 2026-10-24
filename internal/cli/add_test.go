// add_test.go：awem add 验收（T07-F1）。
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

func TestAdd正常添加与人读摘要(t *testing.T) {
	e := newCLIEnv(t)
	code := e.run(false, "add", validCommand)
	if code != exitcode.Success {
		t.Fatalf("add 应成功，退出码 %d，输出：%s", code, e.output())
	}
	out := e.output()
	for _, want := range []string{"已添加执行器 exec-10086", "执行器 ID：10086", "provider：qoder", "autowonder@0.3.22"} {
		if !strings.Contains(out, want) {
			t.Fatalf("摘要应含 %q，实际：\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret-token-abcdef123456") {
		t.Fatal("摘要不得回显明文 token")
	}
	// 落盘验证（API 链路）
	onDisk, _ := config.Load(e.p.ConfigFile)
	if onDisk.FindExecutor("exec-10086") == nil {
		t.Fatal("add 后 config.toml 应含 exec-10086")
	}
}

func TestAdd默认名与自定义名(t *testing.T) {
	e := newCLIEnv(t)
	if code := e.run(false, "add", validCommand, "--name", "my-exec"); code != 0 {
		t.Fatalf("自定义名失败：%d %s", code, e.output())
	}
	if !strings.Contains(e.output(), "my-exec") {
		t.Fatalf("应使用自定义名：%s", e.output())
	}
	// 同名再添加（不同命令）→ 已存在 4
	other := strings.Replace(validCommand, "10086", "10087", 1)
	code := e.run(false, "add", other, "--name", "my-exec")
	if code != exitcode.AlreadyExists {
		t.Fatalf("重名应退出码 4，实际 %d：%s", code, e.output())
	}
}

func TestAdd坏命令退出码2(t *testing.T) {
	e := newCLIEnv(t)
	bad := "npx -y autowonder@0.3.22 connect --ws-url ws://127.0.0.1:1/ws --executor-id abc --provider qoder"
	code := e.run(false, "add", bad)
	if code != exitcode.Usage {
		t.Fatalf("坏命令应退出码 2（本地解析拦截），实际 %d：%s", code, e.output())
	}
	// 不支持的命令形态
	code = e.run(false, "add", "docker run something")
	if code != exitcode.Usage {
		t.Fatalf("非 npx 命令应退出码 2，实际 %d", code)
	}
	// 缺命令
	if code = e.run(false, "add"); code != exitcode.Usage {
		t.Fatalf("缺命令应退出码 2，实际 %d", code)
	}
}

func TestAdd文件输入与参数等价(t *testing.T) {
	e := newCLIEnv(t)
	f := filepath.Join(t.TempDir(), "cmd.txt")
	if err := os.WriteFile(f, []byte(validCommand), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := e.run(false, "add", "--file", f); code != 0 {
		t.Fatalf("--file 添加失败：%d %s", code, e.output())
	}
	onDisk, _ := config.Load(e.p.ConfigFile)
	if onDisk.FindExecutor("exec-10086") == nil {
		t.Fatal("--file 输入未落盘")
	}
}

func TestAddJSON输出掩码(t *testing.T) {
	e := newCLIEnv(t)
	code := e.run(true, "add", validCommand, "--start")
	if code != 0 {
		t.Fatalf("add --json 失败：%d %s", code, e.output())
	}
	var m struct {
		Executor struct {
			Name   string `json:"name"`
			Parsed *struct {
				Token string `json:"token"`
			} `json:"parsed"`
			Local struct {
				State string `json:"state"`
			} `json:"local"`
		} `json:"executor"`
	}
	if err := json.Unmarshal([]byte(e.output()), &m); err != nil {
		t.Fatalf("输出不是合法 JSON：%v\n%s", err, e.output())
	}
	if m.Executor.Name != "exec-10086" {
		t.Fatalf("name 应 exec-10086：%v", m.Executor.Name)
	}
	if m.Executor.Parsed == nil || m.Executor.Parsed.Token == "" || !strings.Contains(m.Executor.Parsed.Token, "***") {
		t.Fatalf("--json token 应掩码：%+v", m.Executor.Parsed)
	}
	if m.Executor.Local.State != "running" {
		t.Fatalf("--start 后应 running（真子进程）：%v", m.Executor.Local.State)
	}
}

func TestAdd开关落盘(t *testing.T) {
	e := newCLIEnv(t)
	cmd := strings.Replace(validCommand, "10086", "20001", 1)
	if code := e.run(false, "add", cmd, "--autostart", "--no-restart"); code != 0 {
		t.Fatalf("带开关添加失败：%d %s", code, e.output())
	}
	onDisk, _ := config.Load(e.p.ConfigFile)
	ex := onDisk.FindExecutor("exec-20001")
	if ex == nil {
		t.Fatal("未落盘")
	}
	if !ex.Autostart {
		t.Fatal("--autostart 未生效")
	}
	if ex.Restart.Enabled == nil || *ex.Restart.Enabled {
		t.Fatal("--no-restart 未生效")
	}
	// --debug 追加布尔 flag（版本号不同避免重名）
	cmd2 := strings.Replace(strings.Replace(validCommand, "10086", "20002", 1), "0.3.22", "0.3.23", 1)
	if code := e.run(false, "add", cmd2, "--debug"); code != 0 {
		t.Fatalf("--debug 添加失败：%d %s", code, e.output())
	}
	onDisk, _ = config.Load(e.p.ConfigFile)
	if !strings.Contains(onDisk.FindExecutor("exec-20002").Command, "--debug") {
		t.Fatal("--debug 未附加到命令")
	}
}
