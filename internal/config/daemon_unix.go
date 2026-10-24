//go:build !windows

// daemon_unix.go 非 Windows 平台判断 pid 是否存活：kill(pid, 0) 探测。
package config

import "syscall"

func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM // EPERM=进程存在但无权发信号
}
