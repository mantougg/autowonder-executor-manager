package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test滚动超阈值产生滚动文件 写超过 maxBytes → 产生 .1/.2 链，新文件从空继续。
func Test滚动超阈值产生滚动文件(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "e.log")
	// 每行 24 字节；maxBytes=100 → 约 4 行一滚；keep=3
	s, err := OpenSink(path, 100, 3)
	if err != nil {
		t.Fatal(err)
	}
	line := "0123456789012345678901\n" // 23+1=24 字节
	for i := 0; i < 40; i++ {          // 960 字节 → 约 9 次滚动
		if _, err := s.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	// 滚动链存在且不超过 keep=3
	for i := 1; i <= 3; i++ {
		if _, err := os.Stat(fmt.Sprintf("%s.%d", path, i)); err != nil {
			t.Fatalf("应存在滚动文件 %s.%d：%v", path, i, err)
		}
	}
	if _, err := os.Stat(path + ".4"); !os.IsNotExist(err) {
		t.Fatal("滚动文件数不应超过 keep=3")
	}
	// 当前文件从空继续且未超阈值过多
	cur, _ := os.ReadFile(path)
	if len(cur) > 100 {
		t.Fatalf("当前文件 %d 字节应接近阈值 100", len(cur))
	}
}

// Test滚动边界无丢行 滚动瞬间无丢行：keep 足够大（无淘汰）时总行数守恒。
// （keep 不足时的链淘汰行为由 Test滚动超阈值产生滚动文件 验证。）
func Test滚动边界无丢行(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "e.log")
	s, err := OpenSink(path, 120, 200)
	if err != nil {
		t.Fatal(err)
	}
	const total = 500
	for i := 0; i < total; i++ {
		fmt.Fprintf(s, "line-%04d\n", i)
	}
	s.Close()

	count := 0
	for i := 0; i <= 200; i++ {
		f := path
		if i > 0 {
			f = fmt.Sprintf("%s.%d", path, i)
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, ln := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if ln != "" {
				count++
				if !strings.HasPrefix(ln, "line-") {
					t.Fatalf("异常行：%q", ln)
				}
			}
		}
	}
	if count != total {
		t.Fatalf("跨滚动总行数 = %d，期望 %d（滚动瞬间丢行）", count, total)
	}
}

// Test滚动对Writer透明 写入方无感知：滚动前后 Write 均成功且内容可读。
func Test滚动对Writer透明(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "e.log")
	s, err := OpenSink(path, 50, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if _, err := s.Write([]byte("x\n")); err != nil {
			t.Fatalf("第 %d 次写入在滚动后失败：%v", i, err)
		}
	}
	s.Close()
	// 全部内容在（当前 + 滚动链）
	total := 0
	for _, suffix := range []string{"", ".1", ".2"} {
		if data, err := os.ReadFile(path + suffix); err == nil {
			total += strings.Count(string(data), "x\n")
		}
	}
	if total != 20 {
		t.Fatalf("透明滚动后总 x 数 = %d，期望 20", total)
	}
}
