// fields.go：结构化字段解析（F2）。从 connect 参数中解析 §5.3 全部已知字段，
// 未知 flag 原样保序进 Passthrough；校验规则：executor-id 数字、token 非空、
// provider ∈ {qoder,qodercn,claude,codex}。
package launchcmd

import (
	"fmt"
	"strings"
)

// Parsed 是一条启动命令的完整解析结果。
type Parsed struct {
	Prefix

	WSURL           string   // --ws-url
	Token           string   // --token
	ExecutorID      string   // --executor-id（字符串保真，Validate 判数字）
	Provider        string   // --provider
	Model           string   // --model
	ReasoningEffort string   // --reasoning-effort
	ContextWindow   string   // --context-window
	MemoryMode      string   // --memory-mode
	MaxTasks        string   // --max-tasks
	TokenAware      bool     // --token-aware-enable（布尔）
	Debug           bool     // --debug（布尔）
	Passthrough     []string // 未知 flag 与多余词，原样保序（roundtrip 不丢）
}

// FieldIssue 定位到具体 flag 的问题。
type FieldIssue struct {
	Flag    string // 相关 flag 名（不带 --）
	Problem string // 中文问题描述
}

func (i FieldIssue) Error() string {
	return fmt.Sprintf("--%s：%s", i.Flag, i.Problem)
}

// FieldErrors 聚合多个字段问题（逐字段定位）。
type FieldErrors []FieldIssue

func (e FieldErrors) Error() string {
	msgs := make([]string, len(e))
	for i, issue := range e {
		msgs[i] = issue.Error()
	}
	return fmt.Sprintf("命令字段校验失败，共 %d 项问题：%s", len(e), strings.Join(msgs, "；"))
}

// Parse 解析整条启动命令字符串（tokenize → 前缀识别 → 字段解析）。
// 语法层问题（不支持的命令、flag 缺值、布尔 flag 带值等）在解析阶段报告；
// 语义层规则（executor-id 数字等）由 Validate 单独报告，便于编辑场景区分。
func Parse(cmd string) (*Parsed, error) {
	tokens, err := Tokenize(cmd)
	if err != nil {
		return nil, err
	}
	prefix, rest, err := parsePrefix(tokens)
	if err != nil {
		return nil, err
	}
	// Passthrough 恒非 nil：JSON 序列化为 [] 而非 null（parsed.passthrough 契约；
	// v0.1.0 曾因 null 导致 Web 详情页 .length 崩溃）
	p := &Parsed{Prefix: prefix, Passthrough: []string{}}
	var issues FieldErrors

	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		if !strings.HasPrefix(tok, "-") {
			// 多余的裸词（非 flag）：原样进 passthrough 保序
			p.Passthrough = append(p.Passthrough, tok)
			continue
		}
		name := strings.TrimPrefix(tok, "-")
		name = strings.TrimPrefix(name, "-") // 兼容单横线形式
		value := ""
		hasValue := false
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			value = name[idx+1:]
			name = name[:idx]
			hasValue = true
		}

		switch name {
		case "ws-url", "token", "executor-id", "provider", "model",
			"reasoning-effort", "context-window", "memory-mode", "max-tasks":
			if !hasValue {
				if i+1 >= len(rest) || strings.HasPrefix(rest[i+1], "-") {
					issues = append(issues, FieldIssue{Flag: name, Problem: "缺少参数值"})
					continue
				}
				i++
				value = rest[i]
			}
			dst := p.valueTarget(name)
			*dst = value
		case "token-aware-enable", "debug":
			v := true
			if hasValue {
				switch value {
				case "true", "":
					v = true
				case "false":
					v = false
				default:
					issues = append(issues, FieldIssue{Flag: name, Problem: fmt.Sprintf("布尔 flag 不接受值 %q", value)})
					continue
				}
			}
			if name == "token-aware-enable" {
				p.TokenAware = v
			} else {
				p.Debug = v
			}
		default:
			// 未知 flag：原样保序（含 --flag=value 形态）
			p.Passthrough = append(p.Passthrough, tok)
		}
	}

	if len(issues) > 0 {
		return nil, issues
	}
	return p, nil
}

// valueTarget 返回值型 flag 对应的字段地址。
func (p *Parsed) valueTarget(name string) *string {
	switch name {
	case "ws-url":
		return &p.WSURL
	case "token":
		return &p.Token
	case "executor-id":
		return &p.ExecutorID
	case "provider":
		return &p.Provider
	case "model":
		return &p.Model
	case "reasoning-effort":
		return &p.ReasoningEffort
	case "context-window":
		return &p.ContextWindow
	case "memory-mode":
		return &p.MemoryMode
	case "max-tasks":
		return &p.MaxTasks
	}
	return nil // 不可能到达：调用方已按白名单分发
}

// Providers 是 provider 允许的枚举值。
var Providers = []string{"qoder", "qodercn", "claude", "codex"}

// Validate 执行 §5.3 语义校验：executor-id 数字、token 非空、provider 枚举。
func (p *Parsed) Validate() error {
	var issues FieldErrors
	if p.ExecutorID == "" {
		issues = append(issues, FieldIssue{Flag: "executor-id", Problem: "缺失（应为数字）"})
	} else if !allDigits(p.ExecutorID) {
		issues = append(issues, FieldIssue{Flag: "executor-id", Problem: fmt.Sprintf("%q 不是数字", p.ExecutorID)})
	}
	if p.Token == "" {
		issues = append(issues, FieldIssue{Flag: "token", Problem: "缺失或为空"})
	}
	if !inList(p.Provider, Providers) {
		issues = append(issues, FieldIssue{Flag: "provider",
			Problem: fmt.Sprintf("%q 不在允许集合 {qoder, qodercn, claude, codex}（须显式指定）", p.Provider)})
	}
	if len(issues) > 0 {
		return issues
	}
	return nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func inList(s string, list []string) bool {
	for _, v := range list {
		if s == v {
			return true
		}
	}
	return false
}
