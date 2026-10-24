// logpipe.go：日志写入路径上的行切分管道（T05-F5）。字节流原样进 LogSink，
// 同时按行回调（连接状态解析挂在同一写入路径上，行级开销可控）。
package supervisor

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// linePipe 把写入的字节流按 \n 切行后回调 onLine；并发安全（stdout/stderr
// 两条管道各自独立，共享的 LogSink 本身线程安全）。
type linePipe struct {
	mu     sync.Mutex
	w      io.Writer
	onLine func(string)
	buf    []byte
}

// newLinePipe 创建行管道：写入 w 的同时逐行回调。
func newLinePipe(w io.Writer, onLine func(string)) *linePipe {
	return &linePipe{w: w, onLine: onLine}
}

// Write 落盘并切行回调；返回值与底层写入一致（回调失败不影响写入）。
func (p *linePipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n, err := p.w.Write(b)
	p.buf = append(p.buf, b[:n]...)
	for {
		idx := bytes.IndexByte(p.buf, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSuffix(string(p.buf[:idx]), "\r")
		p.buf = p.buf[idx+1:]
		if p.onLine != nil {
			p.onLine(line)
		}
	}
	return n, err
}

// close 冲刷残留的半行（进程退出后最后一段无换行日志），并停止缓冲。
func (p *linePipe) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.buf) > 0 && p.onLine != nil {
		p.onLine(string(p.buf))
	}
	p.buf = nil
	p.onLine = nil
}
