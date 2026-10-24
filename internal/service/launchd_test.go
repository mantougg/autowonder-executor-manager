//go:build darwin

// launchd_test.go：plist 模板断言（真机验证另行，见检查记录）。
package service

import (
	"strings"
	"testing"
)

func TestPlist模板字段(t *testing.T) {
	plist := plistTemplate("/Users/某 用户/bin/awem")
	for _, want := range []string{
		`<key>Label</key><string>com.autowonder.awem</string>`,
		"/Users/某 用户/bin/awem",
		`<key>RunAtLoad</key><true/>`,
		`<key>KeepAlive</key><true/>`,
		`<string>daemon</string>`,
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist 缺 %q：\n%s", want, plist)
		}
	}
}
