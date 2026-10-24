// actions.go：`awem start|stop|restart [名称]`（T07-F4）。无名称=全部；
// stop 全部是破坏性操作需确认（--yes 跳过）；幂等由 API/Supervisor 保证；
// 逐条汇报，任一失败整体退出码 1。
package cli

import (
	"context"
	"encoding/json"
	"strconv"
)

func cmdAction(d Deps, action string) *Command {
	summaries := map[string]string{
		"start":   "启动执行器（无名称=全部）",
		"stop":    "停止执行器（无名称=全部，需确认）",
		"restart": "重启执行器（无名称=全部）",
	}
	return &Command{
		Name:    action,
		Summary: summaries[action],
		Usage:   "awem " + action + " [名称...]",
		Run: func(g GlobalFlags, args []string) error {
			if len(args) > 1 {
				return usagef("awem %s 至多一个名称参数（无名称=全部）", action)
			}
			ctx := context.Background()
			cl, err := d.Connect(ctx)
			if err != nil {
				return err
			}

			// 解析目标名单
			var names []string
			if len(args) == 1 {
				// 单名称：交给 API 校验存在性（404 → 3）
				names = []string{args[0]}
			} else {
				data, err := cl.GetJSON(ctx, "/api/executors")
				if err != nil {
					return wrapAPIErr(err)
				}
				var out struct {
					Executors []struct{ Name string } `json:"executors"`
				}
				if err := json.Unmarshal(data, &out); err != nil {
					return generalf("解析执行器列表失败：%v", err)
				}
				for _, e := range out.Executors {
					names = append(names, e.Name)
				}
				if len(names) == 0 {
					d.printf("没有执行器可%s\n", actionWord(action))
					return nil
				}
				// stop 全部：破坏性确认
				if action == "stop" && !g.Yes {
					if !d.confirm(actionPrompt("停止", len(names))) {
						d.printf("已取消\n")
						return nil
					}
				}
			}

			failed := 0
			for _, name := range names {
				data, err := cl.Do(ctx, "POST", "/api/executors/"+name+"/"+action, nil)
				if err != nil {
					failed++
					d.printf("✗ %s：%v\n", name, err)
					// 单名称场景直接返回对应退出码（3/1）
					if len(names) == 1 {
						return wrapAPIErr(err)
					}
					continue
				}
				d.printf("✓ %s 已%s", name, actionWord(action))
				if state := localStateOf(data); state != "" {
					d.printf("（%s）", state)
				}
				d.printf("\n")
			}
			if failed > 0 {
				return generalf("%d 个执行器 %s 失败（详见上方逐条输出）", failed, action)
			}
			return nil
		},
	}
}

// actionWord 动词过去式（输出用）。
func actionWord(action string) string {
	switch action {
	case "start":
		return "启动"
	case "stop":
		return "停止"
	case "restart":
		return "重启"
	}
	return action
}

// actionPrompt 确认文案（写明具体动作与数量）。
func actionPrompt(verb string, n int) string {
	return "将" + verb + "全部 " + strconv.Itoa(n) + " 个执行器"
}

// localStateOf 从动作响应提取本地状态（展示用）。
func localStateOf(data []byte) string {
	var out struct {
		Executor struct {
			Local struct {
				State string `json:"state"`
			} `json:"local"`
		} `json:"executor"`
	}
	if json.Unmarshal(data, &out) != nil {
		return ""
	}
	return out.Executor.Local.State
}
