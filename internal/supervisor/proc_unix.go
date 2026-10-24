//go:build !windows

package supervisor

import (
	"fmt"
	"io"
	"os/exec"
	"syscall"
)

// newExecCmd 构造子进程命令：Unix 下 Setpgid=true 让子进程自成进程组
// （组长=自身 pid），停止时可对整组发信号，兜住 npx → node → daemon 整棵树。
func newExecCmd(path string, args []string, env []string, stdout, stderr io.Writer) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// terminateTree 对进程组发 SIGTERM（优雅终止整棵树）。
// 组长已退出时返回 nil（组已不复存在，视为成功，幂等语义由调用方保证）。
func terminateTree(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH {
			return nil
		}
		return fmt.Errorf("向进程组 %d 发送 SIGTERM 失败：%w", pid, err)
	}
	return nil
}

// killTree 对进程组发 SIGKILL（宽限期后强杀整棵树）。
func killTree(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		if err == syscall.ESRCH {
			return nil
		}
		return fmt.Errorf("向进程组 %d 发送 SIGKILL 失败：%w", pid, err)
	}
	return nil
}
