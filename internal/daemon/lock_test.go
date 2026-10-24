// lock_test.go：单实例锁单测（同进程两个 fd 互斥 + 释放后可重取）。
package daemon

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestLock互斥与释放(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.lock")
	l1, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("首次加锁失败：%v", err)
	}
	// 第二次（模拟另一实例）应失败且错误可识别为 ErrLockHeld
	_, err = AcquireLock(path)
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("二次加锁应报 ErrLockHeld，实际：%v", err)
	}
	l1.Release()
	// 释放后可重新获取；Release 幂等
	l1.Release()
	l2, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("释放后重新加锁失败：%v", err)
	}
	l2.Release()
}
