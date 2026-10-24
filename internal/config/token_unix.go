//go:build !windows

// token_unix.go 非 Windows 平台的 token 文件权限收紧：显式 chmod 0600
// （仅当前用户可读写，design.md §10）。
package config

import (
	"fmt"
	"os"
)

func restrictTokenFilePerms(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("设置 token 文件权限 0600 失败：%w", err)
	}
	return nil
}
