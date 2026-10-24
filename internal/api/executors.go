// executors.go：执行器资源路由（T06-F3）。
//
//	GET    /api/executors                 列表（含本地/平台/漂移视图）
//	POST   /api/executors                 添加（整条命令，服务端解析+校验）
//	GET    /api/executors/{name}          单条
//	PUT    /api/executors/{name}          结构化编辑（--command 整条替换或字段级）
//	DELETE /api/executors/{name}          删除（stop/purge 参数化）
//	POST   /api/executors/{name}/start|stop|restart
//	GET    /api/executors/{name}/log      非流式尾部拉取（tail=N；流式走 /ws/logs）
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"awem/internal/config"
	"awem/internal/launchcmd"
	"awem/internal/logs"
	"awem/internal/supervisor"
)

func (s *Server) registerExecutorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/executors", s.handleList)
	mux.HandleFunc("POST /api/executors", s.handleAdd)
	mux.HandleFunc("GET /api/executors/{name}", s.handleGet)
	mux.HandleFunc("PUT /api/executors/{name}", s.handleEdit)
	mux.HandleFunc("DELETE /api/executors/{name}", s.handleRemove)
	mux.HandleFunc("POST /api/executors/{name}/start", s.handleAction(s.opts.Sup.StartExecutor))
	mux.HandleFunc("POST /api/executors/{name}/stop", s.handleAction(s.opts.Sup.StopExecutor))
	mux.HandleFunc("POST /api/executors/{name}/restart", s.handleAction(s.opts.Sup.RestartExecutor))
	mux.HandleFunc("GET /api/executors/{name}/log", s.handleLog)
}

// handleAPIError 统一把 handler 错误写回响应（apiError 之外的错误按 500）。
func handleAPIError(w http.ResponseWriter, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeAPIError(w, ae)
		return
	}
	writeAPIError(w, internalErr(err))
}

// handleList GET /api/executors
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	cfg := s.currentConfig()
	views := make([]executorView, 0, len(cfg.Executors))
	for _, e := range cfg.Executors {
		views = append(views, s.buildExecutorView(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"executors": views})
}

// addRequest POST /api/executors 请求体。
type addRequest struct {
	Name           string `json:"name"`
	Command        string `json:"command"`
	Autostart      bool   `json:"autostart"`
	Start          bool   `json:"start"` // 添加后立即启动（autostart 语义之外的显式启动）
	WorkspaceRoot  string `json:"workspace_root"`
	RestartEnabled *bool  `json:"restart_enabled"` // nil=默认开
}

// handleAdd POST /api/executors：完整链路（解析→校验→落盘→发布→可选启动）。
func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	var req addRequest
	if err := decodeBody(r, &req); err != nil {
		handleAPIError(w, err)
		return
	}
	if req.Name == "" {
		handleAPIError(w, badRequest("name 不能为空"))
		return
	}
	if !launchcmd.ValidExecutorName(req.Name) {
		handleAPIError(w, invalidName("name 只允许 [a-zA-Z0-9._-]"))
		return
	}
	if req.Command == "" {
		handleAPIError(w, badRequest("command 不能为空（粘贴整条启动命令）"))
		return
	}
	parsed, err := launchcmd.Parse(req.Command)
	if err != nil {
		handleAPIError(w, commandError(err))
		return
	}
	if err := parsed.Validate(); err != nil {
		handleAPIError(w, commandError(err))
		return
	}

	var added config.Executor
	err = s.mutateConfig(func(c *config.Config) error {
		if c.FindExecutor(req.Name) != nil {
			return alreadyExists(req.Name)
		}
		added = config.Executor{
			Name:          req.Name,
			Command:       req.Command,
			Autostart:     req.Autostart,
			WorkspaceRoot: req.WorkspaceRoot,
			Restart:       config.RestartPolicy{Enabled: orDefaultBool(req.RestartEnabled), Backoff: "exponential"},
		}
		c.Executors = append(c.Executors, added)
		return nil
	})
	if err != nil {
		handleAPIError(w, err)
		return
	}
	// 显式启动（autostart=true 已由发布路径的 ApplyConfig 拉起，Start 幂等）
	if req.Start {
		if err := s.opts.Sup.StartExecutor(req.Name); err != nil {
			handleAPIError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"executor": s.buildExecutorView(added)})
}

