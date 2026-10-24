// list.go：`awem list`（T07-F2）。表格：名称/本地态(连接)/平台(T08 前 —)/
// 模型/autostart/漂移(T08 前 —)；--json 直透 API 响应（schema 同构）。
package cli

import (
	"context"
	"encoding/json"
	"strings"
	"text/tabwriter"
)

func cmdList(d Deps) *Command {
	return &Command{
		Name:    "list",
		Summary: "列出全部执行器（本地+平台状态）",
		Usage:   "awem list",
		Run: func(g GlobalFlags, args []string) error {
			if len(args) > 0 {
				return usagef("awem list 不接受参数")
			}
			ctx := context.Background()
			cl, err := d.Connect(ctx)
			if err != nil {
				return err
			}
			data, err := cl.GetJSON(ctx, "/api/executors")
			if err != nil {
				return wrapAPIErr(err)
			}
			if g.JSON {
				d.printJSON(data)
				return nil
			}
			return d.printList(data)
		},
	}
}

// executorBrief 是列表/摘要共用的行视图（与 API executor 视图同构）。
type executorBrief struct {
	Name      string `json:"name"`
	Autostart bool   `json:"autostart"`
	Parsed    *struct {
		Model    string `json:"model"`
		Provider string `json:"provider"`
	} `json:"parsed"`
	Local struct {
		State string `json:"state"`
		Conn  string `json:"conn"`
	} `json:"local"`
	Platform *struct {
		Status string `json:"status"`
	} `json:"platform"`
	Drift any `json:"drift"`
}

// printList 表格输出；空列表给中文引导。
func (d Deps) printList(data []byte) error {
	var out struct {
		Executors []executorBrief `json:"executors"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return generalf("解析列表响应失败：%v", err)
	}
	if len(out.Executors) == 0 {
		d.printf("还没有执行器。添加：awem add \"npx -y autowonder@<版本> connect …\"\n")
		return nil
	}
	w := tabwriter.NewWriter(d.out(), 0, 2, 2, ' ', 0)
	writeRow(w, "名称", "状态", "连接", "平台", "模型", "自启")
	for _, e := range out.Executors {
		model := "—"
		if e.Parsed != nil && e.Parsed.Model != "" {
			model = e.Parsed.Model
		}
		platform := "—"
		if e.Platform != nil && e.Platform.Status != "" {
			platform = e.Platform.Status
		}
		writeRow(w, e.Name, e.Local.State, e.Local.Conn, platform, model, boolMark(e.Autostart))
	}
	_ = w.Flush()
	d.printf("共 %d 个执行器（平台/漂移联动需配置 [platform] 段，T08）\n", len(out.Executors))
	return nil
}

// writeRow 输出一行 tabwriter 表格。
func writeRow(w *tabwriter.Writer, cells ...string) {
	_, _ = w.Write([]byte(strings.Join(cells, "\t") + "\n"))
}

func boolMark(b bool) string {
	if b {
		return "✓"
	}
	return "—"
}
