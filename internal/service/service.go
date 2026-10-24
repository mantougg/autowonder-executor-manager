// Package service 实现 awem service install|uninstall|status（T10-F1 框架）。
// 按 runtime.GOOS 分发到平台实现（systemd.go / launchd.go / startup.go）；
// 操作结果写管理事件日志；重复 install 幂等（已安装 → 更新定义文件并
// restart，策略记录于任务文档：用户重装新版本二进制后执行 install 即生效）。
package service

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"awem/internal/config"
	"awem/internal/logs"
)

// ErrUnsupportedPlatform 平台实现缺失（明确报错而非空跑，F1 验收）。
var ErrUnsupportedPlatform = errors.New("当前平台暂不支持系统服务（支持：linux/darwin/windows）")

// Notifier 上报操作事件（daemon/CLI 侧接事件日志或 stdout）。
type Notifier func(detail string)

// Platform 是平台服务实现的契约。
type Platform interface {
	// Install 安装并启动（幂等：已安装则更新定义并重启）。
	Install(binPath string, p config.Paths, notify Notifier) error
	// Uninstall 停止、注销并删除定义文件（未安装为 no-op 成功）。
	Uninstall(binPath string, p config.Paths, notify Notifier) error
	// Installed 定义文件是否存在。
	Installed(binPath string, p config.Paths) bool
	// Running 服务托管下 daemon 是否在跑（返回 false 与错误信息）。
	Running(binPath string, p config.Paths) (bool, string)
	// Describe 机制人读名（如 "systemd user 服务"）。
	Describe() string
}

// platforms 由各平台实现文件（systemd.go/launchd.go/startup.go）在 init
// 里注册——本文件无 build tag，只能引用注册结果而非具体类型。
var platforms = map[string]Platform{}

// registerPlatform 平台实现注册（各 GOOS 文件 init 调用）。
func registerPlatform(goos string, impl Platform) {
	platforms[goos] = impl
}

// ForOS 返回平台实现。
func ForOS(goos string) (Platform, error) {
	if impl, ok := platforms[goos]; ok {
		return impl, nil
	}
	return nil, ErrUnsupportedPlatform
}

// Current 当前平台实现。
func Current() (Platform, error) { return ForOS(runtime.GOOS) }

// binSelf 当前二进制绝对路径（install 时写进服务定义）。
func binSelf() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("定位 awem 二进制失败：%w", err)
	}
	return self, nil
}

// Install 服务安装（CLI service install 入口）。事件：service_installed。
func Install(el *logs.EventLog) error {
	plat, err := Current()
	if err != nil {
		return err
	}
	bin, err := binSelf()
	if err != nil {
		return err
	}
	p, err := config.DefaultLayout()
	if err != nil {
		return err
	}
	notify := eventNotifier(el, "install")
	if err := plat.Install(bin, p, notify); err != nil {
		return err
	}
	record(el, "已安装"+plat.Describe()+"（二进制："+bin+"）")
	return nil
}

// Uninstall 服务卸载（CLI service uninstall 入口）。
func Uninstall(el *logs.EventLog) error {
	plat, err := Current()
	if err != nil {
		return err
	}
	bin, err := binSelf()
	if err != nil {
		return err
	}
	p, err := config.DefaultLayout()
	if err != nil {
		return err
	}
	notify := eventNotifier(el, "uninstall")
	if err := plat.Uninstall(bin, p, notify); err != nil {
		return err
	}
	record(el, "已卸载"+plat.Describe())
	return nil
}

// StatusInfo 是 status 输出结构。
type StatusInfo struct {
	Installed bool
	Running   bool
	Detail    string // 机制/提示（linger 提示等）
	PID       int
	Port      int
}

// Status 服务状态三态（+ daemon.json 的 pid/port）。
func Status() (StatusInfo, error) {
	plat, err := Current()
	if err != nil {
		return StatusInfo{}, err
	}
	bin, _ := binSelf()
	p, err := config.DefaultLayout()
	if err != nil {
		return StatusInfo{}, err
	}
	info := StatusInfo{Detail: plat.Describe()}
	info.Installed = plat.Installed(bin, p)
	if info.Installed {
		info.Running, info.Detail = plat.Running(bin, p)
	}
	if di, alive, _ := config.ReadDaemonInfo(p); alive {
		info.PID, info.Port = di.PID, di.Port
	}
	return info, nil
}

// eventNotifier 把平台实现的步骤输出转事件。
func eventNotifier(el *logs.EventLog, action string) Notifier {
	return func(detail string) { record(el, "["+action+"] "+detail) }
}

// record 事件落盘（事件日志不可用时静默跳过——CLI 单次运行可能没有事件文件）。
func record(el *logs.EventLog, detail string) {
	if el != nil {
		_ = el.Record(logs.EventServiceInstalled, "", detail)
	}
}
