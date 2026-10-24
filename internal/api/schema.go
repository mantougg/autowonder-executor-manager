// schema.go：REST 响应视图（T06-F3 契约，T07 CLI --json 与 T09 Web 共用）。
// 安全约定（conventions §5）：parsed.token 恒为掩码（logs.MaskSecret）；
// PUT/edit 提交掩码原值视为"未修改"，服务端保留原令牌。
package api

import (
	"time"

	"awem/internal/config"
	"awem/internal/launchcmd"
	"awem/internal/logs"
	"awem/internal/platform"
	"awem/internal/supervisor"
)

// executorView 是执行器完整视图（GET 列表/单条、POST/PUT 的返回体）。
type executorView struct {
	Name          string        `json:"name"`
	Command       string        `json:"command"`
	Autostart     bool          `json:"autostart"`
	WorkspaceRoot string        `json:"workspace_root,omitempty"`
	Restart       restartView   `json:"restart"`
	Parsed        *parsedView   `json:"parsed"`   // 解析失败为 null（配置校验兜底，防御）
	Local         localView     `json:"local"`    // 本地守护状态
	Platform      *platformView `json:"platform"` // 平台状态（T08 前恒 null）
	Drift         any           `json:"drift"`    // 漂移占位（T08 前恒 null）
}

type restartView struct {
	Enabled bool   `json:"enabled"`
	Backoff string `json:"backoff"`
}

type parsedView struct {
	RuntimeVersion  string   `json:"runtime_version"`
	WSURL           string   `json:"ws_url"`
	Token           string   `json:"token"` // 掩码
	ExecutorID      string   `json:"executor_id"`
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	ReasoningEffort string   `json:"reasoning_effort"`
	ContextWindow   string   `json:"context_window"`
	MemoryMode      string   `json:"memory_mode"`
	MaxTasks        string   `json:"max_tasks"`
	TokenAware      bool     `json:"token_aware"`
	Debug           bool     `json:"debug"`
	Passthrough     []string `json:"passthrough"`
}

type localView struct {
	State          string `json:"state"`
	PID            int    `json:"pid"`
	StartedAt      string `json:"started_at"` // RFC3339；未运行为 ""
	UptimeMS       int64  `json:"uptime_ms"`
	Restarts       int    `json:"restarts"`
	BackoffDelayMS int64  `json:"backoff_delay_ms"`
	Conn           string `json:"conn"`
	LastExit       string `json:"last_exit,omitempty"`
}

type platformView struct {
	Status        string `json:"status"` // ONLINE/BUSY/OFFLINE/unreachable/unowned
	LastHeartbeat string `json:"last_heartbeat,omitempty"`
	Name          string `json:"name,omitempty"`       // 平台侧执行器名（接入中文名）
	AgentName     string `json:"agent_name,omitempty"` // 所属数字工人中文名
	Project       string `json:"project,omitempty"`    // 所属项目（工作空间）名
	Squad         string `json:"squad,omitempty"`      // 所属小队名
}

// driftView 是漂移信息（null=无漂移或未联动；文案按 design #9）。
type driftView struct {
	Fields  []platform.DriftField `json:"fields"`
	Message string                `json:"message"` // 固定文案："配置与平台不一致，需 restart 或平台配置已变"
}

// buildExecutorView 组装单执行器视图。
func (s *Server) buildExecutorView(e config.Executor) executorView {
	v := executorView{
		Name:          e.Name,
		Command:       e.Command,
		Autostart:     e.Autostart,
		WorkspaceRoot: e.WorkspaceRoot,
		Restart: restartView{
			Enabled: derefBool(e.Restart.Enabled, true),
			Backoff: orDefault(e.Restart.Backoff, "exponential"),
		},
	}
	if p, err := launchcmd.Parse(e.Command); err == nil {
		v.Parsed = &parsedView{
			RuntimeVersion:  p.RuntimeVersion,
			WSURL:           p.WSURL,
			Token:           logs.MaskSecret(p.Token),
			ExecutorID:      p.ExecutorID,
			Provider:        p.Provider,
			Model:           p.Model,
			ReasoningEffort: p.ReasoningEffort,
			ContextWindow:   p.ContextWindow,
			MemoryMode:      p.MemoryMode,
			MaxTasks:        p.MaxTasks,
			TokenAware:      p.TokenAware,
			Debug:           p.Debug,
			Passthrough:     p.Passthrough,
		}
	}
	if st, ok := s.opts.Sup.SnapshotOne(e.Name); ok {
		conn := st.Conn
		if conn == "" {
			conn = supervisor.ConnUnknown // 未启动/未见过连接日志：视图统一 unknown
		}
		v.Local = localView{
			State:          string(st.State),
			PID:            st.PID,
			StartedAt:      formatTime(st.StartedAt),
			UptimeMS:       st.Uptime.Milliseconds(),
			Restarts:       st.Restarts,
			BackoffDelayMS: st.BackoffDelay.Milliseconds(),
			Conn:           string(conn),
			LastExit:       st.LastExit,
		}
	} else {
		v.Local = localView{State: string(supervisor.StateStopped), Conn: string(supervisor.ConnUnknown)}
	}
	// 平台状态与漂移（联动启用后注入；未联动/未命中为 null 占位）
	if v.Parsed != nil {
	if ps, ok := s.opts.Plat.Lookup(v.Parsed.ExecutorID); ok {
		v.Platform = &platformView{
			Status:        ps.State,
			LastHeartbeat: formatTime(ps.LastHeartbeat),
			Name:          ps.Name,
			AgentName:     ps.AgentName,
			Project:       ps.Project,
			Squad:         ps.Squad,
		}
			if len(ps.Drift) > 0 {
				v.Drift = driftView{
					Fields:  ps.Drift,
					Message: "配置与平台不一致，需 restart 或平台配置已变（v1 不自动回写）",
				}
			}
		}
	}
	return v
}

// statusView 是 GET /api/status 响应。
type statusView struct {
	Version   string       `json:"version"`
	PID       int          `json:"pid"`
	Port      int          `json:"port"`
	StartedAt string       `json:"started_at"`
	UptimeMS  int64        `json:"uptime_ms"`
	Platform  platformMeta `json:"platform"`
}

type platformMeta struct {
	Configured bool `json:"configured"` // [platform] 段是否已配置
	Connected  any  `json:"connected"`  // T08 前恒 null
}

// settingsView 是 GET/PUT /api/settings 的响应体（mcp_token 掩码）。
type settingsView struct {
	Platform *platformSettings `json:"platform"`
	Log      logSettings       `json:"log"`
}

type platformSettings struct {
	BaseURL      string `json:"base_url"`
	MCPToken     string `json:"mcp_token"` // 掩码
	PollInterval string `json:"poll_interval"`
}

type logSettings struct {
	MaxSizeMB     int `json:"max_size_mb"`
	RetentionDays int `json:"retention_days"`
}

func buildSettingsView(c config.Config) settingsView {
	v := settingsView{Log: logSettings{MaxSizeMB: c.Log.MaxSizeMB, RetentionDays: c.Log.RetentionDays}}
	if c.Platform != nil {
		v.Platform = &platformSettings{
			BaseURL:      c.Platform.BaseURL,
			MCPToken:     logs.MaskSecret(c.Platform.MCPToken),
			PollInterval: c.Platform.PollInterval.String(),
		}
	}
	return v
}

// formatTime 时间统一 RFC3339（零值为空串）。
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func derefBool(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}

func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}
