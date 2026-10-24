//go:build !windows

// listen_test.go：端口绑定与自动写回（T06-F2 验收要点）。
package daemon

import (
	"net"
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

// TestListen端口零自动写回与roundtrip port=0 → 实际端口写回，
// 其余字段与格式不变（Load 后再序列化与磁盘逐字节一致）。
func TestListen端口零自动写回与roundtrip(t *testing.T) {
	dir := t.TempDir()
	p := config.At(dir)
	enabled := false
	c := config.Default()
	c.Server.Port = 0
	c.Platform = &config.PlatformConfig{BaseURL: "http://10.0.0.1:7001", MCPToken: "tok-1234567890", PollInterval: config.DefaultPollInterval}
	c.Executors = []config.Executor{{
		Name:    "e1",
		Command: "npx -y autowonder@0.3.22 connect --ws-url ws://h/ws --token tk --executor-id 42 --provider qoder --model qfmodel",
		Restart: config.RestartPolicy{Enabled: config.BoolPtr(true), Backoff: "exponential"},
	}}
	_ = enabled
	if err := config.Save(p.ConfigFile, c); err != nil {
		t.Fatal(err)
	}

	ln, cfg, err := listenWithWriteBack(p, c, nil)
	if err != nil {
		t.Fatalf("监听失败：%v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if port <= 0 {
		t.Fatalf("实际端口应 >0，实际 %d", port)
	}
	if cfg.Server.Port != port {
		t.Fatalf("返回配置 port 应写回为 %d，实际 %d", port, cfg.Server.Port)
	}

	// 写回断言：磁盘 port == 实际端口；其余字段保留
	onDisk, err := config.Load(p.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.Server.Port != port {
		t.Fatalf("config.toml 应写回 port=%d，实际 %d", port, onDisk.Server.Port)
	}
	if len(onDisk.Executors) != 1 || onDisk.Executors[0].Name != "e1" {
		t.Fatalf("写回不应丢失 executor 条目：%+v", onDisk.Executors)
	}
	if onDisk.Platform == nil || onDisk.Platform.BaseURL != "http://10.0.0.1:7001" {
		t.Fatalf("写回不应丢失 platform 段：%+v", onDisk.Platform)
	}
	// 格式稳定：读入再序列化 == 磁盘内容（roundtrip 幂等，注释头保留）
	raw, _ := os.ReadFile(p.ConfigFile)
	if string(raw) != config.MarshalTOML(onDisk) {
		t.Fatalf("写回后格式不稳定：\n%s\n--- 期望 ---\n%s", raw, config.MarshalTOML(onDisk))
	}
	// 监听地址必须是回环
	host, _, err := net.SplitHostPort(ln.Addr().String())
	if err != nil || host != "127.0.0.1" {
		t.Fatalf("监听地址应为 127.0.0.1，实际 %s", ln.Addr())
	}
}

// TestListen固定端口不写回 port 非 0 时不改磁盘。
func TestListen固定端口不写回(t *testing.T) {
	dir := t.TempDir()
	p := config.At(dir)
	c := config.Default()
	c.Server.Port = 0 // 先占一个空闲端口当"固定端口"
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	c.Server.Port = port
	if err := config.Save(p.ConfigFile, c); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(p.ConfigFile)

	ln2, cfg, err := listenWithWriteBack(p, c, nil)
	if err != nil {
		t.Fatalf("固定端口监听失败：%v", err)
	}
	defer ln2.Close()
	after, _ := os.ReadFile(p.ConfigFile)
	if string(before) != string(after) {
		t.Fatal("固定端口启动不应写回配置")
	}
	if cfg.Server.Port != port {
		t.Fatalf("配置端口应保持 %d，实际 %d", port, cfg.Server.Port)
	}
}

// TestDaemon自动端口写回与二次启动 集成：port=0 起 daemon →
// config.toml 与 daemon.json 均为实际端口 → 重启用该端口；写回引发的
// 文件监视重载被抑制（无 config_changed 事件）。
func TestDaemon自动端口写回与二次启动(t *testing.T) {
	bin := buildAwem(t)
	cfgDir := t.TempDir()
	c := config.Default()
	c.Server.Port = 0
	if err := config.Save(filepath.Join(cfgDir, "config.toml"), c); err != nil {
		t.Fatal(err)
	}
	p := config.At(cfgDir)

	d1 := startDaemon(t, bin, cfgDir)
	info, alive, _ := config.ReadDaemonInfo(p)
	if !alive || info.Port <= 0 {
		t.Fatalf("daemon.json 应记录实际端口，实际 %+v alive=%v", info, alive)
	}
	port := info.Port
	// config.toml 已写回该端口
	onDisk, err := config.Load(p.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.Server.Port != port {
		t.Fatalf("config.toml 应写回 port=%d，实际 %d", port, onDisk.Server.Port)
	}
	// 写回触发的文件监视重载应被抑制：等去抖窗口过后无 config_changed 事件
	time.Sleep(600 * time.Millisecond)
	evData, _ := os.ReadFile(p.EventsLog)
	if strings.Contains(string(evData), "config_changed") {
		t.Fatalf("写回引发的等价重载应被抑制，事件日志：\n%s", evData)
	}

	// 优雅退出 → 重启：使用写回的端口
	if err := d1.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = d1.Wait()
	d2 := startDaemon(t, bin, cfgDir)
	defer func() { _ = d2.Process.Kill() }()
	info2, alive2, _ := config.ReadDaemonInfo(p)
	if !alive2 || info2.Port != port {
		t.Fatalf("二次启动应复用写回端口 %d，实际 %+v alive=%v", port, info2, alive2)
	}
	// 端口确实在监听（连接即成功）
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		t.Fatalf("写回端口应可连接：%v", err)
	}
	conn.Close()
	_ = exec.Command // 保持导入（跨平台防御）
}
