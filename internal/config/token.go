// token.go 管理 data/api-token：首次生成随机 token（crypto/rand 32 字节、
// hex 编码 64 字符），权限 0600（Windows 下进一步收紧 ACL 到当前用户，
// 实现在 token_windows.go）；已存在则直接读取复用，不重新生成。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// APIToken 返回 API 鉴权 token：文件存在则读取；不存在（或为空）则生成并落盘。
// 供 daemon（T06 API 鉴权）与 CLI（自动读取，无需用户输入）使用。
func APIToken(p Paths) (string, error) {
	if b, err := os.ReadFile(p.TokenFile); err == nil {
		if tok := strings.TrimSpace(string(b)); tok != "" {
			return tok, nil
		}
		// 空文件视为未生成，走生成路径
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("读取 %s 失败：%w", p.TokenFile, err)
	}

	buf := make([]byte, 32) // 256 位熵
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成随机 token 失败：%w", err)
	}
	tok := hex.EncodeToString(buf)

	if err := os.MkdirAll(p.DataDir, dirPerm); err != nil {
		return "", fmt.Errorf("创建数据目录失败：%w", err)
	}
	if err := os.WriteFile(p.TokenFile, []byte(tok), 0o600); err != nil {
		return "", fmt.Errorf("写入 token 文件失败：%w", err)
	}
	if err := restrictTokenFilePerms(p.TokenFile); err != nil {
		return "", err
	}
	return tok, nil
}
