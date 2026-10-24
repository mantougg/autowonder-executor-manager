// archive.go：旧日志归档（F5，--with-logs 的数据准备）。
// 把旧脚本布局 ~/.autowonder-executor/<name>.log 拷贝到 awem 的
// data/logs/executors/<name>.log（名称以导入确认后的新名为准）；
// 源不存在跳过不报错；只读源、只写目标，不删源文件。
package launchcmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ArchiveResult 是单条日志的归档结果（供 CLI 汇总输出）。
type ArchiveResult struct {
	Source string // 源文件绝对/给定路径
	Dest   string // 目标文件路径
	Copied bool   // 是否实际拷贝
	Reason string // 未拷贝原因（源不存在等）
}

// ArchiveLogs 按 renames（旧名 → 新名）把 srcDir/<旧名>.log 拷贝到
// dstDir/<新名>.log。renames 中同名映射（旧名==新名）也支持。
// 返回每条的处理结果；目录创建失败等致命错误返回 error。
func ArchiveLogs(srcDir, dstDir string, renames map[string]string) ([]ArchiveResult, error) {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建目标目录 %s 失败：%w", dstDir, err)
	}
	var results []ArchiveResult
	for oldName, newName := range renames {
		if newName == "" {
			newName = oldName
		}
		src := filepath.Join(srcDir, oldName+".log")
		dst := filepath.Join(dstDir, newName+".log")
		res := ArchiveResult{Source: src, Dest: dst}

		info, err := os.Stat(src)
		if os.IsNotExist(err) {
			res.Reason = "源日志不存在，跳过"
			results = append(results, res)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("检查源日志 %s 失败：%w", src, err)
		}
		if info.IsDir() {
			res.Reason = "源不是普通文件，跳过"
			results = append(results, res)
			continue
		}
		if err := copyFile(src, dst); err != nil {
			return nil, err
		}
		res.Copied = true
		results = append(results, res)
	}
	return results, nil
}

// copyFile 逐字节拷贝（目标权限 0644，内容与源一致）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源日志 %s 失败：%w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("创建目标日志 %s 失败：%w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("拷贝 %s → %s 失败：%w", src, dst, err)
	}
	return nil
}
