// clienv_test.go：CLI 子命令集测试环境（T07）。httptest 假 daemon
// （api.New 直装配，含 fakeproc 真子进程能力），Deps 直连注入。
package cli

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awem/internal/api"
	"awem/internal/config"
	"awem/internal/daemon"
	"awem/internal/logs"
	"awem/internal/platform"
	"awem/internal/supervisor"
)

// fakeNpxPath 由 TestMain 构建（internal/fakeproc 共享假进程）。
var fakeNpxPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "awem-cli-fakeproc")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "fakeproc")
	build := exec.Command("go", "build", "-o", out, "awem/internal/fakeproc")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}
	fakeNpxPath = out
	os.Exit(m.Run())
}

// cliEnv 是一个可测的 CLI 环境。
type cliEnv struct {
	reg        *Registry
	out        *bytes.Buffer
	stdin      *strings.Reader // 确认交互输入（每次调用前重置）
	p          config.Paths
	mgr        *logs.Manager
	sup        *supervisor.Supervisor
	watcher    *config.Watcher
	server     *httptest.Server
	apiSrv     *api.Server
	localExecs []config.Executor // 预置执行器（平台联动 LocalIDs 用）
	currentSvc *platform.Service // 当前装配的平台联动（端到端测试用）
	home       string            // 注入的家目录（import 归档用）
}

// newCLIEnv 构造环境；execs 为预置执行器（走 API 等价的落盘+发布路径）。
func newCLIEnv(t *testing.T, execs ...config.Executor) *cliEnv {
	t.Helper()
	dir := t.TempDir()
	p := config.At(dir)
	cfg := config.Default()
	cfg.Executors = execs
	if err := cfg.Validate(); err != nil {
		t.Fatalf("测试配置不合法：%v", err)
	}
	if err := config.Save(p.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	mgr := logs.NewManager(p.ExecutorsLogDir, p.EventsLog, 0, 0, 0)
	t.Cleanup(mgr.Close)
	w := config.NewWatcher(p.ConfigFile, cfg)
	sup := supervisor.New(supervisor.Options{
		Logs: mgr,
		Backoff: supervisor.BackoffParams{
			Initial: 20 * time.Millisecond, Max: 80 * time.Millisecond,
			StableReset: 150 * time.Millisecond, Grace: 2 * time.Second,
		},
		ResolveNpx: func() (string, error) { return fakeNpxPath, nil },
		HomeDir:    func() (string, error) { return "/tmp", nil },
	})
	t.Cleanup(sup.Shutdown)
	w.Subscribe(func(c config.Config) {
		cc := c
		sup.ApplyConfig(&cc)
	})
	sup.ApplyConfig(&cfg)

	srv := api.New(api.Options{
		Paths: p, Token: "cli-test-token", Logs: mgr, Sup: sup,
		Watcher: w, Port: 18765, Version: "test-1.2.3",
	})
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)

	env := &cliEnv{
		reg: NewRegistry(), out: &bytes.Buffer{},
		p: p, mgr: mgr, sup: sup, localExecs: execs,
		home: filepath.Join(dir, "home"),
	}
	env.apiSrv = srv
	env.server = server
	env.watcher = w
	if err := os.MkdirAll(env.home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.resetStdin)
	RegisterCommands(env.reg, Deps{
		Layout: func() (config.Paths, error) { return p, nil },
		Connect: func(context.Context) (*daemon.Client, error) {
			return daemon.NewClientAt(env.server.URL, "cli-test-token"), nil
		},
		In:      env, // 实现 io.Reader：每次 run 前重置（见 resetStdin/Run）
		Out:     env.out,
		HomeDir: func() (string, error) { return env.home, nil },
	})
	return env
}

// Read 实现 io.Reader：把预置的应答串作为 stdin（confirm 用）。
func (e *cliEnv) Read(b []byte) (int, error) {
	if e.stdin == nil {
		return 0, io.EOF
	}
	return e.stdin.Read(b)
}

// setAnswers 预置确认应答（逐次读取）。
func (e *cliEnv) setAnswers(answers ...string) {
	e.stdin = strings.NewReader(strings.Join(answers, "\n") + "\n")
}

func (e *cliEnv) resetStdin() { e.stdin = nil }

// run 执行一条命令并返回退出码。
func (e *cliEnv) run(jsonMode bool, args ...string) int {
	e.out.Reset()
	all := append([]string{}, args...)
	if jsonMode {
		all = append([]string{"--json"}, all...)
	}
	return e.reg.Dispatch(all)
}

// output 返回命令输出。
func (e *cliEnv) output() string { return e.out.String() }

const validCommand = "npx -y autowonder@0.3.22 connect --ws-url ws://127.0.0.1:1/ws " +
	"--token secret-token-abcdef123456 --executor-id 10086 --provider qoder --model qfmodel --reasoning-effort medium --context-window 1000000 --token-aware-enable"

// setPlatform 装配平台联动（重建 api server 带 Plat 注入；T08-F6 端到端用）。
// svc 为 nil 表示撤下联动（回到本地态）。
func (e *cliEnv) setPlatform(t *testing.T, svc *platform.Service) {
	t.Helper()
	e.server.Close()
	opts := api.Options{
		Paths: e.p, Token: "cli-test-token", Logs: e.mgr, Sup: e.sup,
		Watcher: e.watcher, Port: 18765, Version: "test-1.2.3",
	}
	if svc != nil {
		opts.Plat = svc
		opts.Ping = svc.Ping
		opts.PlatConfigured = func() bool { return true }
	}
	e.apiSrv = api.New(opts)
	e.server = httptest.NewServer(e.apiSrv.Handler())
	t.Cleanup(e.server.Close)
}
