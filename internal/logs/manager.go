// manager.go：日志管理器（T04-F6）。汇总 sink 注册表、事件日志、
// 保留清理与读取接口（Tail/Follow/ReadLines），供 CLI `awem log`（T07）
// 与 Web WS（T06/T09）共用。
package logs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// followPollInterval 是 Follow 轮询周期（滚动检测与新行推送的延迟上界）。
const followPollInterval = 100 * time.Millisecond

// Manager 管理全部执行器日志 sink、事件日志与清理。
type Manager struct {
	mu            sync.Mutex
	executorsDir  string
	eventsPath    string
	maxBytes      int64
	keep          int
	retentionDays int

	sinks    map[string]*LogSink
	eventLog *EventLog
}

// NewManager 创建日志管理器。executorsDir 为 data/logs/executors，
// eventsPath 为 data/logs/events.log；maxBytes/keep 为滚动参数，
// retentionDays 为保留天数（<=0 取各自默认值）。
func NewManager(executorsDir, eventsPath string, maxBytes int64, keep, retentionDays int) *Manager {
	return &Manager{
		executorsDir:  executorsDir,
		eventsPath:    eventsPath,
		maxBytes:      maxBytes,
		keep:          keep,
		retentionDays: retentionDays,
		sinks:         map[string]*LogSink{},
	}
}

// Sink 返回执行器日志写入器（不存在则打开；单例复用）。
func (m *Manager) Sink(name string) (*LogSink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sinks[name]; ok {
		return s, nil
	}
	s, err := OpenSink(m.executorPath(name), m.maxBytes, m.keep)
	if err != nil {
		return nil, err
	}
	m.sinks[name] = s
	return s, nil
}

// CloseSink 关闭指定执行器的写入器（执行器删除时调用）。
func (m *Manager) CloseSink(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sinks[name]; ok {
		s.Close()
		delete(m.sinks, name)
	}
}

// Purge 关闭写入器并删除该执行器全部日志文件（当前 + 滚动链）。
func (m *Manager) Purge(name string) error {
	m.CloseSink(name)
	matches, _ := filepath.Glob(filepath.Join(m.executorsDir, name+".log*"))
	for _, f := range matches {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除日志 %s 失败：%w", f, err)
		}
	}
	return nil
}

// EventLog 返回事件日志（惰性打开，单例）。
func (m *Manager) EventLog() (*EventLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.eventLog != nil {
		return m.eventLog, nil
	}
	el, err := OpenEventLog(m.eventsPath, m.maxBytes, m.keep)
	if err != nil {
		return nil, err
	}
	m.eventLog = el
	return el, nil
}

// Cleanup 执行保留策略清理（执行器目录 + 事件日志目录）并记录清理事件。
func (m *Manager) Cleanup(now time.Time) (int, error) {
	total := 0
	for _, dir := range []string{m.executorsDir, filepath.Dir(m.eventsPath)} {
		removed, err := CleanupExpired(dir, m.retentionDays, now, nil)
		if err != nil {
			return total, err
		}
		total += len(removed)
	}
	if total > 0 {
		if el, err := m.EventLog(); err == nil {
			_ = el.Record(EventLogCleanup, "", fmt.Sprintf("清理 %d 个过期日志文件", total))
		}
	}
	return total, nil
}

// Close 关闭全部写入器与事件日志（daemon 退出时调用）。
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sinks {
		s.Close()
	}
	m.sinks = map[string]*LogSink{}
	if m.eventLog != nil {
		m.eventLog.Close()
		m.eventLog = nil
	}
}

// executorPath 返回执行器日志文件路径。
func (m *Manager) executorPath(name string) string {
	return filepath.Join(m.executorsDir, name+".log")
}

