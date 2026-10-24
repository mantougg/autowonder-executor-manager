package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLayout路径布局 注入假 userConfigDir 验证 §5.1 的目录推导逻辑。
// 三平台根目录差异由 os.UserConfigDir() 承担（Linux ~/.config、
// macOS ~/Library/Application Support、Windows %AppData%），此处验证追加规则。
func TestLayout路径布局(t *testing.T) {
	p := Layout("/fake/userconfig")
	want := map[string]string{
		"Root":            "/fake/userconfig/awem",
		"ConfigFile":      "/fake/userconfig/awem/config.toml",
		"DataDir":         "/fake/userconfig/awem/data",
		"TokenFile":       "/fake/userconfig/awem/data/api-token",
		"DaemonFile":      "/fake/userconfig/awem/data/daemon.json",
		"LogsDir":         "/fake/userconfig/awem/data/logs",
		"EventsLog":       "/fake/userconfig/awem/data/logs/events.log",
		"ExecutorsLogDir": "/fake/userconfig/awem/data/logs/executors",
	}
	got := map[string]string{
		"Root":            p.Root,
		"ConfigFile":      p.ConfigFile,
		"DataDir":         p.DataDir,
		"TokenFile":       p.TokenFile,
		"DaemonFile":      p.DaemonFile,
		"LogsDir":         p.LogsDir,
		"EventsLog":       p.EventsLog,
		"ExecutorsLogDir": p.ExecutorsLogDir,
	}
	for field, w := range want {
		// Windows 下分隔符不同，比较时统一转为 /
		if filepath.ToSlash(got[field]) != filepath.ToSlash(w) {
			t.Fatalf("%s = %q，期望 %q", field, got[field], w)
		}
	}
}

// TestDefaultLayoutLinuxUserConfigDir 用 XDG_CONFIG_HOME 覆盖验证
// Linux 下 os.UserConfigDir → ~/.config/awem/ 的 §5.1 映射（仅 Linux 跑）。
func TestDefaultLayoutLinuxUserConfigDir(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("仅 Linux 验证 XDG 路径")
	}
	t.Setenv("XDG_CONFIG_HOME", "/fake/xdg")
	p, err := DefaultLayout()
	if err != nil {
		t.Fatalf("DefaultLayout 报错：%v", err)
	}
	if p.Root != "/fake/xdg/awem" {
		t.Fatalf("Root = %q，期望 /fake/xdg/awem", p.Root)
	}
}

// TestDefaultLayout环境变量覆盖 AWEM_CONFIG_DIR 直接作为根目录（不追加 awem/），
// daemon 集成测试用它在临时目录隔离运行。
func TestDefaultLayout环境变量覆盖(t *testing.T) {
	t.Setenv("AWEM_CONFIG_DIR", "/fake/root")
	p, err := DefaultLayout()
	if err != nil {
		t.Fatalf("DefaultLayout 报错：%v", err)
	}
	if p.Root != "/fake/root" || p.ConfigFile != filepath.Join("/fake/root", "config.toml") {
		t.Fatalf("env 覆盖未生效：Root=%q ConfigFile=%q", p.Root, p.ConfigFile)
	}
}

// TestEnsureDir生成目录树与默认配置 验证首次运行行为与幂等。
func TestEnsureDir生成目录树与默认配置(t *testing.T) {
	base := t.TempDir()
	p := Layout(base)

	if err := EnsureDir(p); err != nil {
		t.Fatalf("EnsureDir 失败：%v", err)
	}
	for _, dir := range []string{p.Root, p.DataDir, p.LogsDir, p.ExecutorsLogDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("目录 %s 未创建：%v", dir, err)
		} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			t.Fatalf("目录 %s 权限 = %o，期望 0700", dir, info.Mode().Perm())
		}
	}
	data, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		t.Fatalf("默认 config.toml 未生成：%v", err)
	}
	if !strings.Contains(string(data), "# awem 配置文件") {
		t.Fatalf("默认配置缺少中文注释头：\n%s", data)
	}

	// 幂等：已有配置不被覆盖
	if err := os.WriteFile(p.ConfigFile, []byte("# 用户自定义配置"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(p); err != nil {
		t.Fatalf("第二次 EnsureDir 失败：%v", err)
	}
	data2, _ := os.ReadFile(p.ConfigFile)
	if string(data2) != "# 用户自定义配置" {
		t.Fatal("EnsureDir 覆盖了用户已有配置")
	}
}