// handleGet GET /api/executors/{name}
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg := s.currentConfig()
	e := cfg.FindExecutor(name)
	if e == nil {
		handleAPIError(w, notFound("执行器 "+name+" 不存在"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"executor": s.buildExecutorView(*e)})
}

// editRequest PUT /api/executors/{name} 请求体：command 整条替换（优先）
// 或 fields 字段级编辑（改后服务端重新生成整条命令，未知 flag 不丢）。
type editRequest struct {
	Command        string       `json:"command"`
	Fields         *fieldsPatch `json:"fields"`
	Autostart      *bool        `json:"autostart"`
	WorkspaceRoot  *string      `json:"workspace_root"`
	RestartEnabled *bool        `json:"restart_enabled"`
}

// fieldsPatch 结构化字段补丁：nil=不改；非 nil 字段=设新值（含空串清除，
// 布尔除外）。token 提交掩码原值视为未修改。
type fieldsPatch struct {
	RuntimeVersion  *string `json:"runtime_version"`
	WSURL           *string `json:"ws_url"`
	Token           *string `json:"token"`
	ExecutorID      *string `json:"executor_id"`
	Provider        *string `json:"provider"`
	Model           *string `json:"model"`
	ReasoningEffort *string `json:"reasoning_effort"`
	ContextWindow   *string `json:"context_window"`
	MemoryMode      *string `json:"memory_mode"`
	MaxTasks        *string `json:"max_tasks"`
	TokenAware      *bool   `json:"token_aware"`
	Debug           *bool   `json:"debug"`
}

// handleEdit PUT /api/executors/{name}：结构化编辑。
// 运行中进程不受影响（新命令下次启动生效——与热加载语义一致）。
func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req editRequest
	if err := decodeBody(r, &req); err != nil {
		handleAPIError(w, err)
		return
	}

	var edited config.Executor
	err := s.mutateConfig(func(c *config.Config) error {
		e := c.FindExecutor(name)
		if e == nil {
			return notFound("执行器 " + name + " 不存在")
		}
		if req.Command != "" {
			parsed, err := launchcmd.Parse(req.Command)
			if err != nil {
				return commandError(err)
			}
			if err := parsed.Validate(); err != nil {
				return commandError(err)
			}
			e.Command = req.Command
		}
		if req.Fields != nil {
			newCommand, err := s.applyFields(e.Command, req.Fields)
			if err != nil {
				return err
			}
			e.Command = newCommand
		}
		if req.Autostart != nil {
			e.Autostart = *req.Autostart
		}
		if req.WorkspaceRoot != nil {
			e.WorkspaceRoot = *req.WorkspaceRoot
		}
		if req.RestartEnabled != nil {
			e.Restart.Enabled = req.RestartEnabled
		}
		edited = *e
		return nil
	})
	if err != nil {
		handleAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"executor": s.buildExecutorView(edited)})
}

// applyFields 把字段补丁套到现有命令：解析 → 掩码 token 还原 → 覆盖字段 →
// 重新渲染（passthrough 保序不丢）。
func (s *Server) applyFields(command string, f *fieldsPatch) (string, error) {
	parsed, err := launchcmd.Parse(command)
	if err != nil {
		return "", commandError(err)
	}
	// token 掩码语义：提交值 == 当前掩码 → 保留原值
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	if f.Token != nil && *f.Token == logs.MaskSecret(parsed.Token) {
		f.Token = nil
	}
	set(&parsed.RuntimeVersion, f.RuntimeVersion)
	set(&parsed.WSURL, f.WSURL)
	set(&parsed.Token, f.Token)
	set(&parsed.ExecutorID, f.ExecutorID)
	set(&parsed.Provider, f.Provider)
	set(&parsed.Model, f.Model)
	set(&parsed.ReasoningEffort, f.ReasoningEffort)
	set(&parsed.ContextWindow, f.ContextWindow)
	set(&parsed.MemoryMode, f.MemoryMode)
	set(&parsed.MaxTasks, f.MaxTasks)
	if f.TokenAware != nil {
		parsed.TokenAware = *f.TokenAware
	}
	if f.Debug != nil {
		parsed.Debug = *f.Debug
	}
	if err := parsed.Validate(); err != nil {
		return "", commandError(err)
	}
	return parsed.Render(), nil
}

