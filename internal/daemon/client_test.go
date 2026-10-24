//go:build !windows

// client_test.go：CLI 惰性拉起 daemon（T06-F6 验收）。真实二进制集成：
// 杀掉 daemon → 跑 awem reload → daemon 被自动拉起且命令成功；
// 拉起失败（配置坏）→ 退出码 5 + 中文指引。
package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"awem/internal/config"
	"awem/internal/exitcode"
)

// runCLI 前台跑一条 CLI 命令，返回输出与退出码。
func runCLI(t *testing.T, bin, cfgDir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "AWEM_CONFIG_DIR="+cfgDir)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("运行 CLI 失败：%v", err)
		}
	}
	return string(out), code
}

// killDaemon 读 daemon.json 杀掉 daemon（集成清理/场景构造）。
func killDaemon(t *testing.T, cfgDir string, sig syscall.Signal) {
	t.Helper()
	p := config.At(cfgDir)
	info, alive, err := config.ReadDaemonInfo(p)
	if err != nil || !alive {
		return
	}
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return
	}
	_ = proc.Signal(sig)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, alive, _ := config.ReadDaemonInfo(p); !alive {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("daemon（pid %d）未随信号 %v 退出", info.PID, sig)
}

// TestConnect拉起失败退出码5 注入失败拉起 → 错误可映射退出码 5 且含指引。
func TestConnect拉起失败退出码5(t *testing.T) {
	dir := t.TempDir()
	_, err := Connect(context.Background(), ClientOptions{
		Paths:  config.At(dir),
		Launch: func() error { return os.ErrPermission },
	})
	if err == nil {
		t.Fatal("拉起失败应返回错误")
	}
	if exitcode.Code(err) != exitcode.DaemonNotRunning {
		t.Fatalf("错误应映射退出码 5，实际 %d（%v）", exitcode.Code(err), err)
	}
	for _, want := range []string{"awem daemon", "service install"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("错误信息应含指引 %q：%v", want, err)
		}
	}
}

// TestCLI惰性拉起daemon 集成：无 daemon → awem reload → 自动拉起且成功；
// 杀掉 → 再跑 → 再拉起；坏配置 → 退出码 5。
func TestCLI惰性拉起daemon(t *testing.T) {
	bin := buildAwem(t)
	cfgDir := t.TempDir()
	p := config.At(cfgDir)
	writeTestConfig(t, cfgDir) // port=0 自动挑

	t.Cleanup(func() { killDaemon(t, cfgDir, syscall.SIGTERM) })

	// 1) 无 daemon（daemon.json 不存在）→ reload 自动拉起且成功
	out, code := runCLI(t, bin, cfgDir, "reload")
	if code != 0 {
		t.Fatalf("首次 reload 应成功（自动拉起），退出码 %d，输出：%s", code, out)
	}
	info, alive, _ := config.ReadDaemonInfo(p)
	if !alive || info.PID <= 0 {
		t.Fatalf("reload 后 daemon 应被自动拉起（daemon.json 报活），实际 %+v alive=%v", info, alive)
	}

	// 2) 强杀 daemon（daemon.json 残留 stale）→ reload 再次自动拉起
	killDaemon(t, cfgDir, syscall.SIGKILL)
	out, code = runCLI(t, bin, cfgDir, "reload")
	if code != 0 {
		t.Fatalf("强杀后 reload 应再次自动拉起成功，退出码 %d，输出：%s", code, out)
	}
	info2, alive2, _ := config.ReadDaemonInfo(p)
	if !alive2 || info2.PID == info.PID {
		t.Fatalf("应拉起新 daemon（新 pid），旧 %+v 新 %+v", info, info2)
	}

	// 3) reload 后 API 真实生效：手改配置 → reload → 后续命令看到
	onDisk, _ := config.Load(p.ConfigFile)
	onDisk.Executors = []config.Executor{}
	if err := config.Save(p.ConfigFile, onDisk); err != nil {
		t.Fatal(err)
	}
	out, code = runCLI(t, bin, cfgDir, "reload", "--json")
	if code != 0 || !strings.Contains(out, `"reloaded":true`) {
		t.Fatalf("reload --json 应返回 reloaded:true，退出码 %d，输出：%s", code, out)
	}

	// 4) 拉起失败：坏配置（daemon 启动即拒）→ 退出码 5 + 中文指引
	killDaemon(t, cfgDir, syscall.SIGTERM)
	if err := os.WriteFile(p.ConfigFile, []byte("[server]\nbind = \"0.0.0.0\"\nport = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code = runCLI(t, bin, cfgDir, "reload")
	if code != exitcode.DaemonNotRunning {
		t.Fatalf("拉起失败应退出码 5，实际 %d，输出：%s", code, out)
	}
	if !strings.Contains(out, "awem daemon") {
		t.Fatalf("错误输出应给下一步指引：%s", out)
	}
	// daemon.stderr 留有排障线索
	stderrData, err := os.ReadFile(filepath.Join(p.LogsDir, "daemon.stderr"))
	if err != nil || !strings.Contains(string(stderrData), "127.0.0.1") {
		t.Fatalf("daemon.stderr 应记录启动失败原因，err=%v 内容：%s", err, stderrData)
	}
}
