//go:build linux

// systemd_test.go：unit 模板断言（Linux）。
package service

import (
	"strings"
	"testing"
)

func TestUnit模板含daemon子命令与引号(t *testing.T) {
	unit := unitTemplate("/opt/my awem/bin/awem")
	if !strings.Contains(unit, `ExecStart="/opt/my awem/bin/awem" daemon`) {
		t.Fatalf("ExecStart 应为引号包裹二进制 + daemon 子命令（空格路径安全）：\n%s", unit)
	}
	for _, want := range []string{"Restart=on-failure", "WantedBy=default.target", "[Unit]", "[Service]", "[Install]"} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit 缺 %s：\n%s", want, unit)
		}
	}
}
