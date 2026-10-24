//go:build !windows

package supervisor

import (
	"os/exec"
	"strings"
)

// defaultNpxResolver：Unix 系策略——LookPath 优先，登录 shell 回退。
func defaultNpxResolver() NpxResolver {
	return func() (string, error) {
		if p, err := exec.LookPath("npx"); err == nil {
			return p, nil
		}
		// 回退：登录 shell 的 PATH（GUI/系统服务启动时 daemon 缺 profile 注入的
		// nvm/mise 等 PATH；sh -lc 会加载 /etc/profile 与 ~/.profile）。
		out, err := exec.Command("/bin/sh", "-lc", "command -v npx").Output()
		if err == nil {
			if p := strings.TrimSpace(string(out)); p != "" && !strings.Contains(p, "\n") {
				return p, nil
			}
		}
		return "", ErrNpxNotFound
	}
}
