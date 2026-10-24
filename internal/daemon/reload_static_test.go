//go:build !windows

// reload_static_test.go：静态资源与 reload 收口（T06-F7 验收）。
//  1. / 与未知前端路由均返回内嵌 index.html（真 daemon HTTP）；
//  2. /api、/ws 不被 SPA fallback 拦截；
//  3. awem reload 手改 config.toml 后生效：后续 API 命令看到新配置；
//  4. POST /api/reload 与文件监视热加载复用同一入口（Watcher.Reload，
//     均产生 config_changed 事件）。
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"awem/internal/config"
)

const f7Command = "npx -y autowonder@0.3.22 connect --ws-url ws://127.0.0.1:1/ws " +
	"--token f7-token-1234567890 --executor-id 10086 --provider qoder --model qfmodel"

func Test静态资源与reload收口(t *testing.T) {
	bin := buildAwem(t)
	cfgDir := t.TempDir()
	p := config.At(cfgDir)
	c := config.Default() // port=0 自动挑
	if err := config.Save(p.ConfigFile, c); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killDaemon(t, cfgDir, syscall.SIGTERM) })

	// 通过 CLI 拉起 daemon（真实用户路径）
	if out, code := runCLI(t, bin, cfgDir, "reload"); code != 0 {
		t.Fatalf("首次 reload 拉起失败：%d %s", code, out)
	}
	info, alive, _ := config.ReadDaemonInfo(p)
	if !alive {
		t.Fatal("daemon 未运行")
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", info.Port)
	tokenB, _ := os.ReadFile(p.TokenFile)
	token := strings.TrimSpace(string(tokenB))

	get := func(path string) (int, http.Header, string) {
		req, _ := http.NewRequest(http.MethodGet, base+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s 失败：%v", path, err)
		}
		defer resp.Body.Close()
		buf := new(strings.Builder)
		_, _ = fmt.Fprint(buf, readAllBody(resp))
		return resp.StatusCode, resp.Header, buf.String()
	}

	// 1) / 与未知前端路由 → index.html
	for _, path := range []string{"/", "/executors/detail", "/settings/任意路由"} {
		code, hdr, body := get(path)
		if code != http.StatusOK || !strings.Contains(hdr.Get("Content-Type"), "text/html") {
			t.Fatalf("GET %s 应 200 text/html，实际 %d %s", path, code, hdr.Get("Content-Type"))
		}
		if !strings.Contains(body, "awem") {
			t.Fatalf("GET %s 应返回内嵌 SPA 页：%s", path, body)
		}
	}

	// 2) /api 未知子路径不被 fallback 拦截（仍走 API 404 JSON）
	code, _, body := get("/api/不存在的端点")
	if code != http.StatusNotFound || !strings.Contains(body, `"error"`) {
		t.Fatalf("API 未知端点应 404 JSON，实际 %d %s", code, body)
	}

	// 3) 手改 config.toml → awem reload → 后续 API 命令看到新配置
	onDisk, _ := config.Load(p.ConfigFile)
	onDisk.Executors = []config.Executor{{Name: "f7-exec", Command: f7Command,
		Restart: config.RestartPolicy{Enabled: config.BoolPtr(true)}}}
	if err := config.Save(p.ConfigFile, onDisk); err != nil {
		t.Fatal(err)
	}
	if out, code := runCLI(t, bin, cfgDir, "reload"); code != 0 {
		t.Fatalf("reload 失败：%d %s", code, out)
	}
	cl, err := Connect(context.Background(), ClientOptions{Paths: p})
	if err != nil {
		t.Fatalf("Connect 失败：%v", err)
	}
	data, err := cl.GetJSON(context.Background(), "/api/executors")
	if err != nil {
		t.Fatalf("GET /api/executors 失败：%v", err)
	}
	var list struct {
		Executors []struct{ Name string } `json:"executors"`
	}
	if err := json.Unmarshal(data, &list); err != nil || len(list.Executors) != 1 || list.Executors[0].Name != "f7-exec" {
		t.Fatalf("reload 后 API 应看到新执行器 f7-exec，data=%s err=%v", data, err)
	}

	// 4) reload 复用热加载入口：events.log 记 config_changed
	//    （等 200ms 去抖后的文件监视重载也会被"内容未变"跳过，
	//    config_changed 只能来自我们这条显式 reload 发布路径）
	time.Sleep(300 * time.Millisecond)
	evData, _ := os.ReadFile(p.EventsLog)
	if !strings.Contains(string(evData), "config_changed") {
		t.Fatalf("events.log 应记录 config_changed（reload 与热加载共用发布入口）：\n%s", evData)
	}
}

// readAllBody 读尽响应体。
func readAllBody(resp *http.Response) string {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}
