// reload.go：`awem reload`（T07-F10 自 main.go 移入，统一 Deps 连接与输出）。
package cli

import (
	"context"
	"encoding/json"
)

func cmdReload(d Deps) *Command {
	return &Command{
		Name:    "reload",
		Summary: "手动触发配置重载（正常由文件监视自动完成，此为兜底入口）",
		Usage:   "awem reload",
		Run: func(g GlobalFlags, args []string) error {
			if len(args) > 0 {
				return usagef("awem reload 不接受参数")
			}
			ctx := context.Background()
			cl, err := d.Connect(ctx)
			if err != nil {
				return err // Connect 已包装为退出码 5 语义
			}
			data, err := cl.Do(ctx, "POST", "/api/reload", nil)
			if err != nil {
				return wrapAPIErr(err)
			}
			if g.JSON {
				d.printJSON(data)
				return nil
			}
			var out struct {
				Reloaded bool `json:"reloaded"`
				Changed  bool `json:"changed"`
			}
			_ = json.Unmarshal(data, &out)
			d.printf("配置重载完成（变更：%s）\n", boolCN(out.Changed))
			return nil
		},
	}
}
