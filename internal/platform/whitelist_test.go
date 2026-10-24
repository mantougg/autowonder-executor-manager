// whitelist_test.go：只读白名单强制（T08-F5 验收）。
//  1. 白名单外的工具名（一切平台写操作）在客户端层直接拒绝，无 HTTP 发出；
//  2. 代码检索：internal/platform/ 内不存在平台写工具调用（检索方式记录于
//     任务文档检查记录：grep 平台写动词 + 工具名字符串断言只读三件套）。
package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test白名单外工具直接拒绝(t *testing.T) {
	var httpCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok-1")

	// 绕道尝试：同包内直接调 call（唯一能触到底层的途径）
	for _, tool := range []string{
		"autowonder.create_workitem",
		"autowonder.assign_workitem",
		"autowonder.delete_executor",
		"autowonder.transition_workitem",
		"tools/call",    // 方法名伪装
		"servers/write", // 任意串
	} {
		if _, err := c.call(context.Background(), tool, nil); err == nil {
			t.Fatalf("工具 %q 应被白名单拒绝", tool)
		}
	}
	if httpCalled {
		t.Fatal("白名单拒绝必须发生在 HTTP 发出之前")
	}
	// 公开面只有三个只读方法（编译期约束：Client 的导出方法集）
	// 此处以反射做一次结构性断言，防止后续误加导出方法。
	assertReadOnlyExports(t)
}

// Test白名单恰为只读三件套 白名单内容与 design §7/附录 A 完全一致。
func Test白名单恰为只读三件套(t *testing.T) {
	want := []string{
		"autowonder.list_projects",
		"autowonder.list_executors",
		"autowonder.get_executor_launch_config",
	}
	if len(readOnlyTools) != len(want) {
		t.Fatalf("白名单应恰为 %d 项，实际 %d：%v", len(want), len(readOnlyTools), readOnlyTools)
	}
	for _, w := range want {
		if !readOnlyTools[w] {
			t.Fatalf("白名单缺 %s", w)
		}
	}
}
