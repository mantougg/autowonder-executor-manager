// argv_test.go：Argv 单测（T05-F1）。
package launchcmd

import (
	"reflect"
	"testing"
)

func TestArgv(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want []string
	}{
		{
			name: "标准命令",
			cmd:  "npx -y autowonder@0.3.22 connect --ws-url ws://a/ws --token tk --executor-id 1 --provider qoder",
			want: []string{"npx", "-y", "autowonder@0.3.22", "connect", "--ws-url", "ws://a/ws", "--token", "tk", "--executor-id", "1", "--provider", "qoder"},
		},
		{
			name: "剥掉 nohup/& 包装",
			cmd:  "nohup npx autowonder@1.0.0 connect --executor-id 2 --token tk --provider codex &",
			want: []string{"npx", "-y", "autowonder@1.0.0", "connect", "--executor-id", "2", "--token", "tk", "--provider", "codex"},
		},
		{
			name: "flag=value 形态与未知 flag 原样保留",
			cmd:  "npx -y autowonder@2.0.0 connect --executor-id=3 --token tk --provider qoder -fake-mode=run",
			want: []string{"npx", "-y", "autowonder@2.0.0", "connect", "--executor-id=3", "--token", "tk", "--provider", "qoder", "-fake-mode=run"},
		},
		{
			name: "布尔 flag 与引号值",
			cmd:  `npx -y autowonder@0.3.22 connect --token 'a b' --executor-id 9 --provider qoder --debug`,
			want: []string{"npx", "-y", "autowonder@0.3.22", "connect", "--token", "a b", "--executor-id", "9", "--provider", "qoder", "--debug"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Argv(tc.cmd)
			if err != nil {
				t.Fatalf("Argv 失败：%v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv 不符\ngot  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestArgvRejectsBadCommand(t *testing.T) {
	for _, bad := range []string{"", "bash -c 'echo hi'", "npx -y foo@1.0 connect"} {
		if _, err := Argv(bad); err == nil {
			t.Fatalf("应拒绝不支持的命令：%q", bad)
		}
	}
}
