package cli

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"awem/internal/exitcode"
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
	t.Run("类型化错误映射到对应退出码", func(t *testing.T) {
		r := NewRegistry()
		r.Register(&Command{
			Name: "nf",
			Run: func(g GlobalFlags, args []string) error {
				return exitcode.NotFoundf("执行器 %s 不存在", "demo")
			},
		})
		if code := r.Dispatch([]string{"nf"}); code != exitcode.NotFound {
			t.Fatalf("退出码 = %d，期望 3", code)
		}
	})
	t.Run("json模式下错误输出为合法JSON", func(t *testing.T) {
		r := NewRegistry()
		r.Register(&Command{
			Name: "dup",
			Run: func(g GlobalFlags, args []string) error {
				return exitcode.AlreadyExistsf("执行器 demo 已存在")
			},
		})
		if code := r.Dispatch([]string{"--json", "dup"}); code != exitcode.AlreadyExists {
			t.Fatalf("退出码 = %d，期望 4", code)
		}
	})
}

// TestJSON错误输出合法JSON 捕获 writeError 的真实 stdout 输出并解析校验。
func TestJSON错误输出合法JSON(t *testing.T) {
	old := os.Stdout
	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建管道失败：%v", err)
	}
	os.Stdout = wPipe
	writeError(true, exitcode.Usagef("缺少参数"))
	wPipe.Close()
	os.Stdout = old

	raw, _ := io.ReadAll(rPipe)
	if !json.Valid(raw) {
		t.Fatalf("输出不是合法 JSON：%s", raw)
	}
	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("JSON 解析失败：%v，原文：%s", err, raw)
	}
	if parsed.Error.Code != exitcode.Usage || parsed.Error.Type != "usage" || parsed.Error.Message != "缺少参数" {
		t.Fatalf("JSON 错误结构不符：%+v", parsed.Error)
	}
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
