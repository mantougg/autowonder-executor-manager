//go:build !windows

// detach_unix.go：daemon 后台拉起的脱钩参数（setsid 脱离控制终端）。
package daemon

import (
	"os/exec"
	"syscall"
)

func detachDaemon(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
