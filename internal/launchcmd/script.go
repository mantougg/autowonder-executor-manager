// script.go：旧 executor-manager.sh 脚本解析（F4）。
// 目标形态（真实模板见 testdata/executor-manager.sh）：
//
//	EXECUTORS=(
//	  "名称|npx -y autowonder@<ver> connect …"
//	)
//
// 名称常为中文，违反 D8 字符集 [a-zA-Z0-9._-]，导入必须走改名流程
// （给出建议名并标记 NeedsRename，不直接失败）。
package launchcmd

import (
	"fmt"
	"regexp"
	"strings"
)

// Imported 是旧脚本中的一条导入条目。
type Imported struct {
	Name          string // 原名（脚本里的名称）
	Command       string // 原始命令串（保真，供预览）
	SuggestedName string // 原名不合规时的建议名（合规时为空）
	NeedsRename   bool   // 原名不合规，导入前需确认改名
	ParseErr      error  // 命令解析/校验问题（占位符未替换等），不静默导入
}

// ValidExecutorName 报告名称是否符合 D8 字符集 [a-zA-Z0-9._-]（非空）。
func ValidExecutorName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

// placeholderRe 匹配未替换的模板占位符，如 <runtimeVersion>、<id>、<内网地址>。
var placeholderRe = regexp.MustCompile(`<[^<>]+>`)

// ParseScript 解析旧脚本内容。逐行产出 Imported：
// 注释行、空行跳过；含未替换占位符的示例行标记 ParseErr（占位符）而非静默导入；
// 中文名生成建议名（策略：优先 `exec-<executor-id>`，解析失败时 `imported-<序号>`）。
func ParseScript(content string) ([]Imported, error) {
	lines := strings.Split(content, "\n")
	inArray := false
	found := false
	var out []Imported
	usedNames := map[string]bool{}

	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if !inArray {
			if strings.HasPrefix(trimmed, "EXECUTORS=(") {
				inArray = true
				found = true
				// 行内直接闭合的空数组：EXECUTORS=()
				if strings.HasSuffix(trimmed, ")") && len(trimmed) > len("EXECUTORS=(") {
					break
				}
			}
			continue
		}
		if strings.HasPrefix(trimmed, ")") {
			break
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		entry := strings.TrimSpace(stripQuotes(trimmed))
		if entry == "" {
			continue
		}
		name, command, ok := strings.Cut(entry, "|")
		if !ok {
			out = append(out, Imported{
				Name:     strings.TrimSpace(name),
				Command:  entry,
				ParseErr: fmt.Errorf("条目缺少 | 分隔符（应为 名称|启动命令）"),
			})
			continue
		}
		name = strings.TrimSpace(name)
		command = strings.TrimSpace(command)

		imp := Imported{Name: name, Command: command}

		// 占位符检测：模板示例行的 <runtimeVersion> 等未替换则不可导入
		if placeholderRe.MatchString(command) {
			imp.ParseErr = fmt.Errorf("命令含未替换的占位符（如 <runtimeVersion>），疑似模板示例行，未导入")
		} else if p, err := Parse(command); err != nil {
			imp.ParseErr = fmt.Errorf("命令解析失败：%w", err)
		} else if err := p.Validate(); err != nil {
			imp.ParseErr = fmt.Errorf("命令校验失败：%w", err)
		} else if !ValidExecutorName(name) {
			// 名称不合规但命令有效：给建议名，等用户确认改名
			imp.NeedsRename = true
			imp.SuggestedName = suggestName(name, p.ExecutorID, len(out)+1, usedNames)
		}
		out = append(out, imp)
	}

	if !found {
		return nil, fmt.Errorf("未找到 EXECUTORS=( … ) 数组（可能不是 executor-manager.sh 脚本）")
	}
	return out, nil
}

// stripQuotes 去掉条目两端成对的引号（单/双引号）。
func stripQuotes(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// suggestName 生成合规建议名：优先 exec-<executor-id>，否则 imported-<序号>；
// 与已用名冲突时追加 -2/-3…。原名里的 ASCII 合规字符尽量保留（exec-<id> 已足够定位）。
func suggestName(original, executorID string, seq int, used map[string]bool) string {
	base := ""
	if executorID != "" {
		base = "exec-" + executorID
	} else {
		base = fmt.Sprintf("imported-%d", seq)
	}
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	used[name] = true
	return name
}
