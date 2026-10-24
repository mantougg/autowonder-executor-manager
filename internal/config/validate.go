// validate.go 加载时配置校验（design.md §5.3 / D8）。
// 名称规则与命令字段校验复用 internal/launchcmd（T03 解析器），
// 保证"配置校验"与"命令解析"两处规则同源。
package config

import (
	"fmt"
	"strings"

	"awem/internal/launchcmd"
)

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
		case !launchcmd.ValidExecutorName(e.Name):
			errs = append(errs, loc+"：name 含非法字符，只允许 [a-zA-Z0-9._-]（中文名不支持，D8：用于文件名与 CLI 参数安全）")
		case seen[e.Name]:
			errs = append(errs, loc+"：name 与其他执行器重复（须全局唯一）")
		default:
			seen[e.Name] = true
		}

		switch {
		case strings.TrimSpace(e.Command) == "":
			errs = append(errs, loc+"：command 不能为空")
		default:
			// 命令字段深度校验（§5.3）：executor-id 数字 / token 非空 / provider 枚举
			p, err := launchcmd.Parse(e.Command)
			if err != nil {
				errs = append(errs, loc+"：command 无法解析（"+err.Error()+"）")
			} else if verr := p.Validate(); verr != nil {
				errs = append(errs, loc+"：command 校验不通过（"+verr.Error()+"）")
			}
		}

		// workspace_root 可选；配了就不能是空白（空白意味着"想覆盖但没填路径"，按错误处理）
		if e.WorkspaceRoot != "" && strings.TrimSpace(e.WorkspaceRoot) == "" {
			errs = append(errs, loc+"：workspace_root 不能为空白字符串（留空=用默认值）")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("配置校验失败，共 %d 项问题：\n- %s", len(errs), strings.Join(errs, "\n- "))
	}
	return nil
}
