// supervisor_test.go：T05 验收测试。假 npx = testdata/fakeproc 构建产物
// （TestMain 统一构建），测试不依赖真实 npx/网络/平台。
package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"awem/internal/config"
	"awem/internal/logs"
)

// fakeNpxPath 是假 npx 可执行文件路径（TestMain 构建）。
var fakeNpxPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "awem-fakeproc")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "fakeproc")
	build := exec.Command("go", "build", "-o", out, "./testdata/fakeproc")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(fmt.Sprintf("构建 fakeproc 失败：%v", err))
	}
	fakeNpxPath = out
	os.Exit(m.Run())
}

// testEnv 装配一个可测的 Supervisor（毫秒级快参数 + 假 npx + 隔离家目录）。
type testEnv struct {
	sup  *Supervisor
	mgr  *logs.Manager
	dir  string
	home string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	mgr := logs.NewManager(
		filepath.Join(dir, "logs", "executors"),
		filepath.Join(dir, "logs", "events.log"),
		0, 0, 0,
	)
	t.Cleanup(mgr.Close)
	env := &testEnv{
		mgr:  mgr,
		dir:  dir,
		home: home,
	}
	env.sup = New(Options{
		Logs: mgr,
		Backoff: BackoffParams{
			Initial:     20 * time.Millisecond,
			Max:         80 * time.Millisecond,
			StableReset: 150 * time.Millisecond,
			Grace:       2 * time.Second,
		},
		ResolveNpx: func() (string, error) { return fakeNpxPath, nil },
		HomeDir:    func() (string, error) { return home, nil },
	})
	t.Cleanup(env.sup.Shutdown)
	return env
}

// execWith 构造一个合法的执行器配置（connect 命令 + 追加的 fake 控制 flag）。
func execWith(name, fakeFlags string) config.Executor {
	cmd := "npx -y autowonder@0.3.22 connect --ws-url ws://127.0.0.1:1/ws " +
		"--token test-token-123456 --executor-id 10086 --provider qoder"
	if fakeFlags != "" {
		cmd += " " + fakeFlags
	}
	return config.Executor{
		Name:    name,
		Command: cmd,
		Restart: config.RestartPolicy{Enabled: config.BoolPtr(true)},
	}
}

// setCfg 注入配置并返回它。
func (e *testEnv) setCfg(t *testing.T, execs ...config.Executor) *config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Executors = execs
	if err := cfg.Validate(); err != nil {
		t.Fatalf("测试配置不合法：%v", err)
	}
	e.sup.SetConfig(&cfg)
	return &cfg
}

// waitFor 轮询等待条件成立（超时失败）。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待超时：%s", what)
}

// snapshotOf 取单执行器快照（不存在则失败）。
func (e *testEnv) snapshotOf(t *testing.T, name string) UnitStatus {
	t.Helper()
	st, ok := e.sup.SnapshotOne(name)
	if !ok {
		t.Fatalf("执行器 %s 不在配置中", name)
	}
	return st
}

// tailLog 读执行器日志尾部。
func (e *testEnv) tailLog(t *testing.T, name string, n int) []string {
	t.Helper()
	lines, err := e.mgr.Tail(name, n)
	if err != nil {
		t.Fatalf("读取日志失败：%v", err)
	}
	return lines
}

// eventsText 读全部事件日志文本。
func (e *testEnv) eventsText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.dir, "logs", "events.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("读取事件日志失败：%v", err)
	}
	return string(data)
}

// pidAlive 报告进程是否存活（信号 0 探测）。
func pidAlive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// ---------- F1：spawn ----------

// 验收：spawn 成功、pid 记录；假进程回显环境变量注入生效。
func TestStartExecutorSpawnRecordsPid(t *testing.T) {
	e := newTestEnv(t)
	e.setCfg(t, execWith("e1", "-fake-mode=run"))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	st := e.snapshotOf(t, "e1")
	if st.State != StateRunning {
		t.Fatalf("状态=%s，want running", st.State)
	}
	if st.PID <= 0 || !pidAlive(st.PID) {
		t.Fatalf("pid=%d 未记录或进程不存活", st.PID)
	}
	// 再取一次快照，uptime 应随时间增长（>=0 且StartedAt 非零）
	st2 := e.snapshotOf(t, "e1")
	if st2.StartedAt.IsZero() || st2.Uptime < 0 {
		t.Fatalf("快照时间字段异常：startedAt=%v uptime=%v", st2.StartedAt, st2.Uptime)
	}
	if err := e.sup.StopExecutor("e1"); err != nil {
		t.Fatalf("停止失败：%v", err)
	}
}

