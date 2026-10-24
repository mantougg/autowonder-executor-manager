//go:build !windows

// statfs_free_unix.go：非 Windows 用 statfs 读目录剩余空间。
package cli

import "golang.org/x/sys/unix"

func statfsFree(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
