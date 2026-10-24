//go:build !windows

// lock_unix.go：非 Windows 平台用 flock（fcntl 族之外的 BSD 文件锁；
// 锁属于 open file description，进程退出即释放）。
package daemon

import (
	"os"
	"syscall"
)

func tryLockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockFile(f *os.File) {
	// 关闭 fd 即释放；flock(LOCK_UN) 显式解锁仅作防御
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// splitPath 供 lock.go 的 parentOf 使用（Unix 路径分隔符）。
func splitPath(path string) (dir, file string) {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i], path[i+1:]
		}
	}
	return ".", path
}
