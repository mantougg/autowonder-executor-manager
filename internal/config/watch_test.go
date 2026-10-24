package config

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// waitFor 轮询等待条件成立，超时 fatal。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

// TestReload成功发布新配置 改文件→重载→订阅者收到。
func TestReload成功发布新配置(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	c0 := Default()
	c0.Server.Port = 100
	if err := Save(path, c0); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, c0)

	var mu sync.Mutex
	var got []Config
	w.Subscribe(func(c Config) { mu.Lock(); got = append(got, c); mu.Unlock() })

	c1 := c0
	c1.Server.Port = 200
	if err := Save(path, c1); err != nil {
		t.Fatal(err)
	}
	w.Reload()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Server.Port != 200 {
		t.Fatalf("订阅者应收到 port=200 的新配置，实际 %+v", got)
	}
	if w.Current().Server.Port != 200 {
		t.Fatalf("Current 应更新为 200，实际 %d", w.Current().Server.Port)
	}
}

// TestReload坏配置保留旧配置 写坏 TOML → 旧配置仍在、错误事件产生。
func TestReload坏配置保留旧配置(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	c0 := Default()
	c0.Server.Port = 100
	if err := Save(path, c0); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, c0)

	var mu sync.Mutex
	var errs []error
	w.OnError(func(err error) { mu.Lock(); errs = append(errs, err); mu.Unlock() })

	// 语法错误
	if err := os.WriteFile(path, []byte("[server 未闭合"), 0o600); err != nil {
		t.Fatal(err)
	}
	w.Reload()
	mu.Lock()
	if len(errs) != 1 {
		mu.Unlock()
		t.Fatalf("语法错误应产生 1 条错误事件，实际 %d", len(errs))
	}
	mu.Unlock()
	if w.Current().Server.Port != 100 {
		t.Fatal("语法错误后旧配置应保留")
	}

	// 校验错误（bind 非 127.0.0.1）
	cBad := c0
	cBad.Server.Bind = "0.0.0.0"
	if err := Save(path, cBad); err != nil {
		t.Fatal(err)
	}
	w.Reload()
	mu.Lock()
	defer mu.Unlock()
	if len(errs) != 2 {
		t.Fatalf("校验错误应再产生 1 条错误事件，实际 %d", len(errs))
	}
	if w.Current().Server.Port != 100 {
		t.Fatal("校验错误后旧配置应保留")
	}
}

// Test去抖多次事件只触发一次重载 用假事件通道注入连续写事件。
func Test去抖多次事件只触发一次重载(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	c0 := Default()
	if err := Save(path, c0); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, c0)
	w.debounce = 150 * time.Millisecond

	events := make(chan fsnotify.Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.watchLoop(ctx, events, nil)

	// 静默窗口内连续 5 个写事件（模拟编辑器写一半 + 重复触发）
	for i := 0; i < 5; i++ {
		events <- fsnotify.Event{Name: path, Op: fsnotify.Write}
		time.Sleep(30 * time.Millisecond)
	}
	// 超过去抖窗口后应恰好重载一次
	time.Sleep(400 * time.Millisecond)
	if n := w.ReloadCount(); n != 1 {
		t.Fatalf("去抖后应只重载 1 次，实际 %d 次", n)
	}
	// 再来一个事件 → 再重载一次
	events <- fsnotify.Event{Name: path, Op: fsnotify.Write}
	time.Sleep(400 * time.Millisecond)
	if n := w.ReloadCount(); n != 2 {
		t.Fatalf("新事件应再次重载，共 %d 次，期望 2", n)
	}
}

// Test同目录其他文件事件被忽略 Save 的 .tmp 事件不触发重载。
func Test同目录其他文件事件被忽略(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	c0 := Default()
	if err := Save(path, c0); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, c0)
	w.debounce = 100 * time.Millisecond

	events := make(chan fsnotify.Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.watchLoop(ctx, events, nil)

	events <- fsnotify.Event{Name: path + ".tmp", Op: fsnotify.Write}
	events <- fsnotify.Event{Name: filepath.Join(dir, "别的文件"), Op: fsnotify.Write}
	time.Sleep(300 * time.Millisecond)
	if n := w.ReloadCount(); n != 0 {
		t.Fatalf("非目标文件事件不应触发重载，实际 %d 次", n)
	}
}

// Test真实文件监视端到端 真正走 Start + fsnotify：改文件→订阅者收到新配置。
func Test真实文件监视端到端(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	c0 := Default()
	c0.Server.Port = 100
	if err := Save(path, c0); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, c0)
	w.debounce = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()
	// 等待 fw.Add 注册完成（Start 在 goroutine 内做初始化，立即写文件可能错过首批事件）
	time.Sleep(300 * time.Millisecond)

	var mu sync.Mutex
	var got []Config
	var errs []error
	w.Subscribe(func(c Config) { mu.Lock(); got = append(got, c); mu.Unlock() })
	w.OnError(func(err error) { mu.Lock(); errs = append(errs, err); mu.Unlock() })

	// 原子替换写（与 Save 一致）
	c1 := c0
	c1.Server.Port = 300
	if err := Save(path, c1); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 1 && got[len(got)-1].Server.Port == 300
	})

	// 坏配置：旧配置仍在 + 错误事件
	if err := os.WriteFile(path, []byte("坏 toml ["), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(errs) >= 1
	})
	if w.Current().Server.Port != 300 {
		t.Fatalf("坏配置后旧配置应保留 port=300，实际 %d", w.Current().Server.Port)
	}

	cancel()
	// Start 应随 ctx 退出且无错误
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start 返回错误：%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start 未随 ctx 取消退出")
	}
}
