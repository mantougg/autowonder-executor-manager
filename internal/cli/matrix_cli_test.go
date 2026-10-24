// matrix_cli_test.go：T07-F10 收口验收。退出码矩阵（每命令 × 主要场景）
// 与 --json schema 抽查（对照变更记录汇总表）。
package cli

import (
	"encoding/json"
	"testing"

	"awem/internal/config"
	"awem/internal/exitcode"
)

// Test退出码矩阵 每命令 × 成功/用法错/未找到/已存在/daemon 不可达。
// daemon 不可达（5）由 daemon 包 TestCLI惰性拉起daemon 场景④覆盖（真二进制）。
func Test退出码矩阵(t *testing.T) {
	// 预置了 qoder 执行器 → doctor 的 provider 检查注入绿环境（CI 无 qoder）
	withGreenDoctorEnv(t)
	e := newCLIEnv(t, config.Executor{Name: "mx", Command: validCommand})

	cases := []struct {
		name string
		args []string
		want int
	}{
		// add：成功 0 / 坏命令 2 / 已存在 4
		{"add成功", []string{"add", validCommand}, exitcode.Success},
		{"add已存在", []string{"add", validCommand, "--name", "mx"}, exitcode.AlreadyExists},
		{"add坏命令", []string{"add", "docker run x"}, exitcode.Usage},
		{"add缺参数", []string{"add"}, exitcode.Usage},
		// list：成功 0 / 多余参数 2
		{"list成功", []string{"list"}, exitcode.Success},
		{"list多余参数", []string{"list", "x"}, exitcode.Usage},
		// status：成功 0 / 不存在 3 / 用法 2
		{"status成功", []string{"status", "mx"}, exitcode.Success},
		{"status全部", []string{"status"}, exitcode.Success},
		{"status不存在", []string{"status", "nope"}, exitcode.NotFound},
		{"status多余", []string{"status", "a", "b"}, exitcode.Usage},
		// start/stop/restart：成功 0 / 不存在 3 / 用法 2
		{"start成功", []string{"start", "mx"}, exitcode.Success},
		{"stop成功", []string{"stop", "mx"}, exitcode.Success},
		{"restart成功", []string{"restart", "mx"}, exitcode.Success},
		{"start不存在", []string{"start", "nope"}, exitcode.NotFound},
		{"start多余", []string{"start", "a", "b"}, exitcode.Usage},
		// edit：成功 0 / 不存在 3 / 用法 2
		{"edit成功", []string{"edit", "mx", "--model", "m2"}, exitcode.Success},
		{"edit不存在", []string{"edit", "nope", "--model", "m"}, exitcode.NotFound},
		{"edit无修改", []string{"edit", "mx"}, exitcode.Usage},
		{"edit坏flag", []string{"edit", "mx", "--不存在"}, exitcode.Usage},
		// log：成功 0 / 不存在 3 / 用法 2
		{"log成功", []string{"log", "mx"}, exitcode.Success},
		{"log不存在", []string{"log", "nope"}, exitcode.NotFound},
		{"log缺名称", []string{"log"}, exitcode.Usage},
		// reload：成功 0 / 用法 2
		{"reload成功", []string{"reload"}, exitcode.Success},
		{"reload多余", []string{"reload", "x"}, exitcode.Usage},
		// doctor：成功 0 / 用法 2（本机环境真探测）
		{"doctor成功", []string{"doctor"}, exitcode.Success},
		{"doctor多余", []string{"doctor", "x"}, exitcode.Usage},
		// remove：成功 0 / 不存在 3 / 用法 2（放最后：删掉 mx 影响后续用例）
		{"remove成功", []string{"remove", "mx", "--yes"}, exitcode.Success},
		{"remove不存在", []string{"remove", "nope", "--yes"}, exitcode.NotFound},
		{"remove缺名称", []string{"remove"}, exitcode.Usage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// remove 后续用例依赖 mx 存在：remove 成功用例排在新建之后；
			// add 成功用例会因 mx 已存在变 4——矩阵里 add成功 用独立环境。
			if tc.name == "add成功" {
				e2 := newCLIEnv(t)
				if code := e2.run(false, tc.args...); code != tc.want {
					t.Fatalf("退出码 %d，期望 %d（输出：%s）", code, tc.want, e2.output())
				}
				return
			}
			code := e.run(false, tc.args...)
			if code != tc.want {
				t.Fatalf("退出码 %d，期望 %d（输出：%s）", code, tc.want, e.output())
			}
		})
	}
}

// TestJSON汇总表抽查 三个命令的 --json 结构与变更记录汇总一致。
func TestJSON汇总表抽查(t *testing.T) {
	e := newCLIEnv(t, config.Executor{Name: "js", Command: validCommand})

	// list：{"executors":[...]}，元素含 8 个视图字段
	if code := e.run(true, "list"); code != 0 {
		t.Fatalf("list --json：%d", code)
	}
	var lv struct {
		Executors []map[string]any `json:"executors"`
	}
	if err := json.Unmarshal([]byte(e.output()), &lv); err != nil || len(lv.Executors) != 1 {
		t.Fatalf("list --json 结构：%v\n%s", err, e.output())
	}
	for _, k := range []string{"name", "command", "autostart", "restart", "parsed", "local", "platform", "drift"} {
		if _, ok := lv.Executors[0][k]; !ok {
			t.Fatalf("list --json 缺 %s", k)
		}
	}

	// status 单个：{"executor":{...}}
	if code := e.run(true, "status", "js"); code != 0 {
		t.Fatalf("status --json：%d", code)
	}
	var sv struct {
		Executor map[string]any `json:"executor"`
	}
	if err := json.Unmarshal([]byte(e.output()), &sv); err != nil || sv.Executor == nil {
		t.Fatalf("status --json 结构：%v\n%s", err, e.output())
	}

	// log：{"name","tail","lines"}
	e.mgr.Sink("js")
	if code := e.run(true, "log", "js"); code != 0 {
		t.Fatalf("log --json：%d %s", code, e.output())
	}
	var lg struct {
		Name  string   `json:"name"`
		Tail  int      `json:"tail"`
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal([]byte(e.output()), &lg); err != nil || lg.Name != "js" || lg.Lines == nil {
		t.Fatalf("log --json 结构：%v\n%s", err, e.output())
	}
}

// Test无参数显示用法并退出2 收口（T01 已有行为，此处复核）。
func Test无参数显示用法并退出2(t *testing.T) {
	e := newCLIEnv(t)
	if code := e.reg.Dispatch(nil); code != exitcode.Usage {
		t.Fatalf("无参数应退出 2，实际 %d", code)
	}
}

// Test错误输出统一中文 统一错误格式由 Registry 层 writeError 保证
// （--json 走进程 stdout、文本走 stderr，T01 command_test 已覆盖 JSON 合法性），
// 此处复核 CLI 层错误仍带正确退出码语义。
func Test错误输出统一中文(t *testing.T) {
	e := newCLIEnv(t)
	if code := e.run(true, "status", "nope"); code != exitcode.NotFound {
		t.Fatalf("退出码应 3，实际 %d", code)
	}
}
