package exitcode

import (
	"errors"
	"fmt"
	"testing"
)

// Test常量与设计表一致 确保 design.md §8.2 的 6 个退出码不被意外改动。
func Test常量与设计表一致(t *testing.T) {
	cases := []struct {
		got  int
		want int
	}{
		{Success, 0},
		{General, 1},
		{Usage, 2},
		{NotFound, 3},
		{AlreadyExists, 4},
		{DaemonNotRunning, 5},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Fatalf("退出码常量 = %d，期望 %d", c.got, c.want)
		}
	}
}

// Test构造器错误到退出码映射 覆盖每种错误类型的构造与 Code() 提取。
func Test构造器错误到退出码映射(t *testing.T) {
	cases := []struct {
		name     string
		err      *Error
		wantCode int
		wantKind string
	}{
		{"用法错误", Usagef("参数不对"), Usage, "usage"},
		{"未找到", NotFoundf("执行器 %s 不存在", "x"), NotFound, "not_found"},
		{"已存在", AlreadyExistsf("执行器 %s 已存在", "x"), AlreadyExists, "already_exists"},
		{"daemon未运行", DaemonNotRunningf("daemon 未运行"), DaemonNotRunning, "daemon_not_running"},
		{"一般错误", Generalf("出错了"), General, "general"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code := Code(c.err); code != c.wantCode {
				t.Fatalf("Code = %d，期望 %d", code, c.wantCode)
			}
			if kind := Type(c.err); kind != c.wantKind {
				t.Fatalf("Type = %q，期望 %q", kind, c.wantKind)
			}
			if c.err.Error() == "" {
				t.Fatal("错误信息为空")
			}
		})
	}
}

func TestCode未知错误按一般错误处理(t *testing.T) {
	if code := Code(errors.New("裸错误")); code != General {
		t.Fatalf("Code(裸错误) = %d，期望 1", code)
	}
	if code := Code(nil); code != General {
		t.Fatalf("Code(nil) = %d，期望 1", code)
	}
}

func Test包装与穿透(t *testing.T) {
	base := errors.New("底层错误")
	wrapped := Wrap(NotFound, base)
	if code := Code(wrapped); code != NotFound {
		t.Fatalf("Code(Wrap) = %d，期望 3", code)
	}
	if !errors.Is(wrapped, base) {
		t.Fatal("errors.Is 未穿透到底层错误")
	}
	// 再套一层 fmt.Errorf %w，Code 应仍能提取
	outer := fmt.Errorf("外层: %w", wrapped)
	if code := Code(outer); code != NotFound {
		t.Fatalf("Code(嵌套) = %d，期望 3", code)
	}
}
