package launchcmd

import (
	"errors"
	"reflect"
	"testing"
)

func TestTokenize引号与空白(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"多空格与Tab", "a   b\tc", []string{"a", "b", "c"}},
		{"单引号包裹空格", `--token 'abc def' --x`, []string{"--token", "abc def", "--x"}},
		{"双引号包裹空格", `--token "abc def"`, []string{"--token", "abc def"}},
		{"引号内含另一引号", `"it's" 'say "hi"'`, []string{`it's`, `say "hi"`}},
		{"双引号内转义引号", `--token "a\"b"`, []string{"--token", `a"b`}},
		{"双引号内转义反斜杠", `"a\\b"`, []string{`a\b`}},
		{"裸转义空格", `a\ b`, []string{"a b"}},
		{"空串与纯空白", "  \t\n ", nil},
		{"相邻引号成一词", `"ab""cd"`, []string{"abcd"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Tokenize(c.in)
			if err != nil {
				t.Fatalf("拆词失败：%v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("拆词结果 %v，期望 %v", got, c.want)
			}
		})
	}
}

func TestTokenize未闭合引号报错(t *testing.T) {
	if _, err := Tokenize(`--token 'abc`); err == nil {
		t.Fatal("单引号未闭合应报错")
	}
	if _, err := Tokenize(`--token "abc`); err == nil {
		t.Fatal("双引号未闭合应报错")
	}
}

func TestParsePrefix版本提取(t *testing.T) {
	cmd := `npx -y autowonder@0.3.22 connect --ws-url ws://h/ws --token t`
	tokens, err := Tokenize(cmd)
	if err != nil {
		t.Fatal(err)
	}
	p, rest, err := parsePrefix(tokens)
	if err != nil {
		t.Fatalf("合法前缀报错：%v", err)
	}
	if p.RuntimeVersion != "0.3.22" {
		t.Fatalf("版本 = %q，期望 0.3.22", p.RuntimeVersion)
	}
	if !reflect.DeepEqual(rest, []string{"--ws-url", "ws://h/ws", "--token", "t"}) {
		t.Fatalf("connect 后参数 = %v", rest)
	}
}

func TestParsePrefix包装剥离(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		ver  string
	}{
		{"nohup与前导&尾部", `nohup npx -y autowonder@1.2.3 connect --x v &`, "1.2.3"},
		{"叠加nohup", `nohup nohup npx autowonder@0.1.0 connect`, "0.1.0"},
		{"--yes形式", `npx --yes autowonder@9.9.9 connect`, "9.9.9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tokens, err := Tokenize(c.cmd)
			if err != nil {
				t.Fatal(err)
			}
			p, _, err := parsePrefix(tokens)
			if err != nil {
				t.Fatalf("应容忍包装：%v", err)
			}
			if p.RuntimeVersion != c.ver {
				t.Fatalf("版本 = %q，期望 %q", p.RuntimeVersion, c.ver)
			}
		})
	}
}

func TestParsePrefix不支持的命令(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
	}{
		{"非npx开头", `node index.js connect --x`},
		{"无版本号", `npx -y autowonder connect --x`},
		{"@后无版本", `npx -y autowonder@ connect --x`},
		{"缺connect", `npx -y autowonder@1.0.0 dispatch ./a.json`},
		{"其他npm包", `npx -y someother@1.0.0 connect --x`},
		{"不支持的npx参数", `npx --silent -y autowonder@1.0.0 connect --x`},
		{"空命令", `   `},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tokens, err := Tokenize(c.cmd)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = parsePrefix(tokens)
			var ue *UnsupportedCommandError
			if !errors.As(err, &ue) {
				t.Fatalf("应返回不支持的命令错误，实际：%v", err)
			}
		})
	}
}
