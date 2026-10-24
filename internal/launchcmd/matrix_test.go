// matrix_test.go：T03-F6 单测矩阵收口。矩阵场景索引（对应 t03 文档 F6 节）：
//
//	 M1 正常完整命令        → fields_test.go TestParse全字段
//	 M2 缺字段              → fields_test.go TestValidate逐字段规则（executor-id 缺失/token 为空/provider 缺失）
//	 M3 坏值                → fields_test.go TestValidate逐字段规则（executor-id 非数字/provider 非法）
//	 M4 引号/空格变形        → prefix_test.go TestTokenize引号与空白、render_test.go TestRender特殊字符加引号
//	 M5 nohup/& 包装         → prefix_test.go TestParsePrefix包装剥离
//	 M6 未知 flag 多个       → fields_test.go TestParse未知flag保序保留
//	 M7 --flag=value 形式   → fields_test.go TestParse等号形式
//	 M8 中文/非法名          → 本文件 TestValidExecutorName矩阵
//	 M9 幂等 roundtrip      → render_test.go TestRender幂等含passthrough + 本文件 Test矩阵全场景roundtrip
//	M10 旧脚本全文 fixture  → script_test.go TestParseScript真实模板Fixture
//	M11 解析层错误          → fields_test.go TestParse缺值报错并定位 + 本文件 Test矩阵解析错误场景
package launchcmd

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestValidExecutorName矩阵 M8：D8 字符集边界。
func TestValidExecutorName矩阵(t *testing.T) {
	ok := []string{"a", "A1", "exec-10086", "cli.build", "a_b-c.d", "10086", "x"}
	bad := []string{"", "需求澄清", "带空格 名", "a/b", `a\b`, "a;b", "a|b", "名*字", "tab\tname"}
	for _, name := range ok {
		if !ValidExecutorName(name) {
			t.Fatalf("%q 应为合法名", name)
		}
	}
	for _, name := range bad {
		if ValidExecutorName(name) {
			t.Fatalf("%q 应为非法名", name)
		}
	}
}

// Test矩阵全场景roundtrip M9：一组代表性命令全部满足 parse(render(parse))==parse。
func Test矩阵全场景roundtrip(t *testing.T) {
	cmds := []string{
		fullCmd, // 完整命令
		`npx -y autowonder@0.3.22 connect --ws-url "ws://内网:7001/ws" --token 't k' --executor-id 1 --provider qodercn`,        // 引号/空格变形
		`nohup npx -y autowonder@1.0.0 connect --token t --executor-id 22 --provider claude --model m --token-aware-enable &`, // nohup 包装
		`npx -y autowonder@1.0.0 connect --executor-id=333 --provider=codex --token=tk --x=1 --y 2 --z`,                       // 等号形式 + 未知 flag
	}
	for _, cmd := range cmds {
		assertRoundtrip(t, cmd)
	}
}

// Test矩阵解析错误场景 M11：结构层错误统一返回可定位错误。
func Test矩阵解析错误场景(t *testing.T) {
	t.Run("整条不是启动命令", func(t *testing.T) {
		_, err := Parse(`docker run -d nginx`)
		var ue *UnsupportedCommandError
		if !errors.As(err, &ue) {
			t.Fatalf("应返回 UnsupportedCommandError：%v", err)
		}
	})
	t.Run("布尔flag带非法值", func(t *testing.T) {
		_, err := Parse(`npx -y autowonder@1.0.0 connect --debug=maybe`)
		var fe FieldErrors
		if !errors.As(err, &fe) || !strings.Contains(err.Error(), "--debug") {
			t.Fatalf("应定位 --debug 布尔值错误：%v", err)
		}
	})
	t.Run("引号未闭合", func(t *testing.T) {
		if _, err := Parse(`npx -y autowonder@1.0.0 connect --token 'abc`); err == nil {
			t.Fatal("引号未闭合应报错")
		}
	})
}

// TestParsedPassthrough不因未知flag而失败 M6 补充：未知 flag 不影响已知字段解析。
func TestParsedPassthrough不因未知flag而失败(t *testing.T) {
	p, err := Parse(`npx -y autowonder@9.9.9 connect --future-a --ws-url ws://h --future-b=val --token t --executor-id 1 --provider qoder --unknown-word`)
	if err != nil {
		t.Fatalf("未知 flag 不应导致解析失败：%v", err)
	}
	if p.WSURL != "ws://h" || p.Token != "t" || p.RuntimeVersion != "9.9.9" {
		t.Fatalf("已知字段解析受损：%+v", p)
	}
	if !reflect.DeepEqual(p.Passthrough, []string{"--future-a", "--future-b=val", "--unknown-word"}) {
		t.Fatalf("passthrough = %v", p.Passthrough)
	}
}
