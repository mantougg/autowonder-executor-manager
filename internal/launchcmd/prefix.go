// parse.go：前缀识别（F1）。识别 `npx [-y] autowonder@<ver> connect …`
// 前缀结构，提取 runtime 版本；容忍用户粘贴的 nohup/& 包装残留。
package launchcmd

import (
	"fmt"
	"strings"
)

// UnsupportedCommandError 表示命令前缀不受支持（非 npx autowonder connect）。
type UnsupportedCommandError struct {
	Reason string
}

func (e *UnsupportedCommandError) Error() string { return "不支持的命令：" + e.Reason }

// Prefix 是前缀解析结果。
type Prefix struct {
	RuntimeVersion string // autowonder@<ver> 中的 <ver>
}

// stripWrappers 去掉粘贴残留的包装词：前导 nohup（可叠加）与尾部 &。
// 旧脚本/终端习惯常见形态：`nohup npx … connect … &`。
func stripWrappers(tokens []string) []string {
	for len(tokens) > 0 && (tokens[0] == "nohup" || tokens[0] == "\\nohup") {
		tokens = tokens[1:]
	}
	for len(tokens) > 0 && tokens[len(tokens)-1] == "&" {
		tokens = tokens[:len(tokens)-1]
	}
	return tokens
}

// parsePrefix 识别 `npx [-y] autowonder@<ver> connect` 前缀，
// 返回前缀信息与 connect 之后的参数词列表。
// npx 与包名之间只允许 -y / --yes（其余 npx 参数一律报不支持，避免静默丢弃）。
func parsePrefix(tokens []string) (Prefix, []string, error) {
	tokens = stripWrappers(tokens)
	if len(tokens) == 0 {
		return Prefix{}, nil, &UnsupportedCommandError{Reason: "命令为空"}
	}
	if tokens[0] != "npx" {
		return Prefix{}, nil, &UnsupportedCommandError{
			Reason: fmt.Sprintf("开头必须是 npx（粘贴的完整命令以 %q 开头）", tokens[0]),
		}
	}
	rest := tokens[1:]
	// npx 自身 flag：仅接受 -y / --yes
	for len(rest) > 0 && (rest[0] == "-y" || rest[0] == "--yes") {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return Prefix{}, nil, &UnsupportedCommandError{Reason: "npx 后缺少 autowonder@<版本> connect"}
	}
	pkg := rest[0]
	if pkg == "autowonder" || strings.HasPrefix(pkg, "autowonder@") {
		ver := ""
		if idx := strings.IndexByte(pkg, '@'); idx >= 0 {
			ver = pkg[idx+1:]
			if ver == "" {
				return Prefix{}, nil, &UnsupportedCommandError{Reason: "autowonder@ 后缺少版本号"}
			}
		} else {
			return Prefix{}, nil, &UnsupportedCommandError{Reason: "缺少版本号（应为 autowonder@<版本>）"}
		}
		rest = rest[1:]
		if len(rest) == 0 || rest[0] != "connect" {
			return Prefix{}, nil, &UnsupportedCommandError{Reason: "autowonder@<版本> 后必须是 connect 子命令"}
		}
		return Prefix{RuntimeVersion: ver}, rest[1:], nil
	}
	return Prefix{}, nil, &UnsupportedCommandError{
		Reason: fmt.Sprintf("必须是 autowonder@<版本> 包名（粘贴的为 %q）", pkg),
	}
}
