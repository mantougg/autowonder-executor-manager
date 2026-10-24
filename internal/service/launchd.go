//go:build darwin

// launchd.go：macOS launchd 用户代理（T10-F3，design §12）。
// plist：~/Library/LaunchAgents/com.autowonder.awem.plist（RunAtLoad +
// KeepAlive；模板记录于任务文档）。**未真机验证**——代码实现 + 交叉编译
// 保证可构建，真机安装/卸载待 macOS 环境复核（检查记录标注）。
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"awem/internal/config"
)

const launchdLabel = "com.autowonder.awem"

func plistPath(p config.Paths) string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", launchdLabel+".plist")
}

// plistTemplate 生成 plist 内容（模板记录于任务文档）。
func plistTemplate(bin string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>daemon</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
</dict>
</plist>
`, launchdLabel, bin)
}

type launchd struct{}

func init() { registerPlatform("darwin", launchd{}) }

func (launchd) Describe() string { return "launchd 用户代理（LaunchAgent）" }

func (launchd) Installed(bin string, p config.Paths) bool {
	_, err := os.Stat(plistPath(p))
	return err == nil
}

func (launchd) Running(bin string, p config.Paths) (bool, string) {
	out, err := exec.Command("launchctl", "list", launchdLabel).Output()
	if err != nil {
		return false, "launchd 已安装，未查询到运行状态"
	}
	// launchctl list 输出：PID 与最后退出码；首列为 "-"/pid
	fields := strings.Fields(string(out))
	if len(fields) > 0 && fields[0] != "-" {
		return true, "launchd 运行中（pid " + fields[0] + "）"
	}
	return false, "launchd 已安装，当前未运行（KeepAlive 会拉起）"
}

func (launchd) Install(bin string, p config.Paths, notify Notifier) error {
	path := plistPath(p)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 LaunchAgents 目录失败：%w", err)
	}
	if err := os.WriteFile(path, []byte(plistTemplate(bin)), 0o644); err != nil {
		return fmt.Errorf("写入 plist 失败：%w", err)
	}
	notify("已写入 " + path)
	if err := exec.Command("launchctl", "unload", path).Run(); err == nil {
		notify("已卸载旧实例（幂等重装）")
	}
	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		return fmt.Errorf("launchctl load 失败：%w", err)
	}
	notify("已 launchctl load（RunAtLoad + KeepAlive）")
	return nil
}

func (launchd) Uninstall(bin string, p config.Paths, notify Notifier) error {
	path := plistPath(p)
	if _, err := os.Stat(path); err != nil {
		return nil // 未安装：no-op 成功
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	notify("已 launchctl unload")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除 plist 失败：%w", err)
	}
	notify("已删除 " + path)
	return nil
}
