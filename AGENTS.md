# AGENTS.md — 给在本仓库工作的 AI Agent

## 项目是什么

**awem**（AutoWonder Executor Manager）：跨平台（Win/Linux/macOS）的本机 AutoWonder 执行器管理应用。
单一 Go 二进制 = 常驻 daemon + 内嵌 Web 页 + CLI 子命令；AI 用 CLI（`--json`）维护，人用 Web 页维护，
统一替代此前"每台机器一个 bash 脚本手工管理执行器"的分散方式。

## 当前状态

- **v1 功能开发完成（2026-10-25）：70/71 功能点检查通过**，实时进度见
  [docs/task-awem-dev/index.md](docs/task-awem-dev/index.md)。余量：T10 的
  macOS/Windows 真机验证（⏸️）、T11-F4 十六条决策核对表待用户复核。
- 唯一设计基准：[docs/design/design.md](docs/design/design.md)（16 条核心决策 + D1–D10 默认项，全部经用户确认）。
- **实现与设计文档冲突时，先改文档、征得用户同意，再改代码。**
- 推进开发/检查用 `awem-dev` 技能（`.agents/skills/awem-dev/`）；全链路冒烟
  `scripts/smoke.sh`；发布打 `v*` tag 触发 workflow。
- `docs/task-awem-dev/conventions.md` 的状态流转规则**仅适用于任务计划（T01–T11）的执行与检查**；
  日常功能开发与问题修复按「提交前检查清单」执行，不维护任务状态表。

## 30 秒架构

```
CLI / Web SPA ──HTTP 127.0.0.1+token──▶ daemon（internal/api）
                                            │ supervisor：spawn/进程树终止/崩溃拉起
                                            ▼
                                     npx -y autowonder@<ver> connect …（执行器子进程）
config.toml 文件监视热加载 ◀── daemon；logs 收割执行器日志 + events.log；
platform 只读轮询平台 ONLINE/BUSY/OFFLINE + 漂移检测。
```

- CLI 不直改状态，一切走本地 API；daemon 未运行时 CLI 自动拉起。
- daemon 是唯一持有状态的进程；配置单源是 `config.toml`。

## 改哪里（速查）

| 要做什么 | 动的位置 |
|---|---|
| CLI 子命令 | `internal/cli/`（注册在 `commands.go`） |
| 启动命令字段解析 | `internal/launchcmd/`（纯函数库，roundtrip 不丢未知 flag） |
| API 端点 / JSON 视图 | `internal/api/server.go` + `schema.go` |
| 进程守护行为 | `internal/supervisor/`（平台差异成对改 `_unix.go`/`_windows.go`） |
| 配置字段 | `internal/config/`（model + validate + default_config + toml_io） |
| Web 页面 | `web/src/pages|components/`，契约变则同步 `web/src/api.ts` + 后端 `schema.go` |
| 平台联动 | `internal/platform/`（只读红线） |
| 系统服务 | `internal/service/` |

完整代码地图、隐性契约（退出码 / `--json` 兼容 / 掩码 / 测试隔离）、排障路径
与常见坑见 [docs/dev-guide.md](docs/dev-guide.md)——**开发/修复前先读它**。

## 提交前检查清单

1. 三绿：`go build ./... && go vet ./... && go test ./...`（改了 Web 另跑
   `cd web && npm run build`，含 `tsc --noEmit` 类型门）。
2. 全链路冒烟：`scripts/smoke.sh`（无 systemd 环境用 `--skip-service`）。
3. 设计一致性：实现与 [design.md](docs/design/design.md) 冲突时先改文档、征得同意再改代码；
   UI 元素先符合 [ui-guidelines.md](docs/design/ui-guidelines.md)。
4. 文档同步：改了本文档/design.md/ui-guidelines.md 描述的行为，同步更新对应文档；
   改了包职责或入口，同步该包 `doc.go` 与 [docs/dev-guide.md](docs/dev-guide.md) 代码地图。

## 文档约定

所有文档放 `docs/`，按类型分目录：

| 目录 | 用途 |
|---|---|
| `docs/design/` | 设计文档（架构、决策、协议；Web UI 视觉/交互规范见 [ui-guidelines.md](docs/design/ui-guidelines.md)） |
| `docs/plan/` | 计划文档（里程碑、任务拆解、实施计划） |
| `docs/requirements/` | 需求文档 |

文档用中文写；文件名用小写英文与连字符（如 `v1-milestone-plan.md`）。

## 红线（不可越界，见设计文档 §2 非目标）

- 不做任务派发；不做多机聚合；不做平台**写**操作（v1 对平台只读）。
- v1 不做内置 MCP server（v1.5 再加）；launchConfig 回写留 v2。
- 本地 API 只绑 127.0.0.1，不提供多用户/远程访问。

## 发布约定

- 版本 semver，自 **v0.1.0** 起；打 `v*` tag 推送即触发 release workflow 自动发布（五平台 + checksums）。
- **tag 必须是 annotated tag，且 message 为与上一个 tag 之间的变更总结与列举**
  （首个 tag 相对仓库起点）：按子任务/功能块组织，先一句话总结再逐条列举
  （完成了什么功能 / 修复了什么问题 / 优化了什么内容）；末尾注明已知余量/
  未验证项。禁止裸 tag（`git tag v0.1.0`）与空 message。
  示例：`git tag -a v0.1.0 -m "$(cat TAGNOTES.md)"`。
- **GitHub Release 的正文即 tag 的 message**（workflow 自动提取，不用
  generate_release_notes 的链接式自动摘要）；workflow 运行标题显示版本名
  （`run-name`），而非触发提交的 message。

## 技术约定

- Go 单 module + `go:embed` 内嵌 React SPA；二进制名 `awem`。
- 预期目录：`cmd/awem/`、`internal/`（supervisor / config / platform / api / logs）、`web/`。
- 复用上游 npm 包命令：直接 spawn `npx -y autowonder@<ver> connect …`，不依赖其 daemon。
- 配置：OS 惯例目录 `awem/`（`config.toml` + `data/`），TOML 人类可读、热加载。
- 退出码：0 成功 / 2 用法 / 3 未找到 / 4 已存在 / 5 daemon 未运行 / 1 其他。
- 注释与文档中文，标识符英文；单测 + 假长驻子进程冒烟测试。
- Web UI 视觉/交互以 [docs/design/ui-guidelines.md](docs/design/ui-guidelines.md) 为准：设计令牌（`--awem-*` CSS 变量）、状态色语义、组件规范；新增 UI 元素先入规范再实现。

## 关键外部参考

- 上游开源镜像与运行时文档：https://github.com/aliyun/alibabacloud-landing-zone/tree/master/ai-sdlc/auto-wonder
  （`docs/runtime-usage.md`、`docs/scheduler-executor-protocol.md`）
- 联动接入技能（本仓库 `skills/autowonder-project-onboarding/`，可用 `npx skills` 安装）
  （执行器纳管已切换为 awem：`awem add/start` 上线、`awem import` 迁移旧脚本）
