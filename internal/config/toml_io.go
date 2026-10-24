// toml_io.go 负责 config.toml 的读（容错解析 + 中文错误定位）与
// 写（人可读：节注释头、缩进、字段全量落盘）。
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Load 读取并解析 config.toml，缺省字段补默认值。
// 解析失败时错误信息带行号定位（来自 toml.ParseError）。
func Load(path string) (Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return Config{}, fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	applyDefaults(&c)
	return c, nil
}

// LoadString 从字符串解析配置（测试与热加载共用），缺省字段补默认值。
func LoadString(s string) (Config, error) {
	var c Config
	if _, err := toml.Decode(s, &c); err != nil {
		return Config{}, fmt.Errorf("解析配置内容失败：%w", err)
	}
	applyDefaults(&c)
	return c, nil
}

// Save 把配置写为 TOML 文本（人可读格式），先写临时文件再原子替换。
func Save(path string, c Config) error {
	if err := os.MkdirAll(parentDir(path), dirPerm); err != nil {
		return fmt.Errorf("创建配置目录失败：%w", err)
	}
	content := MarshalTOML(c)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入临时配置失败：%w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("原子替换配置失败：%w", err)
	}
	return nil
}

// MarshalTOML 生成人可读的 TOML 文本：中文节注释头、executor 块缩进两格。
// 字段全量落盘（含默认值），保证 roundtrip 稳定。
func MarshalTOML(c Config) string {
	var b strings.Builder
	b.WriteString("# awem 配置文件（由 awem 生成；人可读人可改，daemon 自动热加载）\n\n")

	b.WriteString("[server]\n")
	fmt.Fprintf(&b, "bind = \"%s\"\n", escapeTOMLString(c.Server.Bind))
	fmt.Fprintf(&b, "port  = %d   # 0 = 自动挑选空闲端口并写回\n\n", c.Server.Port)

	if c.Platform != nil {
		b.WriteString("[platform]   # 平台状态联动（只读）\n")
		fmt.Fprintf(&b, "base_url      = \"%s\"\n", escapeTOMLString(c.Platform.BaseURL))
		fmt.Fprintf(&b, "mcp_token     = \"%s\"\n", escapeTOMLString(c.Platform.MCPToken))
		fmt.Fprintf(&b, "poll_interval = \"%s\"\n", formatDuration(c.Platform.PollInterval))
		b.WriteString("\n")
	}

	b.WriteString("[log]\n")
	fmt.Fprintf(&b, "max_size_mb    = %d\n", c.Log.MaxSizeMB)
	fmt.Fprintf(&b, "retention_days = %d\n\n", c.Log.RetentionDays)

	for _, e := range c.Executors {
		b.WriteString("[[executor]]\n")
		fmt.Fprintf(&b, "  name      = \"%s\"\n", escapeTOMLString(e.Name))
		fmt.Fprintf(&b, "  command   = \"%s\"   # 整条启动命令\n", escapeTOMLString(e.Command))
		fmt.Fprintf(&b, "  autostart = %s\n", strconv.FormatBool(e.Autostart))
		if e.WorkspaceRoot != "" {
			fmt.Fprintf(&b, "  workspace_root = \"%s\"   # 工作区根目录覆盖（缺省 ~/autowonder_workspaces_<provider>_<id>）\n", escapeTOMLString(e.WorkspaceRoot))
		}
		fmt.Fprintf(&b, "  [executor.restart]\n")
		fmt.Fprintf(&b, "    enabled = %s   # 崩溃自动拉起（指数退避：1s 起步 ×2 上限 5m，稳定 10m 重置）\n", strconv.FormatBool(derefBool(e.Restart.Enabled, true)))
		fmt.Fprintf(&b, "    backoff = \"%s\"\n", escapeTOMLString(orDefault(e.Restart.Backoff, "exponential")))
		b.WriteString("\n")
	}
	return b.String()
}

// derefBool 解引用 *bool，nil 取默认值。
func derefBool(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}

// orDefault 空串取默认值。
func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// formatDuration 以 TOML 常见的 "60s" 形式输出。
func formatDuration(d time.Duration) string { return d.String() }

// escapeTOMLString 转义为 TOML 基本字符串（处理引号、反斜杠与控制字符）。
func escapeTOMLString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// parentDir 返回路径所在目录。
func parentDir(path string) string {
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	if i := strings.LastIndexByte(path, '\\'); i > 0 {
		return path[:i]
	}
	return "."
}
