package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sampleConfig 构造覆盖全部字段的样例配置。
func sampleConfig() Config {
	c := Default()
	c.Server = ServerConfig{Bind: "127.0.0.1", Port: 7800}
	c.Platform = &PlatformConfig{
		BaseURL:      "http://10.144.144.1:7001",
		MCPToken:     "tok-测试\"引号\"与\\反斜杠",
		PollInterval: 90 * time.Second,
	}
	c.Log = LogConfig{MaxSizeMB: 30, RetentionDays: 3}
	c.Executors = []Executor{
		{
			Name:      "no1-clarify",
			Command:   `npx -y autowonder@0.3.22 connect --ws-url ws://h/ws --token "t k" --executor-id 10086 --provider qoder --model qfmodel`,
			Autostart: true,
			Restart:   RestartPolicy{Enabled: BoolPtr(true), Backoff: "exponential"},
		},
		{
			Name:    "cli.build",
			Command: "npx -y autowonder@0.3.22 connect --ws-url wss://h2/ws --token t2 --executor-id 10087 --provider claude",
			Restart: RestartPolicy{Enabled: BoolPtr(false), Backoff: "exponential"},
		},
	}
	return c
}

// Test字段与设计5_2一一对应 断言默认值。
func Test字段与设计5_2一一对应(t *testing.T) {
	c := Default()
	if c.Server.Bind != "127.0.0.1" || c.Server.Port != 0 {
		t.Fatalf("server 默认值不符：%+v", c.Server)
	}
	if c.Log.MaxSizeMB != 50 || c.Log.RetentionDays != 7 {
		t.Fatalf("log 默认值不符：%+v", c.Log)
	}
	if c.Platform != nil {
		t.Fatal("platform 缺省应为 nil（联动禁用）")
	}
}

// Test读写Roundtrip 写入→读出→深比较相等（含中文/引号/反斜杠等特殊字符）。
func Test读写Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	want := sampleConfig()
	if err := Save(path, want); err != nil {
		t.Fatalf("Save 失败：%v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败：%v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip 不一致：\n got=%+v\nwant=%+v", got, want)
	}
}

// Test缺省段取默认值 platform 未配、restart 未配时不报错且取默认。
func Test缺省段取默认值(t *testing.T) {
	src := `
[server]
port = 7800

[[executor]]
name = "e1"
command = "npx -y autowonder@0.3.22 connect --ws-url ws://h --token t --executor-id 1 --provider qoder"
`
	c, err := LoadString(src)
	if err != nil {
		t.Fatalf("LoadString 失败：%v", err)
	}
	if c.Platform != nil {
		t.Fatal("platform 未配应为 nil")
	}
	if c.Server.Bind != "127.0.0.1" {
		t.Fatalf("bind 缺省应为 127.0.0.1，得到 %q", c.Server.Bind)
	}
	if c.Log.MaxSizeMB != 50 || c.Log.RetentionDays != 7 {
		t.Fatalf("log 缺省值不符：%+v", c.Log)
	}
	if len(c.Executors) != 1 {
		t.Fatalf("executors 数量 = %d，期望 1", len(c.Executors))
	}
	e := c.Executors[0]
	if e.Restart.Enabled == nil || !*e.Restart.Enabled {
		t.Fatalf("restart 未配置应默认开启，得到 %+v", e.Restart)
	}
	if e.Restart.Backoff != "exponential" {
		t.Fatalf("backoff 缺省应为 exponential，得到 %q", e.Restart.Backoff)
	}
	if e.Autostart {
		t.Fatal("autostart 缺省应为 false")
	}
}

// Test平台段缺省轮询间隔 配了 platform 但 poll_interval 缺省取 60s。
func Test平台段缺省轮询间隔(t *testing.T) {
	src := `
[platform]
base_url = "http://x:1"
mcp_token = "t"
`
	c, err := LoadString(src)
	if err != nil {
		t.Fatalf("LoadString 失败：%v", err)
	}
	if c.Platform.PollInterval != 60*time.Second {
		t.Fatalf("poll_interval 缺省应为 60s，得到 %v", c.Platform.PollInterval)
	}
}

// Test解析错误带行号定位 坏 TOML 的错误信息应包含行号。
func Test解析错误带行号定位(t *testing.T) {
	_, err := LoadString("[server\nbind = \"x\"")
	if err == nil {
		t.Fatal("坏 TOML 未报错")
	}
	if !strings.Contains(err.Error(), "line") && !strings.Contains(err.Error(), "行") {
		t.Fatalf("错误信息缺少行号定位：%v", err)
	}
}

// Test保存为可读格式 输出包含节注释头与缩进。
func Test保存为可读格式(t *testing.T) {
	out := MarshalTOML(sampleConfig())
	for _, want := range []string{"[server]", "[platform]", "[log]", "[[executor]]", "[executor.restart]", "# awem 配置文件", "  name"} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q：\n%s", want, out)
		}
	}
	// 中文与特殊字符必须被正确转义/保留（name 按 D8 只允许 [a-zA-Z0-9._-]，
	// 中文出现在 token 等值字段验证转义能力）
	if !strings.Contains(out, `no1-clarify`) {
		t.Fatalf("name 丢失：\n%s", out)
	}
	if !strings.Contains(out, `\"引号\"`) {
		t.Fatalf("引号未转义：\n%s", out)
	}
}

// Test保存原子替换 Save 后无残留 tmp 文件。
func Test保存原子替换(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("目录应只有 config.toml，实际 %d 个文件", len(entries))
	}
}
