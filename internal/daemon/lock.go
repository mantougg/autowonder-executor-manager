// lock.go：daemon 单实例锁。方案（记录于 T06 变更记录）：独占文件锁
// data/daemon.lock——Unix 用 flock、Windows 用 LockFileEx。锁绑定在打开的
// 文件描述上，进程退出（含强杀 kill -9）时由内核自动释放，因此无需任何
// 陈锁清理逻辑，天然满足"强杀后能重新启动"。
package daemon

import (
	"errors"
	"fmt"
	"os"
)

// ErrLockHeld 表示锁已被另一个 daemon 进程持有（存在运行中的实例）。
var ErrLockHeld = errors.New("daemon 锁已被其他实例持有")

// instanceLock 是一把已获取的独占文件锁；Release 幂等。
type instanceLock struct {
	f *os.File
}

// AcquireLock 尝试获取 daemon 单实例锁（非阻塞）：成功返回锁句柄；
// 已被持有时返回包装了 ErrLockHeld 的错误。
func AcquireLock(path string) (*instanceLock, error) {
	if err := os.MkdirAll(parentOf(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建锁文件目录失败：%w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件 %s 失败：%w", path, err)
	}
	if err := tryLockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("%w（锁文件 %s）", ErrLockHeld, path)
	}
	return &instanceLock{f: f}, nil
}

// Release 释放并关闭锁文件（幂等；进程退出时内核亦会自动释放）。
func (l *instanceLock) Release() {
	if l == nil || l.f == nil {
		return
	}
	unlockFile(l.f)
	l.f.Close()
	l.f = nil
}

// parentOf 返回路径所在目录。
func parentOf(path string) string {
	dir, _ := splitPath(path)
	return dir
}
