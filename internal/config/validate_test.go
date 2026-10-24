package config

import (
	"strings"
	"testing"
)

// validExec 构造一条合法执行器配置。
func validExec(name string) Executor {
	return Executor{
		Name:    name,
		Command: "npx -y autowonder@0.3.22 connect --ws-url ws://h --token t --executor-id 1 --provider qoder",
	}
}

// Test合法配置通过校验。
func Test合法配置通过校验(t *testing.T) {
	c := Default()
	c.Executors = []Executor{validExec("e1"), validExec("e2.build_x")}
	if err := c.Validate(); err != nil {
		t.Fatalf("合法配置不应报错：%v", err)
	}
}

// Test校验逐条报告 覆盖：重名、非法字符（含中文名）、空 command、bind 非 127.0.0.1。
// executor-id 非数字 / token 空 / provider 非法的校验待 T03 解析器接入后补齐。
func Test校验逐条报告(t *testing.T) {
	c := Default()
	c.Server.Bind = "0.0.0.0"
	c.Executors = []Executor{
		validExec("dup"),
		func() Executor { // 重名
			e := validExec("dup")
			return e
		}(),
		func() Executor { // 中文非法字符
			e := validExec("需求澄清")
			return e
		}(),
		func() Executor { // 其他非法字符
			e := validExec("bad name!")
			return e
		}(),
		func() Executor { // 空 command
			e := validExec("empty-cmd")
			e.Command = "   "
			return e
		}(),
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("非法配置未报错")
	}
	msg := err.Error()
	for _, want := range []string{
		"bind 仅允许 127.0.0.1",       // bind 非 127.0.0.1
		"重复",                        // 重名
		"非法字符",                     // 含中文名
		"需求澄清",                     // 条目定位含出错 name
		"bad name!",                 // 条目定位
		"command 不能为空",              // 空 command
		"共 5 项问题",                   // 聚合计数（bind 1 + 重复 1 + 中文 1 + 非法 1 + 空 command 1）
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误信息缺少 %q：\n%s", want, msg)
		}
	}
}

// Test名称字符集边界 合法边界字符与典型非法字符。
func Test名称字符集边界(t *testing.T) {
	for _, ok := range []string{"a", "A1", "a.b_c-d", "10086", ".hidden", "e-1.2_3"} {
		if !nameAllowed(ok) {
			t.Fatalf("%q 应为合法 name", ok)
		}
	}
	for _, bad := range []string{"", "中文名", "a b", "a/b", `a\b`, "a;b", "a|b", "名字*"} {
		if nameAllowed(bad) {
			t.Fatalf("%q 应为非法 name", bad)
		}
	}
}

// Test端口越界报错。
func Test端口越界报错(t *testing.T) {
	c := Default()
	c.Server.Port = 70000
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "port 越界") {
		t.Fatalf("端口越界未报告：%v", err)
	}
}
