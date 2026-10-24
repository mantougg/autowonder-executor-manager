// sink.go：执行器日志收割管道（T04-F1）。
// 每执行器一个 LogSink（data/logs/executors/<name>.log），实现 io.Writer：
// Supervisor（T05）把子进程 stdout/stderr 汇入同一写入器。
package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// LogSink 是单个执行器日志文件的线程安全写入器。
// 缓冲策略：直写无用户态缓冲——收割管道以块/行为单位调用 Write，
// 每次调用即落盘，保证 tail/跟随读取的及时性；代价是每次 Write 一次
// 系统调用 + 一把锁，对执行器日志量（KB/s 级）完全可接受。
type LogSink struct {
	mu     sync.Mutex
	path   string
	f      *os.File
	size   int64 // 当前文件已写字节
	closed bool

	// 滚动参数（T04-F2 起生效）
	maxBytes int64 // 单文件上限，<=0 表示不滚动
	keep     int   // 滚动保留个数（<name>.log.1 … <name>.log.keep）
}

// OpenSink 打开（不存在则创建）执行器日志文件。
// maxBytes/keep 为滚动参数，keep<=0 时用默认值。
func OpenSink(path string, maxBytes int64, keep int) (*LogSink, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败：%w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件 %s 失败：%w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("获取日志文件大小失败：%w", err)
	}
	if keep <= 0 {
		keep = 5
	}
	return &LogSink{path: path, f: f, size: info.Size(), maxBytes: maxBytes, keep: keep}, nil
}

// Write 实现 io.Writer：线程安全，多 goroutine 并发写不交错撕裂。
// 文件达到上限时透明滚动（F2）。
func (s *LogSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, fmt.Errorf("日志写入器已关闭（%s）", s.path)
	}
	if s.maxBytes > 0 && s.size+int64(len(p)) > s.maxBytes {
		s.rotateLocked()
		// 滚动失败时 rotateLocked 内部已尽力恢复原文件，写入继续并透出底层错误
	}
	n, err := s.f.Write(p)
	s.size += int64(n)
	return n, err
}

// Close 关闭文件（执行器删除 --purge 或 daemon 退出时调用）。
// Close 后再 Write 返回错误不 panic；重复 Close 幂等。
func (s *LogSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.f.Close()
}

// Path 返回日志文件路径。
func (s *LogSink) Path() string { return s.path }

// rotateLocked 执行滚动（须持有 s.mu）：<name>.log → <name>.log.1，
// 旧链依次后移，超出 keep 的删除。编号链方案（非时间戳）：
// 顺序明确、与保留个数/天数策略联动简单。
// 任何一步失败都保证 s.f 可继续写入（重开原路径），否则后续 Write 直接报错。
func (s *LogSink) rotateLocked() {
	for i := s.keep - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", s.path, i)
		dst := fmt.Sprintf("%s.%d", s.path, i+1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst) // 最旧的被覆盖即删除
		}
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", s.path, s.keep+1)) // 兜底清理更旧的
	_ = s.f.Close()
	_ = os.Rename(s.path, s.path+".1") // 原文件不存在（外部删过）也不阻塞
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		// 极端情况（磁盘/权限问题）：再按追加模式兜底重开一次
		f, err = os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return // s.f 已关闭，后续 Write 将返回错误，由调用方感知
		}
	}
	s.f = f
	s.size = 0
}
