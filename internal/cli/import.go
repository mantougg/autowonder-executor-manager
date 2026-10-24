// import.go：`awem import`（T07-F8）。解析旧 executor-manager.sh →
// 预览（改名建议/坏行报告）→ 确认 → 逐条走 API add；--with-logs 归档
// 旧日志（T03 ArchiveLogs，旧名→新名映射）。
// 缺省脚本路径（决策记录）：当前目录与家目录下的 executor-manager.sh。
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"awem/internal/exitcode"
	"awem/internal/launchcmd"
)

func cmdImport(d Deps) *Command {
	return &Command{
		Name:    "import",
		Summary: "解析旧 executor-manager.sh 导入",
		Usage:   "awem import [--file <旧脚本>] [--with-logs] [--yes]",
		Run: func(g GlobalFlags, args []string) error {
			var file string
			withLogs := false
			for _, a := range args {
				switch {
				case a == "--with-logs":
					withLogs = true
				case a == "--file":
					return usagef("--file 请用 --file=<路径> 形式")
				default:
					if v, ok := cutPrefix(a, "--file="); ok {
						file = v
						continue
					}
					return usagef("import 不认识参数 %s", a)
				}
			}
			// 缺省路径：当前目录 → 家目录（规则记录于变更记录）
			if file == "" {
				candidates := []string{"executor-manager.sh"}
				if home, err := d.HomeDir(); err == nil {
					candidates = append(candidates, filepath.Join(home, "executor-manager.sh"))
				}
				for _, c := range candidates {
					if _, err := os.Stat(c); err == nil {
						file = c
						break
					}
				}
				if file == "" {
					return usagef("未找到 executor-manager.sh（找了 %s）；用 --file=<路径> 指定", strings.Join(candidates, "、"))
				}
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return exitcode.Generalf("读取脚本失败：%v", err)
			}
			imported, err := launchcmd.ParseScript(string(data))
			if err != nil {
				return exitcode.Generalf("解析脚本失败：%v", err)
			}

			// --json 模式输出纯 JSON：需要确认交互时要求显式 --yes
			//（决策记录：--json 面向脚本消费，不接受交互输入）
			if g.JSON && !g.Yes {
				return usagef("--json 模式无交互确认，请加 --yes 或去掉 --json")
			}
			quiet := g.JSON // 抑制人读输出

			// 预览分类
			var good, renamed, bad []launchcmd.Imported
			for _, imp := range imported {
				switch {
				case imp.ParseErr != nil:
					bad = append(bad, imp)
				case imp.NeedsRename:
					renamed = append(renamed, imp)
				default:
					good = append(good, imp)
				}
			}
			if len(good)+len(renamed) == 0 {
				return exitcode.Generalf("脚本中没有可导入的执行器（%d 条全部有问题）", len(bad))
			}

			if !quiet {
				d.printf("解析 %s：%d 条可导入、%d 条需改名、%d 条有问题\n", file, len(good), len(renamed), len(bad))
				for _, imp := range good {
					d.printf("  ✓ %s\n", imp.Name)
				}
				for _, imp := range renamed {
					d.printf("  ~ %s → %s（原名含不支持字符，建议新名，可导入后 edit）\n", imp.Name, imp.SuggestedName)
				}
				for _, imp := range bad {
					d.printf("  ✗ %s：%v\n", imp.Name, imp.ParseErr)
				}
				if len(bad) > 0 {
					d.printf("（%d 条问题条目将被跳过）\n", len(bad))
				}
			}

			if !g.Yes {
				if !d.confirm(fmt.Sprintf("导入 %d 条执行器（跳过 %d 条）？", len(good)+len(renamed), len(bad))) {
					d.printf("已取消\n")
					return nil
				}
			}

			ctx := context.Background()
			cl, err := d.Connect(ctx)
			if err != nil {
				return err
			}

			type result struct {
				Name   string `json:"name"`
				Status string `json:"status"` // imported / renamed / failed
				Detail string `json:"detail,omitempty"`
			}
			var results []result
			addOne := func(name, command string) (string, error) {
				body, _ := json.Marshal(map[string]any{"name": name, "command": command})
				_, err := cl.Do(ctx, "POST", "/api/executors", body)
				if err != nil {
					return "", err
				}
				return name, nil
			}
			for _, imp := range good {
				if _, err := addOne(imp.Name, imp.Command); err != nil {
					results = append(results, result{Name: imp.Name, Status: "failed", Detail: err.Error()})
					continue
				}
				results = append(results, result{Name: imp.Name, Status: "imported"})
			}
			for _, imp := range renamed {
				if _, err := addOne(imp.SuggestedName, imp.Command); err != nil {
					results = append(results, result{Name: imp.Name, Status: "failed", Detail: err.Error()})
					continue
				}
				results = append(results, result{Name: imp.SuggestedName, Status: "renamed", Detail: "原名 " + imp.Name})
			}

			// --with-logs：归档旧日志（旧名 → 最终导入名）
			var archives []launchcmd.ArchiveResult
			if withLogs {
				srcDir := filepath.Dir(file)
				if home, err := d.HomeDir(); err == nil {
					// 旧布局约定 ~/.autowonder-executor/<name>.log（design §15）
					srcDir = filepath.Join(home, ".autowonder-executor")
				}
				renames := map[string]string{}
				for _, r := range results {
					if r.Status == "imported" {
						renames[r.Name] = r.Name
					} else if r.Status == "renamed" {
						// detail 形如 "原名 X"
						orig := strings.TrimPrefix(r.Detail, "原名 ")
						renames[orig] = r.Name
					}
				}
				dstDir := filepath.Join(d.layoutDataLogs(), "executors")
				var aerr error
				archives, aerr = launchcmd.ArchiveLogs(srcDir, dstDir, renames)
				if aerr != nil && !quiet {
					d.printf("日志归档失败：%v\n", aerr)
				}
			}

			// 汇总
			if g.JSON {
				out, _ := json.Marshal(map[string]any{
					"script":  file,
					"results": results,
					"logs":    archives,
				})
				d.printf("%s\n", out)
			} else if !quiet {
				for _, r := range results {
					switch r.Status {
					case "imported":
						d.printf("✓ 已导入 %s\n", r.Name)
					case "renamed":
						d.printf("✓ 已导入 %s（%s）\n", r.Name, r.Detail)
					case "failed":
						d.printf("✗ 失败 %s：%s\n", r.Name, r.Detail)
					}
				}
				if withLogs {
					copied := 0
					for _, a := range archives {
						if a.Copied {
							copied++
							d.printf("✓ 日志归档 %s → %s\n", a.Source, a.Dest)
						} else {
							d.printf("— 日志跳过 %s（%s）\n", a.Source, a.Reason)
						}
					}
					d.printf("日志归档完成：拷贝 %d 条\n", copied)
				}
			}
			for _, r := range results {
				if r.Status == "failed" {
					return generalf("部分条目导入失败（详见输出）")
				}
			}
			return nil
		},
	}
}

// layoutDataLogs 取 data/logs 路径（--with-logs 归档目标）。
func (d Deps) layoutDataLogs() string {
	if d.Layout != nil {
		if p, err := d.Layout(); err == nil {
			return p.LogsDir
		}
	}
	return ""
}