// removeResponse DELETE 响应。
type removeResponse struct {
	Removed string `json:"removed"`
	Stopped bool   `json:"stopped"`
	Purged  bool   `json:"purged"`
}

// handleRemove DELETE /api/executors/{name}?stop=true&purge=true
// 运行中的执行器要求显式 stop=true（确认语义归 CLI/Web 层），否则 409。
func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	stop := r.URL.Query().Get("stop") == "true"
	purge := r.URL.Query().Get("purge") == "true"

	// 先检查存在与运行态（在配置变更前判断）
	cfg := s.currentConfig()
	e := cfg.FindExecutor(name)
	if e == nil {
		handleAPIError(w, notFound("执行器 "+name+" 不存在"))
		return
	}
	running := false
	if st, ok := s.opts.Sup.SnapshotOne(name); ok && supervisor.RunningLike(st.State) {
		running = true
	}
	if running && !stop {
		handleAPIError(w, &apiError{
			Status: http.StatusConflict, Code: "running",
			Message: "执行器 " + name + " 运行中，删除须显式 stop=true（先停止再删除）",
		})
		return
	}
	// 先停进程再改配置：发布路径（ApplyConfig 消失条目）也会停止，
	// 但那时配置已不含该执行器，显式等待收口须在删除前做
	if running {
		if err := s.opts.Sup.StopExecutor(name); err != nil {
			handleAPIError(w, err)
			return
		}
	}

	err := s.mutateConfig(func(c *config.Config) error {
		idx := -1
		for i := range c.Executors {
			if c.Executors[i].Name == name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return notFound("执行器 " + name + " 不存在")
		}
		c.Executors = append(c.Executors[:idx], c.Executors[idx+1:]...)
		return nil
	})
	if err != nil {
		handleAPIError(w, err)
		return
	}
	if purge {
		if err := s.opts.Logs.Purge(name); err != nil {
			handleAPIError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, removeResponse{Removed: name, Stopped: running, Purged: purge})
}

// handleAction 包装 start/stop/restart 三个动作路由（共用 404 语义）。
func (s *Server) handleAction(fn func(string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := fn(name); err != nil {
			if errors.Is(err, supervisor.ErrExecutorNotFound) {
				handleAPIError(w, notFound("执行器 "+name+" 不存在"))
				return
			}
			handleAPIError(w, err)
			return
		}
		cfg := s.currentConfig()
		if e := cfg.FindExecutor(name); e != nil {
			writeJSON(w, http.StatusOK, map[string]any{"executor": s.buildExecutorView(*e)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"executor": executorView{Name: name}})
	}
}

// handleLog GET /api/executors/{name}/log?tail=N（非流式；流式走 /ws/logs/{name}）。
func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg0 := s.currentConfig()
	if cfg0.FindExecutor(name) == nil {
		handleAPIError(w, notFound("执行器 "+name+" 不存在"))
		return
	}
	tail := 50
	if v := r.URL.Query().Get("tail"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			handleAPIError(w, badRequest("tail 必须为正整数"))
			return
		}
		tail = n
	}
	lines, err := s.opts.Logs.Tail(name, tail)
	if err != nil {
		handleAPIError(w, err)
		return
	}
	if lines == nil {
		lines = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "tail": tail, "lines": lines})
}

// decodeBody 解析 JSON 请求体（限制 1MB，防误用大 payload）。
func decodeBody(r *http.Request, v any) error {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return badRequest("读取请求体失败：" + err.Error())
	}
	if len(data) == 0 {
		return badRequest("请求体不能为空（JSON）")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return badRequest("JSON 解析失败：" + err.Error())
	}
	return nil
}

// orDefaultBool nil → 默认开（与配置默认一致）。
func orDefaultBool(p *bool) *bool {
	if p == nil {
		return config.BoolPtr(true)
	}
	return p
}