// Tail 返回当前日志文件的尾部 N 行（不足 N 返回全部）。
// 文件不存在返回 (nil, nil)——明确空态信号，CLI 显示空、Web 空状态。
func (m *Manager) Tail(name string, n int) ([]string, error) {
	if n <= 0 {
		n = 50
	}
	lines, err := readAllLines(m.executorPath(name))
	if err != nil {
		return nil, err
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// ReadLines 历史分页读取（当前文件）：返回从行偏移 offset 开始的 limit 行
// 与总行数（Web 详情页向上翻页用；翻页策略：offset -= limit）。
func (m *Manager) ReadLines(name string, offset, limit int) ([]string, int, error) {
	if limit <= 0 {
		limit = 100
	}
	lines, err := readAllLines(m.executorPath(name))
	if err != nil {
		return nil, 0, err
	}
	total := len(lines)
	if offset < 0 || offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return lines[offset:end], total, nil
}

// readAllLines 读取整个文件按行切分（不含换行符）；文件不存在返回空。
func readAllLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("打开日志 %s 失败：%w", path, err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // 单行上限 4MB
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

// Subscription 是 Follow 订阅句柄：C 推送新行（不含换行符），Close 停止。
type Subscription struct {
	C      <-chan string
	name   string
	cancel func()
	done   chan struct{}
}

// Close 停止订阅并释放资源（幂等）。
func (s *Subscription) Close() {
	if s.cancel != nil {
		s.cancel()
		<-s.done
		s.cancel = nil
	}
}

// Follow 订阅执行器日志的新行推送：从订阅时刻的文件末尾开始；
// 滚动切换后自动跟随（沿编号链回放，写入快导致连滚多次也不漏行）。
// 文件不存在不报错（出现后自动开始）。
func (m *Manager) Follow(name string) (*Subscription, error) {
	path := m.executorPath(name)
	out := make(chan string, 256)
	stop := make(chan struct{})
	done := make(chan struct{})
	sub := &Subscription{C: out, name: name, cancel: func() { close(stop) }, done: done}

	go func() {
		defer close(done)
		defer close(out)
		var (
			f        *os.File
			lastSize int64
			buf      string // 跨读取的半行缓冲
		)
		openAt := func(target string, offset int64) {
			if f != nil {
				f.Close()
			}
			f = nil
			file, err := os.Open(target)
			if err != nil {
				return
			}
			file.Seek(offset, 0)
			f = file
			lastSize = offset
		}
		emit := func(data []byte) {
			buf += string(data)
			for {
				idx := strings.IndexByte(buf, '\n')
				if idx < 0 {
					break
				}
				line := strings.TrimSuffix(buf[:idx], "\r")
				buf = buf[idx+1:]
				select {
				case out <- line:
				case <-stop:
					return
				}
			}
		}

		ticker := time.NewTicker(followPollInterval)
		defer ticker.Stop()
		opened := false
		for {
			select {
			case <-stop:
				if f != nil {
					f.Close()
				}
				return
			case <-ticker.C:
			}
			pstat, err := os.Stat(path)
			if err != nil {
				// 文件暂不存在（首次或滚动间隙）：等待出现
				if f != nil {
					f.Close()
					f = nil
				}
				continue
			}
			if f == nil {
				if opened {
					openAt(path, 0) // 滚动后新文件：从 0 读
				} else {
					openAt(path, pstat.Size()) // 首次出现：从末尾开始（只推新行）
				}
				opened = true
				if f == nil {
					continue
				}
			}
			fstat, err := f.Stat()
			if err != nil {
				f.Close()
				f = nil
				continue
			}
			// 先排空当前 fd 上的新数据（rename 后 fd 仍指向旧文件，内容仍可读）
			if fstat.Size() > lastSize {
				chunk := make([]byte, fstat.Size()-lastSize)
				n, _ := f.Read(chunk)
				if n > 0 {
					lastSize += int64(n)
					emit(chunk[:n])
				}
			}
			// fd 读尽且当前 path 已不是同一文件 → 滚动发生（SameFile 按 inode
			// 判定，比尺寸回缩可靠）。写入快时可能已连滚多次，沿编号链回放：
			// 定位 fd 文件当前链位 k，下一个读 path.(k-1)（k==1 时读当前 path）。
			if lastSize >= fstat.Size() && !os.SameFile(pstat, fstat) {
				openAt(nextInChain(path, fstat), 0)
			}
		}
	}()
	return sub, nil
}

// nextInChain 返回滚动后应接着读的文件：在编号链 path.1/path.2/… 中定位
// fstat（已读尽文件的 inode）当前所在位置 k，返回 path.(k-1)；k==1 或
// 定位失败（文件被保留策略删除等）返回当前 path。
func nextInChain(path string, fstat os.FileInfo) string {
	matches, _ := filepath.Glob(path + ".*")
	best := 0
	for _, m := range matches {
		suffix := strings.TrimPrefix(m, path+".")
		k, err := strconv.Atoi(suffix)
		if err != nil || k < 1 {
			continue
		}
		if st, err := os.Stat(m); err == nil && os.SameFile(st, fstat) {
			best = k
		}
	}
	if best <= 1 {
		return path
	}
	return fmt.Sprintf("%s.%d", path, best-1)
}

// ListRotated 返回某执行器的滚动文件序号列表（诊断用，升序）。
func (m *Manager) ListRotated(name string) []int {
	matches, _ := filepath.Glob(filepath.Join(m.executorsDir, name+".log.*"))
	var idx []int
	for _, f := range matches {
		suffix := strings.TrimPrefix(filepath.Base(f), name+".log.")
		if n, err := strconv.Atoi(suffix); err == nil {
			idx = append(idx, n)
		}
	}
	sort.Ints(idx)
	return idx
}
