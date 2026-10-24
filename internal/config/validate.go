// validate.go 加载时配置校验（design.md §5.3 / D8）。
// 命令字段深度校验（executor-id 数字 / token 非空 / provider 白名单）
// 依赖 T03 的启动命令解析器，待其完成后接入 Validate（已登记 T02 变更记录）。
package config

import (
	"fmt"
	"strings"
)

// nameAllowed 报告 name 是否符合 D8 字符集 [a-zA-Z0-9._-]。
func nameAllowed(name string) bool {
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

// Validate 校验整个配置；失败时逐条报告（条目定位 + 中文错误），
// 返回的 error 聚合全部问题（不允许半份配置生效）。
func (c *Config) Validate() error {
	var errs []string

	if c.Server.Bind != "127.0.0.1" {
		errs = append(errs, fmt.Sprintf("[server] bind 仅允许 127.0.0.1（本地 API 不对外），当前 %q", c.Server.Bind))
	}
	if c.Server.Port < 0 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Sprintf("[server] port 越界：%d（0=自动，1-65535=固定）", c.Server.Port))
	}

	seen := make(map[string]bool, len(c.Executors))
	for i, e := range c.Executors {
		loc := fmt.Sprintf("executor[%d]", i)
		if e.Name != "" {
			loc = fmt.Sprintf("executor[%d]（name=%q）", i, e.Name)
		}
		switch {
		case e.Name == "":
			errs = append(errs, loc+"：name 不能为空")
		case !nameAllowed(e.Name):
			errs = append(errs, loc+"：name 含非法字符，只允许 [a-zA-Z0-9._-]（中文名不支持，D8：用于文件名与 CLI 参数安全）")
		case seen[e.Name]:
			errs = append(errs, loc+"：name 与其他执行器重复（须全局唯一）")
		default:
			seen[e.Name] = true
		}
		if strings.TrimSpace(e.Command) == "" {
			errs = append(errs, loc+"：command 不能为空")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("配置校验失败，共 %d 项问题：\n- %s", len(errs), strings.Join(errs, "\n- "))
	}
	return nil
}
