// render.go：命令重建（F3）。结构化字段 + passthrough → 完整命令串。
// 保证：渲染顺序稳定（已知字段固定顺序、passthrough 按原序追加）；
// parse(render(parse(x))) == parse(x)（幂等）；未知 flag 不丢；
// 含空格/引号的值正确加引号。
package launchcmd

import "strings"

// Render 生成完整启动命令串。
func (p *Parsed) Render() string {
	var b strings.Builder
	b.WriteString("npx -y autowonder@")
	b.WriteString(quoteValue(p.RuntimeVersion))
	b.WriteString(" connect")

	appendKV := func(flag, value string) {
		if value == "" {
			return
		}
		b.WriteString(" --")
		b.WriteString(flag)
		b.WriteString(" ")
		b.WriteString(quoteValue(value))
	}
	appendKV("ws-url", p.WSURL)
	appendKV("token", p.Token)
	appendKV("executor-id", p.ExecutorID)
	appendKV("provider", p.Provider)
	appendKV("model", p.Model)
	appendKV("reasoning-effort", p.ReasoningEffort)
	appendKV("context-window", p.ContextWindow)
	appendKV("memory-mode", p.MemoryMode)
	appendKV("max-tasks", p.MaxTasks)
	if p.TokenAware {
		b.WriteString(" --token-aware-enable")
	}
	if p.Debug {
		b.WriteString(" --debug")
	}
	for _, tok := range p.Passthrough {
		b.WriteString(" ")
		b.WriteString(quoteValue(tok))
	}
	return b.String()
}

// safeBare 报告值是否可以不加引号（不含空白/引号/反斜杠/引发歧义的字符，
// 且不以 - 开头避免被误认为 flag）。
func safeBare(v string) bool {
	if v == "" || v[0] == '-' {
		return false
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("._~:@/+=,%-", r):
		default:
			return false
		}
	}
	return true
}

// quoteValue 按需加单引号；内嵌单引号用 '\” 转义。
func quoteValue(v string) string {
	if safeBare(v) {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}
