//go:build windows

// daemon_windows.go Windows 平台判断 pid 是否存活：OpenProcess 探测。
package config

import (
	"golang.org/x/sys/windows"
)

func pidAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	// 仍存活判定：ExitCode != STILL_ACTIVE 即已退出
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}
