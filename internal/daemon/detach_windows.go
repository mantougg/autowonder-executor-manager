//go:build windows

// detach_windows.go：daemon 后台拉起的脱钩参数（新进程组 + 分离控制台）。
// DETACHED_PROCESS 常量标准库 syscall 未导出，用 winbase.h 字面值 0x8。
package daemon

import (
	"os/exec"
	"syscall"
)

const detachedProcess = 0x00000008 // windows DETACHED_PROCESS

func detachDaemon(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}
