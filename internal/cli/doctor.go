// doctor.go：`awem doctor` 环境体检（T07-F9）。检查项：Node ≥ 20、npx
// 可用、qoder/qodercn CLI（按配置 provider 检查）、磁盘剩余、平台连通
// （T08 前占位跳过）。任一 ❌ 退出码 1。
package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// doctorEnv 是体检命令的环境注入点（测试替换 PATH 探测/命令执行；
// 生产用默认真实现）。
var doctorEnv = defaultDoctorDeps()

// checkItem 是一项体检结果。
type checkItem struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail"`
	Advice  string `json:"advice,omitempty"` // ❌ 时的修复建议
	Skipped bool   `json:"skipped,omitempty"`
}

// doctorDeps 是体检的可注入环境（测试替换 PATH/命令执行）。
type doctorDeps struct {
	lookupPath func(string) (string, error) // exec.LookPath
	runVersion func(cmd string, args ...string) (string, error)
	statFree   func(path string) (uint64, error) // 剩余字节数
}

func defaultDoctorDeps() doctorDeps {
	return doctorDeps{
		lookupPath: exec.LookPath,
		runVersion: func(cmd string, args ...string) (string, error) {
			out, err := exec.Command(cmd, args...).Output()
			return strings.TrimSpace(string(out)), err
		},
		statFree: statfsFree,
	}
}

func cmdDoctor(d Deps) *Command {
	return &Command{
		Name:    "doctor",
		Summary: "环境体检（Node/npx/qoder CLI/磁盘/平台连通）",
		Usage:   "awem doctor",
		Run: func(g GlobalFlags, args []string) error {
			if len(args) > 0 {
				return usagef("awem doctor 不接受参数")
			}
			dd := doctorEnv
			var items []checkItem
			items = append(items, checkNode(dd), checkNpx(dd), checkProviderCLIs(d, dd), checkDisk(d, dd), checkPlatform(d))

			failed := 0
			for _, it := range items {
				if !it.OK && !it.Skipped {
					failed++
				}
				if g.JSON {
					continue // --json 纯数组输出（人读行抑制，与 import 一致）
				}
				if it.Skipped {
					d.printf("— %s：%s\n", it.Name, it.Detail)
					continue
				}
				if it.OK {
					d.printf("✅ %s：%s\n", it.Name, it.Detail)
				} else {
					d.printf("❌ %s：%s\n", it.Name, it.Detail)
					if it.Advice != "" {
						d.printf("   建议：%s\n", it.Advice)
					}
				}
			}
			if g.JSON {
				out, _ := json.Marshal(items)
				d.printf("%s\n", out)
				if failed > 0 {
					return generalf("环境体检有 %d 项不通过", failed)
				}
				return nil
			}
			if failed > 0 {
				return generalf("环境体检有 %d 项不通过", failed)
			}
			d.printf("环境体检全部通过\n")
			return nil
		},
	}
}

// checkNode Node ≥ 20（qoder/qodercn 要求，design 附录 A）。
func checkNode(dd doctorDeps) checkItem {
	it := checkItem{Name: "Node.js ≥ 20"}
	path, err := dd.lookupPath("node")
	if err != nil {
		it.Detail = "未找到 node"
		it.Advice = "安装 Node.js ≥ 20（qoder/qodercn 执行器的硬性要求）"
		return it
	}
	out, err := dd.runVersion("node", "--version")
	if err != nil {
		it.Detail = "node --version 执行失败：" + err.Error()
		it.Advice = "检查 node 安装完整性"
		return it
	}
	major := parseMajorVersion(out)
	if major < 20 {
		it.Detail = "版本过低（" + out + "）"
		it.Advice = "升级到 Node.js ≥ 20"
		return it
	}
	it.OK = true
	it.Detail = out + "（" + path + "）"
	return it
}

// checkNpx npx 可用（spawn 依赖）。
func checkNpx(dd doctorDeps) checkItem {
	it := checkItem{Name: "npx"}
	npxName := "npx"
	if runtime.GOOS == "windows" {
		npxName = "npx.cmd"
	}
	path, err := dd.lookupPath(npxName)
	if err != nil {
		it.Detail = "未找到 " + npxName
		it.Advice = "npx 随 Node.js 安装，检查 PATH"
		return it
	}
	it.OK = true
	it.Detail = path
	return it
}

