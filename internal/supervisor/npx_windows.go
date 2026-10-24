//go:build windows

package supervisor

import "os/exec"

// defaultNpxResolver：Windows 策略——npm 的 npx 是批处理 npx.cmd，
// 依次尝试 npx.cmd / npx.exe / npx；不做 shell 回退（无 POSIX 登录 shell 语义，
// GUI 启动的 PATH 缺失问题交由文档/doctor 提示用户）。
func defaultNpxResolver() NpxResolver {
	return func() (string, error) {
		for _, name := range []string{"npx.cmd", "npx.exe", "npx"} {
			if p, err := exec.LookPath(name); err == nil {
				return p, nil
			}
		}
		return "", ErrNpxNotFound
	}
}
