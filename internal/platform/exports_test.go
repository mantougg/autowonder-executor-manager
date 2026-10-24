// exports_test.go：Client 导出面（公开方法集）只读性结构断言（T08-F5）。
package platform

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// readPackageFile 读本包目录下的源文件。
func readPackageFile(name string) (string, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), name))
	return string(data), err
}

// assertReadOnlyExports 断言 *Client 的导出方法恰为三个只读工具方法
//（防止后续开发误加导出的通用 call/write 方法破坏只读红线）。
func assertReadOnlyExports(t *testing.T) {
	t.Helper()
	typ := reflect.TypeOf(&Client{})
	var exported []string
	for i := 0; i < typ.NumMethod(); i++ {
		exported = append(exported, typ.Method(i).Name)
	}
	allowed := map[string]bool{
		"ListProjects": true, "ListExecutors": true, "GetExecutorLaunchConfig": true,
	}
	for _, m := range exported {
		if !allowed[m] {
			t.Fatalf("*Client 出现白名单外导出方法 %s（红线：只读三件套之外不允许导出）", m)
		}
	}
}

// Test平台包无写动词工具字符串 代码层静态自检：internal/platform 源码中
// 不出现平台写工具名（与任务文档"代码检索检查"对应的可执行断言）。
func Test平台包无写动词工具字符串(t *testing.T) {
	forbidden := []string{
		"create_workitem", "assign_workitem", "transition_workitem",
		"create_executor", "delete_executor", "update_executor",
		"create_agent", "delete_agent", "publish_agent", "control_delivery",
	}
	// 本包源文件列表（编译期已知；新增文件时此表维护成本可接受，
	// 且 assertReadOnlyExports 已从类型面兜底）
	files := []string{"platform.go", "mcp.go"}
	for _, f := range files {
		src, err := readPackageFile(f)
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", f, err)
		}
		for _, w := range forbidden {
			if strings.Contains(src, w) {
				t.Fatalf("%s 含平台写工具字符串 %s（红线违规）", f, w)
			}
		}
	}
}
