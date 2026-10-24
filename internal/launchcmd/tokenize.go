// tokenize.go 做 shell 风格拆词：支持单引号、双引号、反斜杠转义、
// 多空格/Tab/换行。语义与 bash 的词法规则一致（引号内不成为词边界）。
package launchcmd

import (
	"fmt"
	"strings"
)

// Tokenize 把命令字符串拆为 shell 语义的词列表。
func Tokenize(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	hasToken := false
	var quote rune // 0=无引号，'\'' 或 '"'

	flush := func() {
		if hasToken {
			tokens = append(tokens, cur.String())
			cur.Reset()
			hasToken = false
		}
	}

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case quote == '"':
			switch r {
			case '"':
				quote = 0
			case '\\':
				// 双引号内仅 " \ $ ` 为转义，其余反斜杠保持字面
				if i+1 < len(runes) {
					next := runes[i+1]
					if next == '"' || next == '\\' || next == '$' || next == '`' {
						cur.WriteRune(next)
						i++
					} else {
						cur.WriteRune(r)
					}
				} else {
					cur.WriteRune(r)
				}
			default:
				cur.WriteRune(r)
			}
		default: // 无引号
			switch r {
			case '\'':
				quote = '\''
				hasToken = true
			case '"':
				quote = '"'
				hasToken = true
			case '\\':
				if i+1 < len(runes) {
					cur.WriteRune(runes[i+1])
					i++
					hasToken = true
				} else {
					return nil, fmt.Errorf("命令以悬挂反斜杠结尾")
				}
			case ' ', '\t', '\n', '\r':
				flush()
			default:
				cur.WriteRune(r)
				hasToken = true
			}
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("引号未闭合：%c", quote)
	}
	flush()
	return tokens, nil
}
