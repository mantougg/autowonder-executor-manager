# awem 开发任务索引（task-awem-dev）

- **总任务状态**：⬜ 未开始 —— 0/11 子任务已检查，0/71 功能点已检查
- **计划依据**：[design.md](../design/design.md)（v1 定稿，2026-10-24）+ [AGENTS.md](../../AGENTS.md)
- **计划生成**：2026-10-24，拆解为 11 个子任务、71 个功能点
- **执行方式**：用 `awem-dev` 技能（仓库 `.agents/skills/awem-dev/`）说"继续 awem 开发/执行 T0X/检查 T0X"；或按 [conventions.md](conventions.md) §7–8 的流程手动执行与更新状态。

---

## 状态字典（全目录统一，详见 conventions.md §6）

| 标记 | 子任务/功能点开发状态 | 功能点检查状态 |
|---|---|---|
| ⬜ | 未开始 | 未检查 |
| 🔄 | 进行中 | — |
| ⏸️ | 已暂停（须记原因） | — |
| 🟠 | 已完成（待检查） | — |
| ✅ | 已检查 | 通过 |
| ❌ | — | 不通过（打回 🔄） |

## 子任务状态总览

| 编号 | 子任务 | 前置依赖 | 功能点进度（已检查/总数） | 子任务状态 | 文档 |
|---|---|---|---|---|---|
| T01 | 工程骨架与构建基础 | — | 2/5 | 🔄 进行中 | [t01-project-scaffold.md](t01-project-scaffold.md) |
| T02 | 配置与存储 | T01 | 0/6 | ⬜ 未开始 | [t02-config-storage.md](t02-config-storage.md) |
| T03 | 启动命令解析与导入 | T01 | 0/6 | ⬜ 未开始 | [t03-launch-command-parser.md](t03-launch-command-parser.md) |
| T04 | 日志系统 | T01、T02 | 0/6 | ⬜ 未开始 | [t04-logging.md](t04-logging.md) |
| T05 | Supervisor 进程守护 | T02、T03、T04 | 0/7 | ⬜ 未开始 | [t05-supervisor.md](t05-supervisor.md) |
| T06 | daemon 与本地 API | T02、T04、T05 | 0/7 | ⬜ 未开始 | [t06-daemon-api.md](t06-daemon-api.md) |
| T07 | CLI 子命令集 | T03、T05、T06 | 0/10 | ⬜ 未开始 | [t07-cli.md](t07-cli.md) |
| T08 | 平台状态联动（只读） | T02、T03、T06 | 0/6 | ⬜ 未开始 | [t08-platform-sync.md](t08-platform-sync.md) |
| T09 | Web SPA | T03、T06、T08 | 0/8 | ⬜ 未开始 | [t09-web-spa.md](t09-web-spa.md) |
| T10 | 系统服务与跨平台专项 | T06、T07 | 0/6 | ⬜ 未开始 | [t10-service-cross-platform.md](t10-service-cross-platform.md) |
| T11 | 打包分发与 Agent 文档 | T01–T10 | 0/4 | ⬜ 未开始 | [t11-release-docs.md](t11-release-docs.md) |

## 执行顺序与并行说明

- **推荐顺序 = 编号顺序**：T01 → T02 → T03 → T04 → T05 → T06 → T07 → T08 → T09 → T10 → T11。
- 可并行的组合（同一人/Agent 串行执行时忽略）：T07 与 T08 在 T06 完成后可并行；T09 在 T06 完成后可开工，但"平台状态列/漂移黄标"要等 T08。
- T11 是收尾任务，必须在 T01–T10 全部 ✅ 后执行。

## 执行流程摘要（详细规则见 conventions.md §7–8）

1. 读本索引 + conventions.md；涉 UI 再读 [ui-guidelines.md](../design/ui-guidelines.md)。
2. 选定子任务（用户指定编号，或取推荐顺序中第一个非 ✅ 的）。
3. 开工：子任务状态 ⬜→🔄（本文档与子任务文档同步 + 双方变更记录各加一行）。
4. 逐功能点开发：完成一个 → 该功能点 🟠（index 进度同步）。
5. 全部功能点 🟠 且 `go build/vet/test` 三绿 → 子任务 🟠。
6. 检查环节：逐功能点跑验收要点 → 全 ✅ → 子任务 ✅；不通过 → 打回并登记。
7. 全部子任务 ✅ → 总任务状态改为 ✅。

## 仓库外联动事项（不在本目录跟踪状态）

- 改造 `~/.agents/skills/autowonder-project-onboarding` 技能模板：从生成 `executor-manager.sh` 改为生成 `awem add --launch-command …` 指引（独立仓库，见 design.md §15 本期交付第 3 项）。**在 T11 完成后另行处理。**

## 更新记录

| 日期 | 动作 | 说明 |
|---|---|---|
| 2026-10-24 | 创建 | 依据 design.md v1 定稿拆解：11 子任务 / 71 功能点 |
| 2026-10-24 | T01 开工 | 子任务状态 ⬜→🔄，无人值守开发会话开始 |
| 2026-10-24 | T01-F1 ✅ | module 初始化与目录骨架，三绿通过（1/5） |
| 2026-10-24 | T01-F2 ✅ | 子命令分发框架 + version，验收实测通过（2/5） |
