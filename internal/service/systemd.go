//go:build linux

// systemd.go：Linux systemd user 服务（T10-F2，design §12）。
// unit 路径 ~/.config/systemd/user/awem.service；ExecStart 为当前二进制
// 绝对路径（systemd 支持 %h 与引号，用户名含空格/中文时路径走引号包裹）；
// 日志沿用 awem 自身文件日志（data/logs/，不接 journald——日志体系已含
// 滚动/保留/tail/WS，journald 双轨反而割裂，理由记录于任务文档）。
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"awem/internal/config"
)

// unitPath systemd user unit 文件路径。
func unitPath(p config.Paths) string {
	return filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "awem.service")
}

// unitTemplate 生成 unit 内容（模板记录于任务文档变更记录）。
func unitTemplate(bin string) string {
	// ExecStart 带引号包裹的二进制路径（空格/中文用户名安全）+ daemon 子命令
	return fmt.Sprintf(`[Unit]
Description=awem - AutoWonder Executor Manager
After=network-online.target

[Service]
Type=simple
ExecStart=%q daemon
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, bin)
}

type systemd struct{}

func init() { registerPlatform("linux", systemd{}) }

func (systemd) Describe() string { return "systemd user 服务" }

func (systemd) Installed(bin string, p config.Paths) bool {
	_, err := os.Stat(unitPath(p))
	return err == nil
}

func (systemd) Running(bin string, p config.Paths) (bool, string) {
	out, err := exec.Command("systemctl", "--user", "is-active", "awem").Output()
	state := strings.TrimSpace(string(out))
	if err != nil || state != "active" {
		if state == "" {
			state = "unknown（" + err.Error() + "）"
		}
		return false, "systemd user 服务已安装，状态：" + state
	}
	return true, "systemd user 服务运行中"
}

func (systemd) Install(bin string, p config.Paths, notify Notifier) error {
	path := unitPath(p)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 systemd user 目录失败：%w", err)
	}
	if err := os.WriteFile(path, []byte(unitTemplate(bin)), 0o644); err != nil {
		return fmt.Errorf("写入 unit 失败：%w", err)
	}
	notify("已写入 " + path)
	// 幂等：重复 install = 更新定义并重启（新版本二进制即生效）
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("daemon-reload 失败：%w", err)
	}
	if err := exec.Command("systemctl", "--user", "enable", "--now", "awem").Run(); err != nil {
		return fmt.Errorf("enable --now 失败：%w", err)
	}
	notify("已 systemctl --user enable --now awem")
	notify("提示：执行 `loginctl enable-linger " + os.Getenv("USER") +
		"` 后，不登录也会常驻（当前未自动开启）")
	return nil
}

func (systemd) Uninstall(bin string, p config.Paths, notify Notifier) error {
	path := unitPath(p)
	if _, err := os.Stat(path); err != nil {
		return nil // 未安装：no-op 成功（幂等）
	}
	_ = exec.Command("systemctl", "--user", "disable", "--now", "awem").Run()
	notify("已 systemctl --user disable --now awem")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除 unit 失败：%w", err)
	}
	notify("已删除 " + path)
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}
