// model.go 定义 config.toml 的结构模型（design.md §5.2 字段集）。
package config

import "time"

// Config 是 config.toml 的完整模型。
type Config struct {
	Server    ServerConfig    `toml:"server"`    // 本地 API 服务
	Platform  *PlatformConfig `toml:"platform"`  // 平台联动（nil = 整体禁用）
	Log       LogConfig       `toml:"log"`       // 日志策略
	Executors []Executor      `toml:"executor"`  // 执行器列表
}

// ServerConfig 是 [server] 段。Port=0 表示自动挑空闲端口并写回（写回逻辑在 T06）。
type ServerConfig struct {
	Bind string `toml:"bind"` // 仅允许 127.0.0.1（校验见 F3）
	Port int    `toml:"port"`
}

// PlatformConfig 是可选的 [platform] 段；缺省 = 平台联动整体禁用。
type PlatformConfig struct {
	BaseURL      string        `toml:"base_url"`      // 平台地址
	MCPToken     string        `toml:"mcp_token"`     // 个人 MCP 令牌（0600 明文落盘）
	PollInterval time.Duration `toml:"poll_interval"` // 轮询间隔
}

// LogConfig 是 [log] 段。
type LogConfig struct {
	MaxSizeMB    int `toml:"max_size_mb"`    // 单文件滚动阈值
	RetentionDays int `toml:"retention_days"` // 保留天数
}

// Executor 是单个 [[executor]] 条目。
type Executor struct {
	Name      string        `toml:"name"`      // 全局唯一，[a-zA-Z0-9._-]（D8）
	Command   string        `toml:"command"`   // 整条启动命令（主入口）
	Autostart bool          `toml:"autostart"` // daemon 启动时是否自动拉起（默认 false）
	Restart   RestartPolicy `toml:"restart"`   // 崩溃守护策略
}

// RestartPolicy 是 [executor.restart] 段。
// Backoff="exponential"：1s 起步 ×2 上限 5m；稳定运行 10m 后计数重置
// （此处只存策略，状态机在 T05 supervisor 实现）。
type RestartPolicy struct {
	Enabled *bool  `toml:"enabled"` // nil=未配置（默认开），读入归一化后恒非 nil
	Backoff string `toml:"backoff"`
}

// BoolPtr 返回 bool 的指针（配置写入用）。
func BoolPtr(v bool) *bool { return &v }

// 轮询间隔默认值（design.md 决策 2）。
const DefaultPollInterval = 60 * time.Second

// Default 返回带全部默认值的配置（design.md §5.2 示例的默认项）。
func Default() Config {
	return Config{
		Server: ServerConfig{Bind: "127.0.0.1", Port: 0},
		Log:    LogConfig{MaxSizeMB: 50, RetentionDays: 7},
	}
}

// applyDefaults 把缺省字段补齐为默认值（读入与规范化共用）。
// 选型说明：restart 默认 enabled=true + exponential（design 决策 3"崩溃自动拉起…可关"，
// 即默认开、显式关）；autostart 默认 false（须显式开启）；platform 配置了但
// poll_interval 缺省时取 60s。
func applyDefaults(c *Config) {
	if c.Server.Bind == "" {
		c.Server.Bind = "127.0.0.1"
	}
	if c.Log.MaxSizeMB == 0 {
		c.Log.MaxSizeMB = 50
	}
	if c.Log.RetentionDays == 0 {
		c.Log.RetentionDays = 7
	}
	if c.Platform != nil && c.Platform.PollInterval == 0 {
		c.Platform.PollInterval = DefaultPollInterval
	}
	for i := range c.Executors {
		if c.Executors[i].Restart.Backoff == "" {
			c.Executors[i].Restart.Backoff = "exponential"
		}
		// restart 段未配置（Enabled 为 nil）时默认开启自动拉起
		// （design 决策 3"崩溃自动拉起…可关"→ 默认开、显式关）。
		if c.Executors[i].Restart.Enabled == nil {
			c.Executors[i].Restart.Enabled = BoolPtr(true)
		}
	}
}

// FindExecutor 按名称查找执行器，未找到返回 nil。
func (c *Config) FindExecutor(name string) *Executor {
	for i := range c.Executors {
		if c.Executors[i].Name == name {
			return &c.Executors[i]
		}
	}
	return nil
}
