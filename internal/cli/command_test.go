package cli

import (
	"strings"
	"testing"
)

func TestExtractGlobalFlags(t *testing.T) {
	cases := []struct {
		name     string
		argv     []string
		wantJSON bool
		wantYes  bool
		wantRest []string
	}{
		{
			name:     "无全局flag",
			argv:     []string{"list"},
			wantRest: []string{"list"},
		},
		{
			name:     "子命令前",
			argv:     []string{"--json", "list"},
			wantJSON: true,
			wantRest: []string{"list"},
		},
		{
			name:    "子命令后",
			argv:    []string{"list", "--json", "--yes"},
			wantJSON: true,
			wantYes: true,
			wantRest: []string{"list"},
		},
		{
			name:     "=false形式",
			argv:     []string{"--json=false", "list", "--yes=true"},
			wantYes:  true,
			wantRest: []string{"list"},
		},
		{
			name:     "其他flag不受影响",
			argv:     []string{"add", "--command", "npx --json-example"},
			wantRest: []string{"add", "--command", "npx --json-example"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, rest := extractGlobalFlags(c.argv)
			if g.JSON != c.wantJSON || g.Yes != c.wantYes {
				t.Fatalf("全局 flag 解析不符：JSON=%v Yes=%v，期望 JSON=%v Yes=%v",
					g.JSON, g.Yes, c.wantJSON, c.wantYes)
			}
			if len(rest) != len(c.wantRest) {
				t.Fatalf("剩余参数 %v 不符，期望 %v", rest, c.wantRest)
			}
			for i := range rest {
				if rest[i] != c.wantRest[i] {
					t.Fatalf("剩余参数 %v 不符，期望 %v", rest, c.wantRest)
				}
			}
		})
	}
}

// newTestRegistry 构造带辅助子命令的注册表。
func newTestRegistry() (*Registry, *GlobalFlags, *[]string) {
	r := NewRegistry()
	var got GlobalFlags
	var gotArgs []string
	r.Register(&Command{
		Name:    "probe",
		Summary: "测试用",
		Run: func(g GlobalFlags, args []string) error {
			got = g
			gotArgs = args
			return nil
		},
	})
	r.Register(&Command{Name: "stub", Summary: "占位（Run 为 nil）"})
	return r, &got, &gotArgs
}

func TestDispatch(t *testing.T) {
	t.Run("无参数退出码2", func(t *testing.T) {
		r, _, _ := newTestRegistry()
		if code := r.Dispatch(nil); code != 2 {
			t.Fatalf("退出码 = %d，期望 2", code)
		}
	})
	t.Run("未知子命令退出码2", func(t *testing.T) {
		r, _, _ := newTestRegistry()
		if code := r.Dispatch([]string{"不存在命令"}); code != 2 {
			t.Fatalf("退出码 = %d，期望 2", code)
		}
	})
	t.Run("help成功退出码0", func(t *testing.T) {
		r, _, _ := newTestRegistry()
		if code := r.Dispatch([]string{"help"}); code != 0 {
			t.Fatalf("退出码 = %d，期望 0", code)
		}
	})
	t.Run("占位子命令未实现退出码1", func(t *testing.T) {
		r, _, _ := newTestRegistry()
		if code := r.Dispatch([]string{"stub"}); code != 1 {
			t.Fatalf("退出码 = %d，期望 1", code)
		}
	})
	t.Run("成功退出码0且透传全局flag与剩余参数", func(t *testing.T) {
		r, got, gotArgs := newTestRegistry()
		if code := r.Dispatch([]string{"--json", "probe", "a", "--yes"}); code != 0 {
			t.Fatalf("退出码 = %d，期望 0", code)
		}
		if !got.JSON || !got.Yes {
			t.Fatalf("子命令收到的全局 flag 不符：JSON=%v Yes=%v", got.JSON, got.Yes)
		}
		if len(*gotArgs) != 1 || (*gotArgs)[0] != "a" {
			t.Fatalf("子命令收到参数 %v，期望 [a]", *gotArgs)
		}
	})
	t.Run("子命令返回错误退出码1", func(t *testing.T) {
		r := NewRegistry()
		r.Register(&Command{
			Name: "fail",
			Run: func(g GlobalFlags, args []string) error {
				return errForTest{}
			},
		})
		if code := r.Dispatch([]string{"fail"}); code != 1 {
			t.Fatalf("退出码 = %d，期望 1", code)
		}
	})
}

type errForTest struct{}

func (errForTest) Error() string { return "测试错误" }

func TestUsage包含子命令与全局flag说明(t *testing.T) {
	r, _, _ := newTestRegistry()
	usage := r.Usage()
	for _, want := range []string{"awem", "--json", "--yes", "probe", "用法"} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage 缺少关键内容 %q：\n%s", want, usage)
		}
	}
}
