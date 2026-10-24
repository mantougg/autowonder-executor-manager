// watch.go 实现 config.toml 热加载：文件监视 + 去抖 + 原子重读校验。
// 语义（design.md §5.1/§4）：
//   - 变更 → 重读 + 校验，成功则把新配置发布给订阅者（supervisor/api 在 T05/T06 订阅）；
//   - 失败（语法/校验错）→ 保留旧内存配置继续运行，并向错误回调报告一条事件
//     （T04 事件日志完成后接入，当前默认打到 stderr）；
//   - 编辑器"写一半"与重复触发去抖（默认 200ms 静默窗口）。
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultDebounce 是写事件去抖的静默窗口。
const DefaultDebounce = 200 * time.Millisecond

// Watcher 监视 config.toml 变更并发布校验通过的新配置。
type Watcher struct {
	path     string
	debounce time.Duration

	mu      sync.RWMutex
	cur     Config
	subs    []func(Config)  // 配置变更订阅者
	onErr   []func(error)   // 错误事件回调（热加载失败等）

	reloadCount     int // 测试观测用：实际执行重载的次数（含被跳过发布的）
	publishedCount  int // 测试观测用：实际发布给订阅者的次数
}

// NewWatcher 创建监视器；initial 为当前内存中的配置（热加载失败的兜底）。
func NewWatcher(path string, initial Config) *Watcher {
	return &Watcher{path: path, debounce: DefaultDebounce, cur: initial}
}

// Subscribe 注册配置变更订阅者（新配置已通过校验）。
func (w *Watcher) Subscribe(fn func(Config)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.subs = append(w.subs, fn)
}

// OnError 注册错误事件回调（T04 完成后由事件日志统一订阅）。
func (w *Watcher) OnError(fn func(error)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onErr = append(w.onErr, fn)
}

// Current 返回当前生效配置（热加载失败时仍是旧配置）。
func (w *Watcher) Current() Config {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cur
}

// ReloadCount 返回已执行的重载次数（测试观测用）。
func (w *Watcher) ReloadCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.reloadCount
}

// PublishedCount 返回实际发布给订阅者的重载次数（测试观测用；
// 与 ReloadCount 之差即"内容未变被跳过"的次数）。
func (w *Watcher) PublishedCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.publishedCount
}

// SetCurrent 覆盖当前内存配置（不经校验、不发布订阅）。供 daemon 的
// "自我写盘"路径（端口写回，T06 F2）同步内存用：写回后磁盘与该内存值
// 一致，后续文件监视触发的重载会被"内容未变"检查跳过，不产生多余的
// config_changed 事件。外部变更仍走 Reload 正常发布。
func (w *Watcher) SetCurrent(c Config) {
	w.mu.Lock()
	w.cur = c
	w.mu.Unlock()
}

// reportError 上报错误事件；无注册回调时打到 stderr。
func (w *Watcher) reportError(err error) {
	w.mu.RLock()
	handlers := append([]func(error){}, w.onErr...)
	w.mu.RUnlock()
	if len(handlers) == 0 {
		fmt.Fprintln(os.Stderr, "[awem] "+err.Error())
		return
	}
	for _, fn := range handlers {
		fn(err)
	}
}

// Reload 重读并校验配置：成功且内容有变 → 发布给订阅者；成功但与当前
// 内存配置等价（内容未变）→ 跳过发布（T06 F2：端口写回引发的自触发
// 热加载由此抑制，不产生多余的 config_changed 事件与 Supervisor 对齐）；
// 失败 → 保留旧配置并报告错误事件。
func (w *Watcher) Reload() {
	cfg, err := Load(w.path)
	if err == nil {
		err = cfg.Validate()
	}
	w.mu.Lock()
	w.reloadCount++
	w.mu.Unlock()
	if err != nil {
		w.reportError(fmt.Errorf("配置热加载失败，继续使用旧配置（%s）：%w", w.path, err))
		return
	}
	w.mu.Lock()
	if reflect.DeepEqual(cfg, w.cur) {
		w.mu.Unlock()
		return
	}
	w.cur = cfg
	w.publishedCount++
	subs := append([]func(Config){}, w.subs...)
	w.mu.Unlock()
	for _, fn := range subs {
		fn(cfg)
	}
}

// Start 启动文件监视（阻塞直到 ctx 取消，通常 go 起 goroutine 跑）。
// 监视 config.toml 所在目录而非文件本身：Save 走临时文件+rename 原子替换，
// 直接监视文件会在替换后丢失 inode。按文件名过滤事件。
func (w *Watcher) Start(ctx context.Context) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("创建文件监视器失败：%w", err)
	}
	defer fw.Close()
	if err := fw.Add(filepath.Dir(w.path)); err != nil {
		return fmt.Errorf("监视配置目录失败：%w", err)
	}
	w.watchLoop(ctx, fw.Events, fw.Errors)
	return nil
}

// watchLoop 事件主循环：同目录事件按 basename 过滤，写类事件去抖后重载。
// 独立出来便于单测注入假事件通道。
func (w *Watcher) watchLoop(ctx context.Context, events <-chan fsnotify.Event, errs <-chan error) {
	target := filepath.Base(w.path)
	var quiet <-chan time.Time
	var timer *time.Timer
	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case err, ok := <-errs:
			if !ok {
				return
			}
			w.reportError(fmt.Errorf("配置文件监视出错：%w", err))
		case ev, ok := <-events:
			if !ok {
				return
			}
			if filepath.Base(ev.Name) != target {
				continue // 忽略同目录其他文件（含 Save 的 .tmp）
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue // 只关心会改变内容的操作
			}
			// 触发/重置去抖计时器：静默窗口内不再有事件才重载
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(w.debounce)
			quiet = timer.C
		case <-quiet:
			quiet = nil
			w.Reload()
		}
	}
}
