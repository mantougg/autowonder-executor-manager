// daemon.go 管理 data/daemon.json：daemon 启动时写入 {pid, port}，
// 正常退出清理；读取方（CLI 惰性拉起，T06）读到 stale 记录视为无 daemon。
// 强杀残留的 stale/损坏文件在读时自愈（校验不崩溃、损坏文件直接删除）。
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// DaemonInfo 是 daemon 元数据。
type DaemonInfo struct {
	PID  int `json:"pid"`
	Port int `json:"port"`
}

// WriteDaemonInfo 启动时写入元数据（覆盖旧值）。
func WriteDaemonInfo(p Paths, info DaemonInfo) error {
	if err := os.MkdirAll(p.DataDir, dirPerm); err != nil {
		return fmt.Errorf("创建数据目录失败：%w", err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("序列化 daemon 元数据失败：%w", err)
	}
	if err := os.WriteFile(p.DaemonFile, data, 0o600); err != nil {
		return fmt.Errorf("写入 %s 失败：%w", p.DaemonFile, err)
	}
	return nil
}

// ReadDaemonInfo 读取元数据。
// 返回值：(信息, daemon 是否存活, 错误)。语义：
//   - 文件不存在 → (零值, false, nil)
//   - 文件损坏（非 JSON）→ (零值, false, nil)，并删除残留文件自愈
//   - pid 已不存活（stale）→ (记录值, false, nil)
//   - pid 存活 → (记录值, true, nil)
func ReadDaemonInfo(p Paths) (DaemonInfo, bool, error) {
	data, err := os.ReadFile(p.DaemonFile)
	if os.IsNotExist(err) {
		return DaemonInfo{}, false, nil
	}
	if err != nil {
		return DaemonInfo{}, false, fmt.Errorf("读取 %s 失败：%w", p.DaemonFile, err)
	}
	var info DaemonInfo
	if err := json.Unmarshal(data, &info); err != nil {
		// 损坏文件：按无 daemon 处理并自愈删除
		_ = os.Remove(p.DaemonFile)
		return DaemonInfo{}, false, nil
	}
	if info.PID <= 0 {
		// 非法记录：同样按无 daemon 处理并自愈
		_ = os.Remove(p.DaemonFile)
		return DaemonInfo{}, false, nil
	}
	return info, pidAlive(info.PID), nil
}

// ClearDaemonInfo 正常退出时清理元数据文件（不存在时为 no-op）。
func ClearDaemonInfo(p Paths) error {
	if err := os.Remove(p.DaemonFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理 %s 失败：%w", p.DaemonFile, err)
	}
	return nil
}
