//go:build windows

// token_windows.go Windows 平台的 token 文件权限收紧：用 icacls 移除继承并
// 仅授予当前用户完全控制（design.md §10；真机行为验证记 T10）。
package config

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
)

func restrictTokenFilePerms(path string) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("获取当前用户失败：%w", err)
	}
	// /inheritance:r 移除继承的 ACE；/grant:r 仅授予当前用户，替换已有授权
	out, err := exec.Command("cmd", "/c",
		fmt.Sprintf(`icacls "%s" /inheritance:r /grant:r "%s:(F)"`, path, u.Username)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls 收紧权限失败：%w，输出：%s", err, out)
	}
	_ = os.Chmod(path, 0o600) // 附加一层，Windows 上主要靠 ACL
	return nil
}
