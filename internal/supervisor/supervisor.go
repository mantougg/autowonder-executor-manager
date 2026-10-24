// supervisor.go：Supervisor —— 每执行器一个受管子进程的守护器（T05）。
// 职责：spawn（F1）、崩溃退避拉起（F2）、进程树停止（F3）、autostart（F4）、
// 状态快照与连接解析（F5）、事件收口（F6）。所有状态迁移都发事件
// （logs.EventLog），无"静默"路径。
//
// 并发模型：Supervisor.mu 只保护 units 表与 cfg；每个 unit 自带锁。
// 每次人为启动起一个 supervise 循环（代数 gen），循环内自动拉起不换循环、
// 不换 goroutine；人为停止通过 u.stopping + 关闭 u.sleepCh 通知循环收口。
package supervisor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awem/internal/config"
	"awem/internal/launchcmd"
	"awem/internal/logs"
)

// ErrExecutorNotFound 表示配置里没有该执行器（CLI 映射为退出码 3）。
var ErrExecutorNotFound = errors.New("执行器未找到")

// Options 是 Supervisor 的可注入依赖（测试注入替身，daemon 装配真身）。
type Options struct {
	Logs       *logs.Manager          // 日志与事件管理器（必需）
	Clock      Clock                  // 时钟（nil=系统时钟）
	Backoff    BackoffParams          // 守护参数（零值=设计默认）
	ResolveNpx NpxResolver            // npx 解析策略（nil=平台默认）
	Environ    func() []string        // 子进程环境基底（nil=os.Environ）
	HomeDir    func() (string, error) // 家目录（nil=os.UserHomeDir）
}

// Supervisor 管理全部执行器子进程；并发安全。
type Supervisor struct {
	opts   Options
	mu     sync.Mutex
	cfg    *config.Config
	units  map[string]*unit
	wg     sync.WaitGroup
	closed bool
}

// unit 是单个执行器的受管状态。
type unit struct {
	name string
	exec config.Executor // 当前生效配置（热加载更新，运行中进程不受影响）

	mu           sync.Mutex
	state        State
	cmd          *exec.Cmd
	pid          int
	done         chan struct{} // 当前 supervise 循环退出信号（每次人为启动换新）
	sleepCh      chan struct{} // 退避等待的中断信号（人为停止时关闭）
	startedAt    time.Time     // 当前进程启动时刻（uptime 与稳定期判定基准）
	restarts     int           // 自动拉起成功累计次数
	backoffStep  int           // 退避序列位置（0=下次起步用 Initial）
	pendingDelay time.Duration // Backoff 态下当前等待的退避间隔（快照展示用）
	lastExit     string        // 上次进程退出描述
	conn         ConnState     // 连接状态（F5，从日志行解析）
	stopping     bool          // 人为停止进行中（supervise 据此不拉起）
	gen          int           // 代数：每次人为启动递增，隔离旧循环迟写
}

// New 创建 Supervisor；配置由 SetConfig 注入。
func New(opts Options) *Supervisor {
	if opts.Clock == nil {
		opts.Clock = RealClock{}
	}
	if opts.Backoff == (BackoffParams{}) {
		opts.Backoff = DefaultBackoffParams()
	}
	if opts.ResolveNpx == nil {
		opts.ResolveNpx = DefaultNpxResolver()
	}
	if opts.Environ == nil {
		opts.Environ = os.Environ
	}
	if opts.HomeDir == nil {
		opts.HomeDir = os.UserHomeDir
	}
	return &Supervisor{opts: opts, units: map[string]*unit{}}
}

// SetConfig 设置当前配置（F4 的 ApplyConfig 在此基础上扩展热加载语义）。
// 已在管 unit 的配置即时更新——运行中进程不受影响，下次启动生效。
func (s *Supervisor) SetConfig(cfg *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	if cfg == nil {
		return
	}
	for i := range cfg.Executors {
		if u, ok := s.units[cfg.Executors[i].Name]; ok {
			u.mu.Lock()
			u.exec = cfg.Executors[i]
			u.mu.Unlock()
		}
	}
}

// unit 返回（不存在则按配置创建）执行器的受管单元。
func (s *Supervisor) unit(name string) (*unit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("supervisor 已关闭")
	}
	if u, ok := s.units[name]; ok {
		return u, nil
	}
	if s.cfg == nil {
		return nil, fmt.Errorf("%w：%s（尚未加载配置）", ErrExecutorNotFound, name)
	}
	e := s.cfg.FindExecutor(name)
	if e == nil {
		return nil, fmt.Errorf("%w：%s", ErrExecutorNotFound, name)
	}
	u := &unit{name: name, exec: *e, state: StateStopped}
	s.units[name] = u
	return u, nil
}

