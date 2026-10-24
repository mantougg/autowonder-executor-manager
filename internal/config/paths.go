// paths.go 提供 awem 全部约定路径的唯一入口（design.md §5.1）。
// 全项目取路径只允许经此处，禁止散落拼路径。

package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths 保存 awem 的目录与文件布局。
type Paths struct {
	Root            string // 配置根：os.UserConfigDir()/awem
	ConfigFile      string // Root/config.toml
	DataDir         string // Root/data
	TokenFile       string // Root/data/api-token
	DaemonFile      string // Root/data/daemon.json
	LogsDir         string // Root/data/logs
	EventsLog       string // Root/data/logs/events.log
	ExecutorsLogDir string // Root/data/logs/executors
}

// Layout 依据 OS 惯例配置根目录（userConfigDir 为 os.UserConfigDir() 的结果）
// 推导 awem 布局：
//
//	Linux   ~/.config/awem/
//	macOS   ~/Library/Application Support/awem/
//	Windows %AppData%\awem\
//
// 三平台差异全部由 os.UserConfigDir() 承担，本函数只负责追加 awem/ 与子布局，
// 测试可注入假 userConfigDir。
func Layout(userConfigDir string) Paths {
	return layoutAt(filepath.Join(userConfigDir, "awem"))
}

// At 以 root 直接作为 awem 根目录构造布局（root 已是 ~/.config/awem 这一层，
// 不再追加 awem/）。DefaultLayout 的 AWEM_CONFIG_DIR 分支与测试隔离用。
func At(root string) Paths { return layoutAt(root) }

// DefaultLayout 返回当前 OS 的真实布局。
// 支持环境变量 AWEM_CONFIG_DIR 覆盖配置根：其值直接作为 awem 根目录
// （等价于 ~/.config/awem 这一层），不再追加 awem/；测试隔离与多实例
// 冒烟用（实现细节选型，记录于 T06 变更记录）。
func DefaultLayout() (Paths, error) {
	if dir := os.Getenv("AWEM_CONFIG_DIR"); dir != "" {
		return At(dir), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("无法定位 OS 惯例配置目录：%w", err)
	}
	return layoutAt(filepath.Join(dir, "awem")), nil
}

// layoutAt 以 root 为 awem 根目录推导完整布局（Layout 的底层实现）。
func layoutAt(root string) Paths {
	data := filepath.Join(root, "data")
	logs := filepath.Join(data, "logs")
	return Paths{
		Root:            root,
		ConfigFile:      filepath.Join(root, "config.toml"),
		DataDir:         data,
		TokenFile:       filepath.Join(data, "api-token"),
		DaemonFile:      filepath.Join(data, "daemon.json"),
		LogsDir:         logs,
		EventsLog:       filepath.Join(logs, "events.log"),
		ExecutorsLogDir: filepath.Join(logs, "executors"),
	}
}

// dirPerm 是非 Windows 平台下 awem 目录的权限（仅当前用户）。
const dirPerm os.FileMode = 0o700

// EnsureDir 创建布局中的全部目录（已存在则跳过），非 Windows 下权限 0700，
// 并在 config.toml 不存在时写入默认模板。幂等：不覆盖已有配置。
func EnsureDir(p Paths) error {
	for _, dir := range []string{p.Root, p.DataDir, p.LogsDir, p.ExecutorsLogDir} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("创建目录 %s 失败：%w", dir, err)
		}
	}
	if _, err := os.Stat(p.ConfigFile); os.IsNotExist(err) {
		if err := os.WriteFile(p.ConfigFile, []byte(defaultConfigTOML), 0o600); err != nil {
			return fmt.Errorf("写入默认配置 %s 失败：%w", p.ConfigFile, err)
		}
	} else if err != nil {
		return fmt.Errorf("检查配置文件 %s 失败：%w", p.ConfigFile, err)
	}
	return nil
}
