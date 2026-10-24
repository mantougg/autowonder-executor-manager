// status.go：`awem status [名称]`（T07-F3）。单个=详细（运行态/pid/uptime/
// 重启次数/连接状态/平台/漂移占位）；无名称=全部摘要；未找到 → 3。
package cli

import (
	"context"
	"encoding/json"
	"time"
)

func cmdStatus(d Deps) *Command {
	return &Command{
		Name:    "status",
		Summary: "查看运行态、pid、uptime、重启次数、连接状态、平台状态、漂移",
		Usage:   "awem status [名称]",
		Run: func(g GlobalFlags, args []string) error {
			if len(args) > 1 {
				return usagef("awem status [名称] 至多一个名称参数")
			}
			ctx := context.Background()
			cl, err := d.Connect(ctx)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				data, err := cl.GetJSON(ctx, "/api/executors/"+args[0])
				if err != nil {
					return wrapAPIErr(err) // not_found → 3
				}
				if g.JSON {
					d.printJSON(data)
					return nil
				}
				var out struct {
					Executor statusDetail `json:"executor"`
				}
				if err := json.Unmarshal(data, &out); err != nil {
					return generalf("解析响应失败：%v", err)
				}
				d.printStatusDetail(out.Executor)
				return nil
			}
			// 全部摘要
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

// statusDetail 是单个执行器的详细视图（API executor 视图同构）。
type statusDetail struct {
	Name      string `json:"name"`
	Autostart bool   `json:"autostart"`
	Parsed    *struct {
		ExecutorID string `json:"executor_id"`
		Provider   string `json:"provider"`
		Model      string `json:"model"`
		RuntimeVer string `json:"runtime_version"`
	} `json:"parsed"`
	Local struct {
		State          string `json:"state"`
		PID            int    `json:"pid"`
		StartedAt      string `json:"started_at"`
		UptimeMS       int64  `json:"uptime_ms"`
		Restarts       int    `json:"restarts"`
		BackoffDelayMS int64  `json:"backoff_delay_ms"`
		Conn           string `json:"conn"`
		LastExit       string `json:"last_exit"`
	} `json:"local"`
	Platform *struct {
		Status        string `json:"status"`
		LastHeartbeat string `json:"last_heartbeat"`
	} `json:"platform"`
	Drift any `json:"drift"`
}

// printStatusDetail 输出单个执行器详情。
func (d Deps) printStatusDetail(e statusDetail) {
	d.printf("执行器：%s\n", e.Name)
	d.printf("  运行状态：%s", e.Local.State)
	if e.Local.State == "backoff" && e.Local.BackoffDelayMS > 0 {
		d.printf("（%.1fs 后自动拉起）", float64(e.Local.BackoffDelayMS)/1000)
	}
	d.printf("\n")
	if e.Local.PID > 0 {
		d.printf("  pid：%d    uptime：%s\n", e.Local.PID, formatMS(e.Local.UptimeMS))
		d.printf("  启动时间：%s\n", orDash(e.Local.StartedAt))
	}
	d.printf("  连接状态：%s\n", orDash(e.Local.Conn))
	d.printf("  自动重启次数：%d", e.Local.Restarts)
	if e.Local.LastExit != "" {
		d.printf("    上次退出：%s", e.Local.LastExit)
	}
	d.printf("\n")
	if e.Parsed != nil {
		d.printf("  执行器 ID：%s    provider：%s    模型：%s    autowonder@%s\n",
			orDash(e.Parsed.ExecutorID), orDash(e.Parsed.Provider), orDash(e.Parsed.Model), orDash(e.Parsed.RuntimeVer))
	}
	d.printf("  autostart：%s\n", boolCN(e.Autostart))
	if e.Platform != nil {
		d.printf("  平台状态：%s（心跳 %s）\n", e.Platform.Status, orDash(e.Platform.LastHeartbeat))
	} else {
		d.printf("  平台状态：—（未配置 [platform] 联动，T08）\n")
	}
	d.printf("  配置漂移：—（T08 检测）\n")
}

// formatMS 毫秒时长转人读格式。
func formatMS(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	return (time.Duration(ms) * time.Millisecond).Truncate(time.Second).String()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