// checkProviderCLIs 按配置执行器的 provider 检查对应 CLI（qoder/qodercn）。
func checkProviderCLIs(d Deps, dd doctorDeps) checkItem {
	it := checkItem{Name: "provider CLI（qoder/qodercn）"}
	// 从 API 拿当前配置的 provider 集合
	providers := map[string]bool{}
	if cl, err := d.Connect(context.Background()); err == nil {
		if data, err := cl.GetJSON(context.Background(), "/api/executors"); err == nil {
			var out struct {
				Executors []struct {
					Parsed *struct {
						Provider string `json:"provider"`
					} `json:"parsed"`
				} `json:"executors"`
			}
			if json.Unmarshal(data, &out) == nil {
				for _, e := range out.Executors {
					if e.Parsed != nil {
						providers[e.Parsed.Provider] = true
					}
				}
			}
		}
	}
	needCLI := map[string]bool{"qoder": false, "qodercn": false}
	for p := range providers {
		if _, ok := needCLI[p]; ok {
			needCLI[p] = true
		}
	}
	if !needCLI["qoder"] && !needCLI["qodercn"] {
		return checkItem{Name: it.Name, OK: true, Skipped: true,
			Detail: "无 qoder/qodercn 执行器，跳过（claude/codex 不需要本地 CLI 预装）"}
	}
	var missing, found []string
	for cli, need := range needCLI {
		if !need {
			continue
		}
		if _, err := dd.lookupPath(cli); err != nil {
			missing = append(missing, cli)
		} else {
			found = append(found, cli)
		}
	}
	if len(missing) > 0 {
		it.Detail = "缺少 " + strings.Join(missing, "、")
		it.Advice = "按平台接入文档安装对应 CLI 并完成登录"
		return it
	}
	it.OK = true
	it.Detail = strings.Join(found, "、") + " 已安装"
	return it
}

// checkDisk 数据目录所在盘剩余空间（阈值 1GB）。
func checkDisk(d Deps, dd doctorDeps) checkItem {
	it := checkItem{Name: "磁盘剩余（数据目录）"}
	dir := "."
	if p, err := d.Layout(); err == nil {
		dir = p.DataDir
		_ = os.MkdirAll(dir, 0o700)
	}
	free, err := dd.statFree(dir)
	if err != nil {
		it.Detail = "无法读取剩余空间：" + err.Error()
		return it
	}
	if free < 1<<30 {
		it.Detail = "仅剩 " + humanBytes(free)
		it.Advice = "日志滚动/保留策略会占用磁盘，请预留至少 1GB"
		return it
	}
	it.OK = true
	it.Detail = humanBytes(free) + " 可用（" + dir + "）"
	return it
}

// checkPlatform 平台连通（T08-F6）：未配置跳过；配置了调一次
// /api/platform/health（daemon 侧 list_projects 探测）。
func checkPlatform(d Deps) checkItem {
	it := checkItem{Name: "平台连通"}
	cl, err := d.Connect(context.Background())
	if err != nil {
		it.Detail = "daemon 不可达：" + err.Error()
		it.Advice = "awem daemon 前台运行排查"
		return it
	}
	data, err := cl.GetJSON(context.Background(), "/api/platform/health")
	if err != nil {
		it.Detail = "查询失败：" + err.Error()
		return it
	}
	var h struct {
		Configured bool   `json:"configured"`
		OK         *bool  `json:"ok"`
		Detail     string `json:"detail"`
	}
	if json.Unmarshal(data, &h) != nil {
		it.Detail = "响应解析失败"
		return it
	}
	if !h.Configured {
		return checkItem{Name: it.Name, OK: true, Skipped: true,
			Detail: "未配置 [platform] 段，跳过（仅本地管理）"}
	}
	if h.OK != nil && *h.OK {
		it.OK = true
		it.Detail = h.Detail
		return it
	}
	it.Detail = h.Detail
	it.Advice = "检查 [platform] base_url 与 mcp_token（awem edit 设置或改 config.toml）"
	return it
}

// parseMajorVersion 从 v20.11.0 提取 20。
func parseMajorVersion(v string) int {
	v = strings.TrimPrefix(v, "v")
	major, _, _ := strings.Cut(v, ".")
	n, _ := strconv.Atoi(major)
	return n
}

// humanBytes 人类可读字节数。
func humanBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return strconv.FormatUint(n>>30, 10) + "GB"
	case n >= 1<<20:
		return strconv.FormatUint(n>>20, 10) + "MB"
	case n >= 1<<10:
		return strconv.FormatUint(n>>10, 10) + "KB"
	}
	return strconv.FormatUint(n, 10) + "B"
}
