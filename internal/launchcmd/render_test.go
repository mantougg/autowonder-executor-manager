package launchcmd

import (
	"reflect"
	"strings"
	"testing"
)

// assertRoundtrip 断言 parse(render(parse(cmd))) == parse(cmd)。
func assertRoundtrip(t *testing.T, cmd string) {
	t.Helper()
	p1, err := Parse(cmd)
	if err != nil {
		t.Fatalf("首次解析失败：%v（命令：%s）", err, cmd)
	}
	rendered := p1.Render()
	p2, err := Parse(rendered)
	if err != nil {
		t.Fatalf("重渲染后解析失败：%v（渲染结果：%s）", err, rendered)
	}
	if !reflect.DeepEqual(*p1, *p2) {
		t.Fatalf("roundtrip 不一致：\n 原=%+v\n重建=%+v\n渲染：%s", *p1, *p2, rendered)
	}
}

func TestRender固定顺序(t *testing.T) {
	// 打乱输入顺序，渲染仍按固定顺序输出
	cmd := `npx -y autowonder@0.3.22 connect --max-tasks 5 --provider qoder --token t --model m --executor-id 1 --ws-url ws://h --reasoning-effort low --context-window 100 --memory-mode none --token-aware-enable`
	p, err := Parse(cmd)
	if err != nil {
		t.Fatal(err)
	}
	want := "npx -y autowonder@0.3.22 connect --ws-url ws://h --token t --executor-id 1 " +
		"--provider qoder --model m --reasoning-effort low --context-window 100 " +
		"--memory-mode none --max-tasks 5 --token-aware-enable"
	if got := p.Render(); got != want {
		t.Fatalf("渲染顺序不稳定：\n got=%s\nwant=%s", got, want)
	}
}

func TestRender幂等含passthrough(t *testing.T) {
	cases := []string{
		fullCmd,
		`npx -y autowonder@1.0.0 connect --token t --executor-id 1 --provider qoder --future-flag=v1 --another 42 --token-aware-enable`,
		`npx -y autowonder@1.0.0 connect --token t --executor-id 1 --provider qoder --odd --word`,
		// 最小字段 + 多个未知 flag
		`npx autowonder@2.3.4 connect --ws-url wss://h/ws --token tk --executor-id 99 --provider codex --a=1 --b 2 --c 3 --d`,
	}
	for _, cmd := range cases {
		assertRoundtrip(t, cmd)
	}
}

func TestRender特殊字符加引号(t *testing.T) {
	cmd := `npx -y autowonder@1.0.0 connect --token 'tok with space' --executor-id 1 --provider qoder --model 'model"x'`
	p, err := Parse(cmd)
	if err != nil {
		t.Fatal(err)
	}
	rendered := p.Render()
	// 含空格的值应被单引号包裹
	if !strings.Contains(rendered, `'tok with space'`) {
		t.Fatalf("空格值未加引号：%s", rendered)
	}
	assertRoundtrip(t, cmd)
}

func TestRender编辑字段后passthrough不丢(t *testing.T) {
	cmd := `npx -y autowonder@1.0.0 connect --ws-url ws://h --token t --executor-id 1 --provider qoder --model old-model --future-keep=v9`
	p, err := Parse(cmd)
	if err != nil {
		t.Fatal(err)
	}
	// 编辑已知字段
	p.Model = "new-model"
	p.ReasoningEffort = "high"
	p2, err := Parse(p.Render())
	if err != nil {
		t.Fatalf("编辑后重建解析失败：%v", err)
	}
	if p2.Model != "new-model" || p2.ReasoningEffort != "high" {
		t.Fatalf("编辑未生效：%+v", p2)
	}
	if !reflect.DeepEqual(p2.Passthrough, []string{"--future-keep=v9"}) {
		t.Fatalf("passthrough 丢失：%v", p2.Passthrough)
	}
}

func TestRender布尔false省略(t *testing.T) {
	p := &Parsed{
		Prefix:     Prefix{RuntimeVersion: "1.0.0"},
		Token:      "t",
		ExecutorID: "1",
		Provider:   "qoder",
	}
	rendered := p.Render()
	if strings.Contains(rendered, "--token-aware-enable") || strings.Contains(rendered, "--debug") {
		t.Fatalf("false 布尔不应渲染：%s", rendered)
	}
}

func TestQuoting规则(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"ws://h:7001/ws", "ws://h:7001/ws"}, // 安全字符集内
		{"a.b_c-d@e", "a.b_c-d@e"},
		{"has space", `'has space'`},
		{`has"quote`, `'has"quote'`},
		{"has'squote", `'has'\''squote'`},
		{"-leading", `'-leading'`},
		{"", `''`},
	}
	for _, c := range cases {
		if got := quoteValue(c.in); got != c.want {
			t.Fatalf("quoteValue(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}
