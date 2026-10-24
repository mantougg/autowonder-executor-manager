# awem · AutoWonder Executor Manager

> 跨平台（Windows / Linux / macOS）的本机 AutoWonder 执行器管理应用。
> 单一 Go 二进制 = 常驻 daemon + 内嵌 Web 页 + CLI 子命令；AI 用 CLI 维护，人用 Web 页维护。

**状态**：🚧 设计定稿（2026-10-24），开发中 —— 进度见 [开发任务索引](docs/task-awem-dev/index.md)。

## 为什么需要 awem

AutoWonder 执行器本质是一个长驻进程（`npx -y autowonder@<ver> connect …`，WebSocket 连平台领任务）。此前每台机器靠一份手工 bash 脚本管理，痛点集中：

- 脚本分散、手工编辑、无统一视图；改模型 / 换内网地址要手改硬编码数组；
- bash 在 Windows 原生跑不了；无进程守护，崩溃无人拉起；日志无上限增长；
- 本地进程状态与平台 ONLINE 状态两头看，排障割裂。

awem 用**结构化配置 + 进程守护 + 统一视图**替代散落脚本。

## 功能一览（v1 规划）

| 能力 | 说明 |
|---|---|
| CLI | `add / list / status / start / stop / restart / edit / remove / log / import / doctor / service / reload / version`；全程 `--json` 稳定 schema、幂等、规范退出码（0/1/2/3/4/5） |
| Web 页 | 总览双状态列（本地进程 / 平台在线并列展示）、实时日志流、粘贴整条命令添加、设置 |
| 进程守护 | 崩溃自动拉起（指数退避 1s→5m）、进程树优雅终止、两层自启（`service install` + 每执行器 `autostart`） |
| 平台联动 | 个人 MCP 令牌只读轮询 ONLINE/BUSY/OFFLINE + 配置漂移标黄提示（v1 不回写平台） |
| 日志 | 每执行器独立文件、按大小滚动、保留策略 + 管理事件日志；`awem log -f` 与 Web 实时流 |
| 迁移 | `awem import` 解析旧 `executor-manager.sh` 脚本一键导入 |

**非目标（红线）**：不做任务派发；不做多机聚合面板；不做平台写操作（v1 只读）；v1 不做内置 MCP server；本地 API 仅绑 `127.0.0.1`。

## 当前状态与路线

- ✅ 设计定稿：[docs/design/design.md](docs/design/design.md)（16 条核心决策 + D1–D10 默认项，唯一实现基准）
- ✅ UI 视觉/交互规范：[docs/design/ui-guidelines.md](docs/design/ui-guidelines.md)
- ✅ 开发任务拆解：[docs/task-awem-dev/](docs/task-awem-dev/index.md)（11 个子任务 / 71 个功能点，含状态追踪与检查环节）
- 🚧 编码开发中（自 T01 起，按任务索引推进）

## 快速开始

> v0.1.0 尚未发布。规划发布方式：GitHub Releases 五平台产物（linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64）+ sha256 校验和 + `install.sh`（POSIX）/ `install.ps1`（PowerShell）一键安装。

## 文档导航

| 文档 | 内容 |
|---|---|
| [docs/design/design.md](docs/design/design.md) | 设计基准：架构、数据模型、CLI/API/Web、安全、跨平台、里程碑 |
| [docs/design/ui-guidelines.md](docs/design/ui-guidelines.md) | Web UI 设计令牌（`--awem-*`）、状态色语义、组件规范 |
| [docs/task-awem-dev/index.md](docs/task-awem-dev/index.md) | 开发任务索引与实时进度 |
| [docs/task-awem-dev/conventions.md](docs/task-awem-dev/conventions.md) | 开发任务共享规范（状态机、红线、检查环节） |
| [AGENTS.md](AGENTS.md) | 给 AI Agent 的仓库工作约定 |

## 目录结构（随开发推进落位）

```
cmd/awem/        # 入口
internal/        # config / launchcmd / logs / supervisor / platform / api / cli
web/             # React SPA 源码（构建产物 go:embed 内嵌进二进制）
docs/            # design / plan / requirements / task-awem-dev
.agents/skills/  # awem-dev 技能（按任务计划推进开发用）
```

## 开发者快速开始

环境要求：Go ≥ 1.26（当前稳定版）、make；前端阶段（T09）另需 Node.js ≥ 20。

```bash
make build                 # 构建单二进制 ./awem（自动注入版本号；web/ 未建时用占位页）
make build VERSION=v0.1.0  # 指定版本号构建（ldflags 注入，awem version 可见）
make test                  # go test 全量（单元 + 假长驻子进程冒烟）
make vet                   # go vet 静态检查
make clean                 # 清理构建产物
```

说明：

- 提交前三绿：`go build ./... && go vet ./... && go test ./...`（或 `make build test vet`）。
- `web/dist/` 约定**构建时生成、仓库忽略**：`make build` 会在缺失时从 `web/placeholder/`
  复制占位页，保证 fresh clone 可直接构建；`go:embed` 不读取 .gitignore，本地存在即可嵌入。

工程约定：注释与文档中文、标识符英文；**实现与设计冲突时，先改设计文档、征得同意、再改代码**；测试不依赖真实平台与 npx（假长驻子进程）。详见 [AGENTS.md](AGENTS.md)。

## 关联

- 上游开源镜像与运行时文档：[aliyun/alibabacloud-landing-zone · ai-sdlc/auto-wonder](https://github.com/aliyun/alibabacloud-landing-zone/tree/master/ai-sdlc/auto-wonder)

## 平台支持与 7×24 长跑建议

- **Linux**：完整支持（systemd user 服务，`awem service install`；不登录常驻
  需 `loginctl enable-linger <用户>`，install 输出会提示）。
- **macOS**：launchd LaunchAgent（RunAtLoad + KeepAlive）；实现已就绪，
  真机安装/卸载验证待 macOS 环境。
- **Windows**：默认登录自启（HKCU Run，无需管理员）；备选 `sc create`
  系统服务（需管理员，注意 session 0 限制：Node/qoder 环境变量须机器级）。
  实现已就绪，真机验证待 Windows 环境。
- **7×24 长跑场景推荐 Linux/macOS 或 WSL**（与平台最佳实践一致）：
  进程树终止、UTF-8 管道、npx.cmd 解析均已做平台适配（`GOOS=windows`
  交叉编译保证），但 Windows 原生路径未经真机回归。

## 冒烟测试

```bash
scripts/smoke.sh               # 三绿 + 交叉编译 + 前端构建 + Linux 服务全链路
scripts/smoke.sh --skip-service # 无 systemd 的环境只跑构建与测试层
```

## 发布

打 tag 推送即自动发布（`.github/workflows/release.yml`）：

```bash
git tag v0.1.0 && git push origin v0.1.0
# CI：前端构建 → go vet/test + UI 自检（质量门）→ make release 五平台 +
# checksums → 产物自检（sha256sum -c + 本机产物可执行）→ 创建 GitHub Release
```

- 版本含 `-`（如 `v0.1.0-rc1`）自动标记 prerelease；
- `workflow_dispatch` 手动触发为草稿演练（用输入的版本号，不真正发版）；
- 本地等价命令：`make release VERSION=v0.1.0`（产物与校验和在 `release/`）。