// StartExecutor 人为启动执行器；已在运行/退避等待/停止中 = no-op 成功（幂等）。
// 启动失败（npx 缺失、exec 错误）返回错误并记 spawn_fail 事件，状态置 Failed。
func (s *Supervisor) StartExecutor(name string) error {
	u, err := s.unit(name)
	if err != nil {
		return err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if RunningLike(u.state) {
		return nil
	}
	if _, err := s.spawnLocked(u); err != nil {
		return err
	}
	// 启动成功：换代并启动守护循环
	u.gen++
	u.done = make(chan struct{})
	u.sleepCh = make(chan struct{})
	done := u.done
	gen := u.gen
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(done)
		s.supervise(u, gen)
	}()
	return nil
}

// spawnLocked 执行一次 spawn（须持有 u.mu）：解析 npx → 重建 argv/env →
// 启动子进程并把 stdout/stderr 接到执行器 LogSink。成功后状态置 Running。
// 事件：成功记 executor_start（debug 标记），失败记 executor_spawn_fail。
// 不负责守护循环与代数管理（人为启动与自动拉起共用本入口）。
func (s *Supervisor) spawnLocked(u *unit) (*launchcmd.Parsed, error) {
	fail := func(reason string) (*launchcmd.Parsed, error) {
		u.state = StateFailed
		s.record(logs.EventExecutorSpawnFail, u.name, "启动失败："+reason)
		return nil, fmt.Errorf("执行器 %s 启动失败：%s", u.name, reason)
	}
	npxPath, err := s.opts.ResolveNpx()
	if err != nil {
		return fail("解析 npx 失败：" + err.Error())
	}
	parsed, argv, env, err := s.buildSpawn(&u.exec)
	if err != nil {
		return fail(err.Error())
	}
	sink, err := s.opts.Logs.Sink(u.name)
	if err != nil {
		return fail("打开日志失败：" + err.Error())
	}
	// stdout/stderr 汇入执行器 LogSink（T04-F1 管道）；连接状态解析（F5）
	// 挂到同一写入路径。argv[0] 的字面 "npx" 替换为解析到的真实路径。
	cmd := newExecCmd(npxPath, argv[1:], env, sink, sink)
	if err := cmd.Start(); err != nil {
		return fail("spawn 失败：" + err.Error())
	}
	u.cmd = cmd
	u.pid = cmd.Process.Pid
	u.state = StateRunning
	u.stopping = false
	u.startedAt = s.opts.Clock.Now()
	u.conn = ConnUnknown
	u.pendingDelay = 0
	s.recordStart(u.name, parsed.Debug)
	return parsed, nil
}

// buildSpawn 从配置命令重建 argv 与子进程环境。
// 环境：全量透传 + 注入 AUTOWONDER_WORKSPACE_ROOT（真实 connect shim 会按
// 同规则重算该变量，注入仅为对齐设计措辞；按执行器覆盖的实际生效途径是
// argv 追加 --workspace-root，见任务文档变更记录）。
func (s *Supervisor) buildSpawn(e *config.Executor) (*launchcmd.Parsed, []string, []string, error) {
	parsed, err := launchcmd.Parse(e.Command)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("命令解析失败（配置应已校验）：%w", err)
	}
	argv, err := launchcmd.Argv(e.Command)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("命令重建失败：%w", err)
	}
	root, err := s.workspaceRoot(e, parsed)
	if err != nil {
		return nil, nil, nil, err
	}
	env := append([]string{}, s.opts.Environ()...)
	env = append(env, "AUTOWONDER_WORKSPACE_ROOT="+root)
	if e.WorkspaceRoot != "" {
		argv = append(argv, "--workspace-root", root)
	}
	return parsed, argv, env, nil
}

// workspaceRoot 计算工作区根目录：覆盖值（支持 ~/ 前缀展开）或默认
// ~/autowonder_workspaces_<provider>_<executor-id>（与真实 shim 缺省规则一致）。
func (s *Supervisor) workspaceRoot(e *config.Executor, p *launchcmd.Parsed) (string, error) {
	home := ""
	needHome := e.WorkspaceRoot == "" || e.WorkspaceRoot == "~" || strings.HasPrefix(e.WorkspaceRoot, "~/")
	if needHome {
		var err error
		if home, err = s.opts.HomeDir(); err != nil {
			return "", fmt.Errorf("解析家目录失败：%w", err)
		}
	}
	if e.WorkspaceRoot != "" {
		if e.WorkspaceRoot == "~" {
			return home, nil
		}
		if strings.HasPrefix(e.WorkspaceRoot, "~/") {
			return filepath.Join(home, e.WorkspaceRoot[2:]), nil
		}
		return e.WorkspaceRoot, nil
	}
	return filepath.Join(home, fmt.Sprintf("autowonder_workspaces_%s_%s", p.Provider, p.ExecutorID)), nil
}