// 验收：AUTOWONDER_WORKSPACE_ROOT 默认值注入（~/autowonder_workspaces_<provider>_<id>）。
func TestSpawnInjectsWorkspaceRootEnv(t *testing.T) {
	e := newTestEnv(t)
	e.setCfg(t, execWith("e1", "-fake-mode=run -fake-echo-env=AUTOWONDER_WORKSPACE_ROOT"))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	want := filepath.Join(e.home, "autowonder_workspaces_qoder_10086")
	waitFor(t, "日志出现环境变量回显", func() bool {
		for _, l := range e.tailLog(t, "e1", 20) {
			if l == "ENV AUTOWONDER_WORKSPACE_ROOT="+want {
				return true
			}
		}
		return false
	})
	_ = e.sup.StopExecutor("e1")
}

// 验收：workspace_root 覆盖 → env 注入覆盖值 + argv 追加 --workspace-root。
func TestSpawnWorkspaceRootOverride(t *testing.T) {
	e := newTestEnv(t)
	ws := filepath.Join(e.dir, "custom-ws")
	ex := execWith("e1", "-fake-mode=run -fake-echo-env=AUTOWONDER_WORKSPACE_ROOT -fake-argv-file="+filepath.Join(e.dir, "argv.txt"))
	ex.WorkspaceRoot = ws
	e.setCfg(t, ex)
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	waitFor(t, "日志出现环境变量回显", func() bool {
		for _, l := range e.tailLog(t, "e1", 20) {
			if l == "ENV AUTOWONDER_WORKSPACE_ROOT="+ws {
				return true
			}
		}
		return false
	})
	waitFor(t, "argv 文件落盘", func() bool {
		return fileContains(filepath.Join(e.dir, "argv.txt"), "--workspace-root\n"+ws+"\n")
	})
	_ = e.sup.StopExecutor("e1")
}

// 验收：命令里的 --debug 原样透传到子进程 argv。
func TestSpawnDebugFlagPassthrough(t *testing.T) {
	e := newTestEnv(t)
	e.setCfg(t, execWith("e1", "--debug -fake-mode=run -fake-argv-file="+filepath.Join(e.dir, "argv.txt")))
	if err := e.sup.StartExecutor("e1"); err != nil {
		t.Fatalf("启动失败：%v", err)
	}
	waitFor(t, "argv 文件落盘且含 --debug", func() bool {
		return fileContains(filepath.Join(e.dir, "argv.txt"), "--debug\n")
	})
	_ = e.sup.StopExecutor("e1")
}

// 验收：npx 不存在时错误清晰（中文、指向 doctor），状态 Failed，事件留痕。
func TestSpawnNpxMissingClearError(t *testing.T) {
	dir := t.TempDir()
	mgr := logs.NewManager(filepath.Join(dir, "logs", "executors"), filepath.Join(dir, "logs", "events.log"), 0, 0, 0)
	t.Cleanup(mgr.Close)
	sup := New(Options{
		Logs:       mgr,
		Backoff:    BackoffParams{Initial: 10 * time.Millisecond, Max: 20 * time.Millisecond, StableReset: 50 * time.Millisecond, Grace: time.Second},
		ResolveNpx: func() (string, error) { return "", ErrNpxNotFound },
		HomeDir:    func() (string, error) { return dir, nil },
	})
	t.Cleanup(sup.Shutdown)
	cfg := config.Default()
	cfg.Executors = []config.Executor{execWith("e1", "")}
	sup.SetConfig(&cfg)

	err := sup.StartExecutor("e1")
	if err == nil {
		t.Fatal("应返回错误")
	}
	if !strings.Contains(err.Error(), "awem doctor") || !strings.Contains(err.Error(), "npx") {
		t.Fatalf("错误信息不清晰：%v", err)
	}
	st, ok := sup.SnapshotOne("e1")
	if !ok || st.State != StateFailed {
		t.Fatalf("状态应为 failed，got %+v", st)
	}
	data, rerr := os.ReadFile(filepath.Join(dir, "logs", "events.log"))
	if rerr != nil || !strings.Contains(string(data), "executor_spawn_fail") {
		t.Fatalf("事件日志缺 spawn_fail：%v %s", rerr, data)
	}
}

// 验收：默认 npx 解析策略在本机可用时返回存在的文件（无 npx 的环境跳过）。
func TestDefaultNpxResolver(t *testing.T) {
	path, err := DefaultNpxResolver()()
	if err == ErrNpxNotFound {
		t.Skip("本机无 npx，跳过（策略分支已由注入测试覆盖）")
	}
	if err != nil {
		t.Fatalf("默认解析失败：%v", err)
	}
	if _, serr := os.Stat(path); serr != nil {
		t.Fatalf("解析到的路径不存在：%s（%v）", path, serr)
	}
}

// fileContains 报告文件内容是否包含子串。
func fileContains(path, sub string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), sub)
}
