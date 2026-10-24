package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestSink写入与路径约定 文件出现在约定路径，内容完整可读（§5.1 布局由调用方按
// config.Paths 拼出，这里验证 sink 落盘行为）。
func TestSink写入与路径约定(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "executors", "e1.log")
	s, err := OpenSink(path, 0, 0) // 不滚动
	if err != nil {
		t.Fatalf("OpenSink 失败：%v", err)
	}
	line := "hello 执行器日志\n"
	if n, err := s.Write([]byte(line)); err != nil || n != len(line) {
		t.Fatalf("写入失败：%v（n=%d）", err, n)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("文件未出现在约定路径 %s：%v", path, err)
	}
	if string(data) != line {
		t.Fatalf("内容不符：%q", data)
	}
	// 再次打开应追加（不截断历史）
	s2, err := OpenSink(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	s2.Write([]byte(line))
	s2.Close()
	data2, _ := os.ReadFile(path)
	if len(data2) != 2*len(line) {
		t.Fatalf("重开应追加，当前 %d 字节", len(data2))
	}
}

// TestSink并发写无撕裂 8 goroutine 各写 50 行，总行数完整且无交错撕裂行。
func TestSink并发写无撕裂(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenSink(filepath.Join(dir, "e.log"), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	const workers, perWorker = 8, 50
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				payload := fmt.Sprintf("w%d-line-%04d-padding-padding-padding\n", w, i)
				if _, err := s.Write([]byte(payload)); err != nil {
					t.Errorf("并发写失败：%v", err)
				}
			}
		}(w)
	}
	wg.Wait()
	s.Close()

	data, _ := os.ReadFile(filepath.Join(dir, "e.log"))
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != workers*perWorker {
		t.Fatalf("总行数 = %d，期望 %d", len(lines), workers*perWorker)
	}
	// 每行格式完整（无撕裂）：匹配 wN-line-NNNN-padding-padding-padding
	for _, ln := range lines {
		var w, i int
		if n, _ := fmt.Sscanf(ln, "w%d-line-%04d-padding-padding-padding", &w, &i); n != 2 {
			t.Fatalf("撕裂行：%q", ln)
		}
	}
}

// TestSink关闭后再写 Close 后 Write 返回错误不 panic；重复 Close 幂等。
func TestSink关闭后再写(t *testing.T) {
	s, err := OpenSink(filepath.Join(t.TempDir(), "e.log"), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("重复 Close 应幂等：%v", err)
	}
	if _, err := s.Write([]byte("x")); err == nil {
		t.Fatal("关闭后 Write 应返回错误")
	}
	// 不 panic 即通过
}