// supervise 是单个执行器的守护循环（每次人为启动一个循环；循环内的自动
// 拉起复用本循环）。退出条件：人为停止 / 代数换代 / 不再拉起的终态。
func (s *Supervisor) supervise(u *unit, gen int) {
	for {
		u.mu.Lock()
		cur := u.cmd
		sleepCh := u.sleepCh
		u.mu.Unlock()

		err := cur.Wait()
		exitDesc := describeExit(err)

		u.mu.Lock()
		u.lastExit = exitDesc
		if u.stopping || u.gen != gen {
			// 人为停止（或已换代）：不拉起，收口 Stopped
			u.state = StateStopped
			u.pid = 0
			u.cmd = nil
			u.pendingDelay = 0
			u.mu.Unlock()
			return
		}
		// 崩溃路径（F2）
		uptime := s.opts.Clock.Now().Sub(u.startedAt)
		resetNote := ""
		if uptime >= s.opts.Backoff.StableReset {
			u.backoffStep = 0 // 稳定运行满 StableReset 后退避计数重置（design §6）
			resetNote = fmt.Sprintf("；稳定运行 %s，退避计数已重置", truncDur(uptime))
		}
		if !derefBool(u.exec.Restart.Enabled, true) {
			u.state = StateFailed
			u.pid = 0
			u.cmd = nil
			u.pendingDelay = 0
			exitDescFinal := exitDesc
			u.mu.Unlock()
			s.record(logs.EventExecutorCrash, u.name, "崩溃退出（"+exitDescFinal+"），restart.enabled=false 不拉起")
			return
		}
		delay := s.opts.Backoff.delay(u.backoffStep)
		u.backoffStep++
		u.pendingDelay = delay
		u.state = StateBackoff
		u.pid = 0
		u.cmd = nil
		u.mu.Unlock()
		s.record(logs.EventExecutorCrash, u.name, "崩溃退出（"+exitDesc+")"+resetNote)

		// 退避等待（人为停止可中断）
		select {
		case <-s.opts.Clock.After(delay):
		case <-sleepCh:
			u.mu.Lock()
			u.state = StateStopped
			u.pendingDelay = 0
			u.mu.Unlock()
			return
		}
		u.mu.Lock()
		if u.stopping || u.gen != gen || u.state != StateBackoff {
			u.mu.Unlock()
			return
		}
		if _, err := s.spawnLocked(u); err != nil {
			// 拉起失败（如 npx 被卸载）：终态 Failed，spawn_fail 事件已记录
			u.pendingDelay = 0
			u.mu.Unlock()
			return
		}
		u.restarts++
		u.pendingDelay = 0
		u.mu.Unlock()
		s.record(logs.EventExecutorRestart, u.name,
			fmt.Sprintf("第 %d 次自动拉起（退避 %s）", u.restarts, truncDur(delay)))
	}
}

// StopExecutor 人为停止执行器（整棵进程树，F3）：SIGTERM 进程组 → 宽限 →
// 强杀。已停止 = no-op 成功（幂等）；退避等待中 = 取消拉起。人为停止不触发
// 崩溃拉起。
func (s *Supervisor) StopExecutor(name string) error {
	u, err := s.unit(name)
	if err != nil {
		return err
	}
	u.mu.Lock()
	if u.state == StateStopped || u.state == StateFailed {
		u.mu.Unlock()
		return nil // 幂等
	}
	wasRunning := u.state == StateRunning || u.state == StateStarting
	u.stopping = true
	u.state = StateStopping
	pid := u.pid
	done := u.done
	if ch := u.sleepCh; ch != nil {
		close(ch)
		u.sleepCh = nil
	}
	u.mu.Unlock()

	var detail string
	if wasRunning {
		if pid > 0 {
			_ = terminateTree(pid)
		}
		if waitOrTimeout(done, s.opts.Backoff.Grace) {
			detail = "人为停止（优雅退出，" + s.lastExitOf(u) + "）"
		} else {
			_ = killTree(pid)
			waitOrTimeout(done, 3*time.Second)
			detail = "人为停止（宽限超时强杀，" + s.lastExitOf(u) + "）"
		}
	} else {
		// 退避等待中被停止：等守护循环收口（防御性限时）
		waitOrTimeout(done, 5*time.Second)
		detail = "人为停止（退避等待中取消拉起）"
	}
	u.mu.Lock()
	// 仅在仍是本次停止的语境下收口（期间若被重新启动则不动：startLocked 已
	// 清 stopping 并置 Running）
	if u.stopping {
		u.state = StateStopped
		u.pid = 0
		u.cmd = nil
		u.pendingDelay = 0
	}
	u.mu.Unlock()
	s.record(logs.EventExecutorStop, u.name, detail)
	return nil
}

