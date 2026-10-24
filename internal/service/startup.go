//go:build windows

// startup.go：Windows 登录自启（T10-F4，design §12）。
// **方式选定（记录）**：默认实现"登录自启"（注册表 HKCU\...\Run，无需
// 管理员、以当前用户会话运行、无 session 0 环境变量问题）；帮助文案写明
// `sc create` 服务方式备选（须管理员、session 0 限制下 Node/qoder 环境
// 变量须机器级，7×24 场景才推荐）。
// **未真机验证**——代码实现 + GOOS=windows 交叉编译保证可构建，真机待
// Windows 环境复核（检查记录标注）。
package service

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"awem/internal/config"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValueName = "awem"

type startup struct{}

func init() { registerPlatform("windows", startup{}) }

func (startup) Describe() string {
	return "Windows 登录自启（HKCU Run）"
}

// runCommand 自启命令（带引号，兼容路径空格/中文）。
func runCommand(bin string) string {
	return `"` + bin + `" daemon`
}

func (startup) Installed(bin string, p config.Paths) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValueName)
	return err == nil
}

func (startup) Running(bin string, p config.Paths) (bool, string) {
	// 登录自启无服务管理器的"运行态"概念：以 daemon.json 存活为准
	if di, alive, _ := config.ReadDaemonInfo(p); alive {
		return true, fmt.Sprintf("daemon 运行中（pid %d，登录自启已注册）", di.PID)
	}
	return false, "daemon 未运行（登录自启已注册，注销/重启后自动拉起）"
}

func (startup) Install(bin string, p config.Paths, notify Notifier) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表 Run 键失败：%w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(runValueName, runCommand(bin)); err != nil {
		return fmt.Errorf("写入自启项失败：%w", err)
	}
	notify("已注册 HKCU Run 自启：" + runValueName)
	notify("备选：`sc create awem binPath= \"...\"` 注册系统服务（需管理员；session 0 下 Node/qoder 环境变量须机器级，7×24 场景推荐）")
	// 立即拉起 daemon（与 Linux/macOS 的"装好即跑"语义一致）
	return spawnDaemonNow(bin, notify)
}

func (startup) Uninstall(bin string, p config.Paths, notify Notifier) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表 Run 键失败：%w", err)
	}
	defer k.Close()
	if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("删除自启项失败：%w", err)
	}
	notify("已删除 HKCU Run 自启项")
	// 停止在跑 daemon（如有）
	if di, alive, _ := config.ReadDaemonInfo(p); alive {
		if proc, err := os.FindProcess(di.PID); err == nil {
			_ = proc.Kill()
			notify(fmt.Sprintf("已停止 daemon（pid %d）", di.PID))
		}
	}
	return nil
}

// spawnDaemonNow 立即启动 daemon（分离进程组 + 无控制台）。
func spawnDaemonNow(bin string, notify Notifier) error {
	const detachedProcess = 0x00000008
	cmd := exec.Command(bin, "daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 daemon 失败：%w", err)
	}
	notify(fmt.Sprintf("已启动 daemon（pid %d）", cmd.Process.Pid))
	return nil
}
