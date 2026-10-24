//go:build windows

// statfs_free_windows.go：Windows 用 GetDiskFreeSpaceEx 读剩余空间。
package cli

import "golang.org/x/sys/windows"

func statfsFree(path string) (uint64, error) {
	var free, total, avail uint64
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &avail); err != nil {
		return 0, err
	}
	return free, nil
}