// RestartExecutor 重启执行器（先停后起；未运行时等价于启动）。
func (s *Supervisor) RestartExecutor(name string) error {
	st, ok := s.SnapshotOne(name)
	if !ok {
		return fmt.Errorf("%w：%s", ErrExecutorNotFound, name)
	}
	if RunningLike(st.State) {
		if err := s.StopExecutor(name); err != nil {
			return err
		}
	}
	return s.StartExecutor(name)
}

// Shutdown 停止全部执行器并等待守护循环退出（daemon 退出时调用）。
func (s *Supervisor) Shutdown() {
	s.mu.Lock()
	s.closed = true
	names := make([]string, 0, len(s.units))
	for name := range s.units {
		names = append(names, name)
	}
	s.mu.Unlock()
	for _, name := range names {
		_ = s.StopExecutor(name)
	}
	s.wg.Wait()
}

// UnitStatus 是单执行器的本地状态快照（F5；CLI status 与 Web 总览共用）。
type UnitStatus struct {
	Name           string
	State          State
	PID            int
	StartedAt      time.Time // 当前进程启动时刻（零值=未运行）
	Uptime         time.Duration
	Restarts       int           // 自动拉起累计次数
	BackoffDelay   time.Duration // Backoff 态下当前等待的退避间隔，其余为 0
	Conn           ConnState     // 连接状态（日志解析）
	LastExit       string        // 上次退出描述
	Autostart      bool
	RestartEnabled bool
}

// Snapshot 返回全部执行器状态快照（按配置顺序）。
func (s *Supervisor) Snapshot() []UnitStatus {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if cfg == nil {
		return nil
	}
	out := make([]UnitStatus, 0, len(cfg.Executors))
	for i := range cfg.Executors {
		if st, ok := s.SnapshotOne(cfg.Executors[i].Name); ok {
			out = append(out, st)
		}
	}
	return out
}

// SnapshotOne 返回单执行器状态快照；配置里没有该执行器时 ok=false。
func (s *Supervisor) SnapshotOne(name string) (UnitStatus, bool) {
	s.mu.Lock()
	u := s.units[name]
	cfg := s.cfg
	s.mu.Unlock()
	var e *config.Executor
	if cfg != nil {
		e = cfg.FindExecutor(name)
	}
	if e == nil {
		return UnitStatus{}, false
	}
	if u == nil {
		return UnitStatus{
			Name: name, State: StateStopped,
			Autostart: e.Autostart, RestartEnabled: derefBool(e.Restart.Enabled, true),
		}, true
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	st := UnitStatus{
		Name:           name,
		State:          u.state,
		PID:            u.pid,
		StartedAt:      u.startedAt,
		Restarts:       u.restarts,
		BackoffDelay:   u.pendingDelay,
		Conn:           u.conn,
		LastExit:       u.lastExit,
		Autostart:      e.Autostart,
		RestartEnabled: derefBool(u.exec.Restart.Enabled, true),
	}
	if u.state == StateRunning || u.state == StateStopping {
		st.Uptime = s.opts.Clock.Now().Sub(u.startedAt)
	}
	return st, true
}

// record 记录管理事件（事件日志不可用时不阻塞主流程）。
func (s *Supervisor) record(typ logs.EventType, name, detail string) {
	if el, err := s.opts.Logs.EventLog(); err == nil {
		_ = el.Record(typ, name, detail)
	}
}

// recordStart 记录启动事件（含 --debug 标记，复用 T04-F5 的封装）。
func (s *Supervisor) recordStart(name string, debug bool) {
	if el, err := s.opts.Logs.EventLog(); err == nil {
		_ = el.RecordExecutorStart(name, debug)
	}
}

// lastExitOf 读取上次退出描述（停止事件详情用）。
func (s *Supervisor) lastExitOf(u *unit) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastExit
}

// describeExit 把 cmd.Wait 的结果转为人可读的退出描述
// （ExitCode()<0 在 Unix 表示被信号终止）。
func describeExit(err error) string {
	if err == nil {
		return "exit 0"
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return fmt.Sprintf("exit %d", code)
		}
		return "被信号终止"
	}
	return "异常退出：" + err.Error()
}

// waitOrTimeout 等待 done，d 内未完成返回 false。
func waitOrTimeout(done <-chan struct{}, d time.Duration) bool {
	if done == nil {
		return false
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-done:
		return true
	case <-t.C:
		return false
	}
}

// derefBool 解引用 *bool（nil 取默认）。
func derefBool(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}

// truncDur 截断到毫秒（事件/快照里的时长展示）。
func truncDur(d time.Duration) time.Duration { return d.Truncate(time.Millisecond) }
