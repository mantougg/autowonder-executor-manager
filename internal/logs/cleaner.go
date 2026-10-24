// cleaner.go：保留策略清理（T04-F3）。
// 按 retention_days 清理过期的滚动文件；触发时机（daemon 启动 + 周期）
// 由 daemon（T06）编排，本层提供纯函数；now 可注入便于测试。
package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// rotatedFileRe 匹配滚动文件 <name>.log.<N>。
var rotatedFileRe = regexp.MustCompile(`\.log\.\d+$`)

// CleanupExpired 删除 dir 下 mtime 早于 now-retentionDays 的滚动文件
// （*.log.<N>；活动文件 *.log 正在被写，不由本函数处理——执行器删除走 purge）。
// onRemove（可为 nil）在每个删除后回调，用于登记清理事件（F4 事件日志）。
// 返回全部被删除的路径。
func CleanupExpired(dir string, retentionDays int, now time.Time, onRemove func(path string)) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // 目录不存在视为无事可做
	}
	if err != nil {
		return nil, fmt.Errorf("读取日志目录 %s 失败：%w", dir, err)
	}
	if retentionDays <= 0 {
		retentionDays = 7
	}
	cutoff := now.AddDate(0, 0, -retentionDays)

	var removed []string
	for _, e := range entries {
		if e.IsDir() || !rotatedFileRe.MatchString(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				return removed, fmt.Errorf("删除过期日志 %s 失败：%w", path, err)
			}
			removed = append(removed, path)
			if onRemove != nil {
				onRemove(path)
			}
		}
	}
	return removed, nil
}

// IsRotatedName 报告文件名是否为滚动文件形态（导出供测试与诊断用）。
func IsRotatedName(name string) bool { return rotatedFileRe.MatchString(name) }
