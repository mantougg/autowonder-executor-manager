// settings.go：状态与设置路由（T06-F3）。
//	GET /api/status    daemon 版本/uptime/平台连接占位
//	GET /api/settings  platform/log 段（mcp_token 掩码）
//	PUT /api/settings  同结构写回（token 掩码=未修改；日志参数下次启动生效）
//	POST /api/reload   手动触发热加载（与文件监视共用 Watcher.Reload 入口）
package api

import (
	"net/http"
	"os"
	"time"

	"awem/internal/config"
	"awem/internal/logs"
)

func (s *Server) registerSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/settings", s.handleSettingsGet)
	mux.HandleFunc("PUT /api/settings", s.handleSettingsPut)
	mux.HandleFunc("POST /api/reload", s.handleReload)
}

// handleStatus GET /api/status（CLI 探活与版本展示共用）。
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.currentConfig()
	writeJSON(w, http.StatusOK, statusView{
		Version:   s.opts.Version,
		PID:       os.Getpid(),
		Port:      s.opts.Port,
		StartedAt: formatTime(s.startedAt),
		UptimeMS:  time.Since(s.startedAt).Milliseconds(),
		Platform:  platformMeta{Configured: cfg.Platform != nil},
	})
}

// handleSettingsGet GET /api/settings
func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildSettingsView(s.currentConfig()))
}

// settingsPutRequest PUT /api/settings 请求体（与 settingsView 同构；
// platform 为 null 表示删除平台联动段）。
type settingsPutRequest struct {
	Platform *platformSettingsPut `json:"platform"`
	Log      *logSettings         `json:"log"`
}

type platformSettingsPut struct {
	BaseURL      string `json:"base_url"`
	MCPToken     string `json:"mcp_token"` // 掩码/空=保留原值
	PollInterval string `json:"poll_interval"`
}

// handleSettingsPut PUT /api/settings：整段替换。
// 语义：mcp_token 提交掩码原值或空串 → 保留旧令牌；日志参数写盘即时、
// 滚动参数对已打开文件下次 daemon 启动生效（记入变更记录）。
func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var req settingsPutRequest
	if err := decodeBody(r, &req); err != nil {
		handleAPIError(w, err)
		return
	}
	var saved settingsView
	err := s.mutateConfig(func(c *config.Config) error {
		if req.Platform != nil {
			// 掩码/空 token → 保留旧值
			token := req.Platform.MCPToken
			if token == "" && c.Platform != nil {
				token = c.Platform.MCPToken
			} else if c.Platform != nil && token == logs.MaskSecret(c.Platform.MCPToken) {
				token = c.Platform.MCPToken
			}
			interval := config.DefaultPollInterval
			if req.Platform.PollInterval != "" {
				d, err := time.ParseDuration(req.Platform.PollInterval)
				if err != nil || d <= 0 {
					return badRequest("poll_interval 必须为正时长（如 60s）")
				}
				interval = d
			}
			c.Platform = &config.PlatformConfig{
				BaseURL:      req.Platform.BaseURL,
				MCPToken:     token,
				PollInterval: interval,
			}
		} else {
			c.Platform = nil // 显式 null = 关闭平台联动
		}
		if req.Log != nil {
			if req.Log.MaxSizeMB <= 0 || req.Log.RetentionDays <= 0 {
				return badRequest("log.max_size_mb 与 log.retention_days 必须为正整数")
			}
			c.Log = config.LogConfig{MaxSizeMB: req.Log.MaxSizeMB, RetentionDays: req.Log.RetentionDays}
		}
		saved = buildSettingsView(*c)
		return nil
	})
	if err != nil {
		handleAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleReload POST /api/reload：手动触发热加载（兜底入口；文件监视的兜底）。
func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.currentConfig()
	s.opts.Watcher.Reload()
	after := s.currentConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"reloaded": true,
		"changed":  before.Server.Port != after.Server.Port || len(before.Executors) != len(after.Executors) ||
			!configEqualShallow(before, after),
	})
}

// configEqualShallow 粗粒度比较（reload 响应的 changed 提示，非严格比较）。
func configEqualShallow(a, b config.Config) bool {
	if len(a.Executors) != len(b.Executors) {
		return false
	}
	for i := range a.Executors {
		if a.Executors[i].Command != b.Executors[i].Command ||
			a.Executors[i].Name != b.Executors[i].Name ||
			a.Executors[i].Autostart != b.Executors[i].Autostart {
			return false
		}
	}
	return true
}
