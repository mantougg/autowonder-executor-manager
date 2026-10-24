//go:build windows

// lock_windows.go：Windows 用 LockFileEx 独占锁（进程退出由内核释放）。
// LockFileEx 不在标准库 syscall 中，使用 golang.org/x/sys/windows（go.mod 已依赖）。
package daemon

import (
	"os"

	"golang.org/x/sys/windows"
)

func tryLockFile(f *os.File) error {
	const flags = windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY
	// 锁文件第一个字节（范围 0..1），足够实现互斥
	return windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, new(windows.Overlapped))
}

func unlockFile(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}

// splitPath 供 lock.go 的 parentOf 使用（兼容 / 与 \ 分隔符）。
func splitPath(path string) (dir, file string) {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i], path[i+1:]
		}
	}
	return ".", path
}
