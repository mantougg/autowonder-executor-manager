// fakeproc 是假长驻子进程（supervisor/api/cli 测试共用，TestMain 构建为临时二进制，不进产品 awem）。
// 充当"假 npx"：忽略自身 argv 里的 npx 包装词（-y / autowonder@… / connect
// 及全部 connect 参数），只识别 -fake-xxx=yyy 形态的控制 flag。
//
// 控制_flag（全部用 = 单词形态，避免与 connect 参数的取值语义混淆）：
//
//	-fake-mode=run|crash|ignore-term|tree|child|exit-now   （默认 run）
//	-fake-after=ms        crash 前的存活时长（默认 0）
//	-fake-exit-code=N     crash/exit-now 的退出码（默认 crash=1 / exit-now=0）
//	-fake-echo-env=A,B    启动时逐行打印 ENV <name>=<value>（未设置打 <unset>）
//	-fake-lines=a|b|c     启动时逐行打印（| 分隔），配合 -fake-line-delay=ms
//	-fake-argv-file=PATH  把收到的 argv 逐行写入文件（透传断言用）
//	-fake-pidfile=PATH    把自身 pid 写入文件
//	-fake-child-pidfile=PATH  tree 模式：子进程 pid 写入该文件
//
// 信号语义：run/child/tree 收 SIGTERM/SIGINT 优雅退出（exit 0）；ignore-term
// 无视 SIGTERM（只能被 SIGKILL 强杀，验证强杀路径）；tree 会再起一个 child
// 组成进程树（验证整树终止）。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	flags := map[string]string{}
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "-fake-") {
			if i := strings.IndexByte(a, '='); i > 0 {
				flags[a[6:i]] = a[i+1:]
			} else {
				flags[a[6:]] = "true"
			}
		}
	}

	if pf := flags["pidfile"]; pf != "" {
		_ = os.WriteFile(pf, []byte(strconv.Itoa(os.Getpid())), 0o644)
	}
	if af := flags["argv-file"]; af != "" {
		var b strings.Builder
		for _, a := range os.Args[1:] {
			b.WriteString(a)
			b.WriteByte('\n')
		}
		_ = os.WriteFile(af, []byte(b.String()), 0o644)
	}
	for _, name := range strings.Split(flags["echo-env"], ",") {
		if name == "" {
			continue
		}
		if v, ok := os.LookupEnv(name); ok {
			fmt.Printf("ENV %s=%s\n", name, v)
		} else {
			fmt.Printf("ENV %s=<unset>\n", name)
		}
	}
	if ls := flags["lines"]; ls != "" {
		delay := dur(flags["line-delay"])
		for _, line := range strings.Split(ls, "|") {
			if line != "" {
				fmt.Println(line)
			}
			time.Sleep(delay)
		}
	}

	switch flags["mode"] {
	case "crash":
		time.Sleep(dur(flags["after"]))
		os.Exit(atoiOr(flags["exit-code"], 1))
	case "exit-now":
		os.Exit(atoiOr(flags["exit-code"], 0))
	case "ignore-term":
		// 只忽略 SIGTERM（再对 SIGTERM 调 Notify 会撤销忽略，故只监听 SIGINT）；
		// -fake-ready-file 在忽略生效后写入，供测试确认可安全发信号
		signal.Ignore(syscall.SIGTERM)
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT)
		if rf := flags["ready-file"]; rf != "" {
			_ = os.WriteFile(rf, []byte("ready"), 0o644)
		}
		<-ch
	case "tree":
		self, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		child := exec.Command(self, "-fake-mode=child", "-fake-pidfile="+flags["child-pidfile"])
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		_ = child.Process.Signal(syscall.SIGTERM)
		_, _ = child.Process.Wait()
		os.Exit(0)
	case "child":
		waitSignal()
	default: // run
		waitSignal()
	}
}

// waitSignal 等 SIGTERM/SIGINT 后以 0 退出（优雅退出语义）。
func waitSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
	os.Exit(0)
}

func dur(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		if ms, err2 := strconv.Atoi(s); err2 == nil {
			return time.Duration(ms) * time.Millisecond
		}
		return 0
	}
	return d
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
