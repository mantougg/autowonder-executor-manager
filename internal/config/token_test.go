package config

import (
	"os"
	"regexp"
	"runtime"
	"testing"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TestToken首次生成 长度（64 hex = 32 字节熵）与随机性（两次生成不同）。
func TestToken首次生成(t *testing.T) {
	p1 := Layout(t.TempDir())
	p2 := Layout(t.TempDir())
	tok1, err := APIToken(p1)
	if err != nil {
		t.Fatalf("APIToken 失败：%v", err)
	}
	tok2, err := APIToken(p2)
	if err != nil {
		t.Fatalf("APIToken 失败：%v", err)
	}
	if !hex64.MatchString(tok1) || !hex64.MatchString(tok2) {
		t.Fatalf("token 应为 64 位 hex（32 字节熵），得到 %q / %q", tok1, tok2)
	}
	if tok1 == tok2 {
		t.Fatal("两次生成的 token 相同，随机性不足")
	}
	// 落盘内容与返回值一致
	data, err := os.ReadFile(p1.TokenFile)
	if err != nil {
		t.Fatalf("token 文件未落盘：%v", err)
	}
	if string(data) != tok1 {
		t.Fatalf("落盘内容 %q 与返回值 %q 不一致", data, tok1)
	}
}

// TestToken二次读取复用 已存在则直接读，不重新生成。
func TestToken二次读取复用(t *testing.T) {
	p := Layout(t.TempDir())
	if err := os.MkdirAll(p.DataDir, dirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.TokenFile, []byte("preset-token-123"), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := APIToken(p)
	if err != nil {
		t.Fatalf("APIToken 失败：%v", err)
	}
	if tok != "preset-token-123" {
		t.Fatalf("应复用已有 token，得到 %q", tok)
	}
}

// TestToken权限0600 非 Windows 下断言文件权限。
func TestToken权限0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 走 icacls 分支，真机验证记 T10")
	}
	p := Layout(t.TempDir())
	if _, err := APIToken(p); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p.TokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token 文件权限 = %o，期望 0600", info.Mode().Perm())
	}
}

// TestToken空文件重新生成 存在但为空时视为未生成。
func TestToken空文件重新生成(t *testing.T) {
	p := Layout(t.TempDir())
	if err := os.MkdirAll(p.DataDir, dirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.TokenFile, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := APIToken(p)
	if err != nil {
		t.Fatal(err)
	}
	if !hex64.MatchString(tok) {
		t.Fatalf("空文件应触发生成，得到 %q", tok)
	}
}
