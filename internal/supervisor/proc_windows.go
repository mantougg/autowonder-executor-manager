//go:build windows

package supervisor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// newExecCmd 构造子进程命令：Windows 的 npx 是批处理（npx.cmd），Go 无法直接
// exec 批处理文件，须经 cmd.exe /c 包装（design §13；Job Object 方案留 T10 定稿）。
func newExecCmd(path string, args []string, env []string, stdout, stderr io.Writer) *exec.Cmd {
	full := append([]string{"/c", path}, args...)
	cmd := exec.Command(os.Getenv("ComSpec"), full...)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd
}

// terminateTree 用 taskkill /T 优雅终止进程树（无 SIGTERM 概念，/T 递归子树）。
func terminateTree(pid int) error {
	return runTaskkill(pid, false)
}

// killTree 用 taskkill /T /F 强杀进程树（宽限期后）。
func killTree(pid int) error {
	return runTaskkill(pid, true)
}

func runTaskkill(pid int, force bool) error {
	args := []string{"/T", "/PID", fmt.Sprint(pid)}
	if force {
		args = append(args, "/F")
	}
	out, err := exec.Command("taskkill", args...).CombinedOutput()
	if err != nil {
		// 进程已退出：taskkill 报"找不到进程"，视为成功（幂等语义由调用方保证）
		if strings.Contains(string(out), "not found") || strings.Contains(string(out), "找不到") {
			return nil
		}
		return fmt.Errorf("taskkill %v 失败：%v：%s", args, err, out)
	}
	return nil
}
