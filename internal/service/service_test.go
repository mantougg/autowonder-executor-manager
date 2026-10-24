// service_test.go：service 包单测（T10-F1/F2）。
package service

import (
	"runtime"
	"testing"
)

func TestForOS平台分发(t *testing.T) {
	// 三平台实现齐全由交叉编译保证（GOOS=linux/darwin/windows go build
	// ./internal/service 均通过，见 Makefile/CI；init 注册按 GOOS 生效）。
	if _, err := ForOS("freebsd"); err == nil {
		t.Fatal("不支持的平台应明确报错而非空跑")
	}
	if _, err := ForOS(runtime.GOOS); err != nil {
		t.Fatalf("当前平台 %s 应有实现：%v", runtime.GOOS, err)
	}
	plat, _ := Current()
	if plat == nil || plat.Describe() == "" {
		t.Fatal("当前平台实现缺失")
	}
}
