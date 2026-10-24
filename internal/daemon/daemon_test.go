//go:build !windows

// daemon_test.go：daemon 生命周期集成冒烟（T06-F1 验收要点）。
// 用真实编译的二进制跑 `awem daemon`（AWEM_CONFIG_DIR 隔离到临时目录）：
//   1) 第二个实例拒绝启动且报错含已占端口；
//   2) 强杀第一个后（锁随进程释放）能重新启动，daemon.json 更新为新 pid；
//   3) 优雅退出（SIGTERM）后 daemon.json 清理。
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"awem/internal/config"
)

const testPort = 18765 // 预置到 config.toml（F1 daemon 尚未监听端口，仅验证报错回显）

// buildAwem 编译真实二进制（go build 有缓存，重复编译开销很小）。
func buildAwem(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "awem-bin")
	out, err := exec.Command("go", "build", "-o", bin, "awem/cmd/awem").CombinedOutput()
	if err != nil {
		t.Fatalf("编译 awem 失败：%v\n%s", err, out)
	}
	return bin
}

// startDaemon 后台启动一个 daemon 实例并等到 daemon.json 报活。
func startDaemon(t *testing.T, bin, cfgDir string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin, "daemon")
	cmd.Env = append(os.Environ(), "AWEM_CONFIG_DIR="+cfgDir)
	cmd.Stderr = os.Stderr // 透传诊断信息，失败时可见
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 daemon 失败：%v", err)
	}
	p := config.At(cfgDir)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, alive, _ := config.ReadDaemonInfo(p); alive && info.PID == cmd.Process.Pid {
			return cmd
		}
		// 进程可能已退出（如配置错误）——及早暴露
		if cmd.ProcessState != nil {
			t.Fatalf("daemon 启动后立即退出：%v", cmd.ProcessState)
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	t.Fatal("等待 daemon 就绪超时（daemon.json 未报活）")
	return nil
}

// runDaemonBlocked 前台跑一个应失败的 daemon 实例，返回输出与退出码。
func runDaemonBlocked(t *testing.T, bin, cfgDir string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, "daemon")
	cmd.Env = append(os.Environ(), "AWEM_CONFIG_DIR="+cfgDir)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("运行 daemon 失败：%v", err)
		}
	}
	return string(out), code
}

// writeTestConfig 预置一份最小有效配置（固定端口便于断言报错回显）。
func writeTestConfig(t *testing.T, cfgDir string) {
	t.Helper()
	c := config.Default()
	c.Server.Port = testPort
	if err := config.Save(filepath.Join(cfgDir, "config.toml"), c); err != nil {
		t.Fatal(err)
	}
}

func TestDaemon单实例与重启(t *testing.T) {
	bin := buildAwem(t)
	cfgDir := t.TempDir()
	writeTestConfig(t, cfgDir)
	p := config.At(cfgDir)

	// 1) 第一个实例正常运行
	d1 := startDaemon(t, bin, cfgDir)
	defer func() { _ = d1.Process.Kill() }()

	// 2) 第二个实例拒绝启动，报错含已占端口与 pid
	out, code := runDaemonBlocked(t, bin, cfgDir)
	if code == 0 {
		t.Fatalf("第二个实例应失败退出，实际成功。输出：%s", out)
	}
	for _, want := range []string{strconv.Itoa(testPort), strconv.Itoa(d1.Process.Pid)} {
		if !strings.Contains(out, want) {
			t.Fatalf("报错应含 %q，实际输出：%s", want, out)
		}
	}

	// 3) 强杀第一个：锁随进程释放，daemon.json 残留为 stale
	if err := d1.Process.Kill(); err != nil {
		t.Fatalf("强杀第一个 daemon 失败：%v", err)
	}
	_ = d1.Wait()
	if _, alive, _ := config.ReadDaemonInfo(p); alive {
		t.Fatal("强杀后 daemon.json 应为 stale（pid 不存活）")
	}

	// 4) 第三个实例能重新启动，daemon.json 更新为新 pid
	d3 := startDaemon(t, bin, cfgDir)
	info, alive, _ := config.ReadDaemonInfo(p)
	if !alive || info.PID != d3.Process.Pid || info.Port != testPort {
		t.Fatalf("重启后 daemon.json 应为新实例（pid %d port %d），实际 %+v alive=%v",
			d3.Process.Pid, testPort, info, alive)
	}

	// 5) 优雅退出（SIGTERM）：daemon 清理并退出
	if err := d3.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("发送 SIGTERM 失败：%v", err)
	}
	done := make(chan error, 1)
	go func() { done <- d3.Wait() }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = d3.Process.Kill()
		t.Fatal("daemon 未随 SIGTERM 退出")
	}
	if _, err := os.Stat(p.DaemonFile); !os.IsNotExist(err) {
		t.Fatalf("优雅退出后 daemon.json 应被清理，实际 err=%v", err)
	}
}

// TestDaemon启动时配置无效拒绝运行 启动即拒（区别于热加载的"保留旧配置"）。
func TestDaemon启动时配置无效拒绝运行(t *testing.T) {
	bin := buildAwem(t)
	cfgDir := t.TempDir()
	_ = os.MkdirAll(cfgDir, 0o755)
	// bind 非 127.0.0.1：校验失败
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"),
		[]byte("[server]\nbind = \"0.0.0.0\"\nport = 1\n\n[log]\nmax_size_mb = 50\nretention_days = 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := runDaemonBlocked(t, bin, cfgDir)
	if code == 0 {
		t.Fatalf("配置无效时 daemon 应拒绝启动，实际成功。输出：%s", out)
	}
	if !strings.Contains(out, "配置必须有效") {
		t.Fatalf("报错应说明启动时配置校验失败，实际：%s", out)
	}
	// daemon.json 不应残留
	if _, err := os.Stat(config.At(cfgDir).DaemonFile); !os.IsNotExist(err) {
		t.Fatalf("启动失败不应残留 daemon.json，实际 err=%v", err)
	}
}

// TestDaemon事件日志记录启停 daemon 启停在 events.log 留痕（F1 装配验证）。
func TestDaemon事件日志记录启停(t *testing.T) {
	bin := buildAwem(t)
	cfgDir := t.TempDir()
	writeTestConfig(t, cfgDir)
	p := config.At(cfgDir)

	d := startDaemon(t, bin, cfgDir)
	if err := d.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM 失败：%v", err)
	}
	_ = d.Wait()

	data, err := os.ReadFile(p.EventsLog)
	if err != nil {
		t.Fatalf("读取事件日志失败：%v", err)
	}
	s := string(data)
	for _, want := range []string{"daemon_start", "daemon_stop"} {
		if !strings.Contains(s, want) {
			t.Fatalf("事件日志应含 %s，实际：\n%s", want, s)
		}
	}
	fmt.Printf("事件日志样例：\n%s", s)
}
