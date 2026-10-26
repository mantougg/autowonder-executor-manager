# autowonder-project-onboarding

把你的项目接入 [AutoWonder](https://github.com/aliyun/alibabacloud-landing-zone) 平台的 AI agent skill：
自动完成小队/数字人/SDLC 初始化、仓库与权限配置、执行器注册上线，并指导接入后的
工单交付日常使用。由项目侧的 AI agent（ZCode / Qoder / Claude Code 等）加载执行。

**不含任何环境信息**——服务地址、MCP 配置、空间、模型等全部在运行时向你询问或从平台动态获取。

## 安装

| 环境 | 放置位置 |
|---|---|
| ZCode / 通用 agents | `~/.agents/skills/autowonder-project-onboarding/`（用户级）或项目 `<repo>/.agents/skills/` |
| Claude Code | `~/.claude/skills/autowonder-project-onboarding/` |
| AutoWonder 平台技能 | `zip -r autowonder-project-onboarding.zip .`（根目录含 SKILL.md）后在平台「技能」页上传 |

## 使用

对你的 agent 说：

```
/autowonder-project-onboarding 把当前项目接入 AutoWonder
```

或自然语言：「帮我配置 XXX 项目接入 AutoWonder」「注册执行器」「AutoWonder 工单怎么交付」。

agent 会先向你收集：AutoWonder 服务地址与 MCP 配置、目标工作空间、模板选择、
项目数据库/部署/分支变量、仓库清单、执行器机器环境，然后按
[SKILL.md](SKILL.md) → [references/onboarding.md](references/onboarding.md) 执行对接，
之后按 [references/delivery-usage.md](references/delivery-usage.md) 指导日常交付。

## 目录

```
SKILL.md                        入口：信息收集清单 + 流程编排 + 坑速查表
references/onboarding.md        对接六步详细手册
references/delivery-usage.md    日常使用手册（工单/澄清/交付/跟进）
references/troubleshooting.md   排障手册
scripts/aw_rpc.py               MCP 原始 JSON-RPC 客户端（批量写/降级通道）
scripts/bulk_init.py            模板批量初始化（含结构校验与回读断言）
```

执行器管理已切换为 **[awem](https://github.com/mantougg/awem)**
（本机执行器管理器：配置化纳管、崩溃自动拉起、日志滚动、系统服务自启）；
旧的 `executor-manager.sh` 模板已移除，存量机器用 `awem import` 迁移
（见 references/troubleshooting.md）。

## 前置条件

- 一个可访问的 AutoWonder 实例（Web 地址 + MCP 配置，向管理员获取）
- 执行器机器：`qoder` 或 `qodercn` CLI 已安装并登录，Node.js ≥ 20，awem 已安装（`awem doctor` 一键体检）
- 接入模板来自 AutoWonder 公开仓库，无需额外授权
