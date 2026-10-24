# awem 开发指南

面向在 awem 仓库做功能开发 / 问题修复的工程师与 AI Agent：一张架构图、
一份代码地图、一张"改哪里"路由表、必须遵守的隐性契约与常见坑。

上位文档：[AGENTS.md](../AGENTS.md)（仓库工作约定）、
[design.md](design/design.md)（唯一设计基准）。本文是这两者的工程化索引，
不重复其条款；冲突时以上位文档为准。

## 1. 架构总览（30 秒）

```
┌─────────────┐   HTTP 127.0.0.1 + Bearer token    ┌──────────────────────────────┐
│ awem CLI    │ ──────────────────────────────────▶│ daemon（awem daemon，常驻）   │
│ (子命令进程) │ ◀────────── JSON / error{code…} ──│  internal/api（REST+WS+鉴权） │
└─────────────┘                                    │       │                        │
      ▲  CLI 未连上时自动拉起 daemon                │       ▼                        │
      │  （internal/daemon/client.go 惰性拉起）     │  internal/supervisor           │
┌─────────────┐   同一份 API + WS 日志流           │  spawn/stop 进程树/崩溃拉起    │
│ Web SPA     │ ──────────────────────────────────▶│       │                        │
│ (go:embed)  │                                    │       ▼                        │
└─────────────┘                                    │  npx -y autowonder@<ver>       │
                                                   │  connect …（执行器子进程）     │
                                                   └──────┬───────────┬─────────────┘
              config.toml 文件监视热加载 ◀────────────────┘           │
              （internal/config/watch.go）                            ▼
                                                   internal/logs：执行器日志滚动落盘
                                                   + events.log 管理事件（JSONL）
                                                   internal/platform：只读轮询平台
                                                   ONLINE/BUSY/OFFLINE + 漂移检测
```

要点：

- **CLI 不直接改状态**：一切数据操作打本地 API；测试里用 httptest 假 daemon
  替代（`cli.Deps.Connect` 注入）。
- **daemon 是唯一持有状态的进程**：supervisor、logs、platform syncer 都活在
  daemon 进程内；CLI/Web 都是无状态客户端。
- **配置单源**：`config.toml`（人可读）是唯一配置源，文件监视热加载，
  `awem reload` 只是手动触发同一条路径。

## 2. 代码地图

### Go 后端（`cmd/awem` + `internal/`）

| 包 | 职责 | 关键文件 | 关键类型 / 入口 |
|---|---|---|---|
| `cmd/awem` | 唯一入口：CLI/daemon/Web 同体 | `main.go` | 分发到 cli 或 daemon |
| `internal/cli` | 子命令注册分发、`--json/--yes`、错误→退出码 | `command.go`（Registry/GlobalFlags）、`commands.go`（Deps 注入与装配）、`add.go`/`edit.go`/`actions.go`/`status.go`/`log.go`/`import.go`/`doctor.go` | `Registry`、`Deps`、`RegisterCommands` |
| `internal/daemon` | daemon 生命周期 + CLI 端惰性拉起 | `daemon.go`（`Run`）、`client.go`（`Connect` 自动拉起）、`listen.go`（端口写回）、`lock*.go`（单实例）、`wsclient.go` | `Run`、`Connect`、`Client.FollowLogs` |
| `internal/api` | 本地 HTTP API（仅 127.0.0.1）+ WS 日志流 + 内嵌 SPA | `server.go`（路由/鉴权）、`schema.go`（JSON 视图契约）、`executors.go`、`settings.go`、`ws.go`、`static.go` | `Server`、`New(Options)`、`Handler()` |
| `internal/supervisor` | 执行器进程守护：spawn、进程树终止、崩溃拉起 | `supervisor.go`、`state.go`（状态机）、`npx*.go`（npx 解析）、`proc_*.go`（平台进程操作）、`connstate.go`（连接状态） | `Supervisor`、`New`、`ApplyConfig`、`StartExecutor/StopExecutor/RestartExecutor`、`Snapshot` |
| `internal/config` | config.toml 读写校验、目录布局、热加载、token | `model.go`（Config/Executor）、`validate.go`、`toml_io.go`、`paths.go`、`watch.go`（Watcher）、`default_config.go`、`token*.go`、`daemon*.go`（daemon.json） | `Config`、`Paths`、`DefaultLayout`、`Watcher` |
| `internal/launchcmd` | 启动命令解析/校验/重建（纯函数库，不碰进程网络） | `tokenize.go`、`argv.go`、`fields.go`（Parsed）、`render.go`（roundtrip）、`prefix.go`、`script.go`（旧脚本导入）、`archive.go` | `Parse`、`Parsed.Validate/Render`、`Argv` |
| `internal/logs` | 日志收割滚动、事件日志、follow 订阅、过期清理 | `manager.go`（Manager）、`sink.go`、`events.go`（EventLog/掩码）、`cleaner.go`、`debug.go` | `Manager`、`NewManager`、`Tail/ReadLines/Follow`、`EventLog`、`MaskSecret` |
| `internal/platform` | 平台只读联动：MCP 白名单调用、轮询、漂移、归属发现 | `platform.go`（Client）、`mcp.go`、`poller.go`（Start/Refresh）、`syncer.go`（Lookup/Summary）、`drift.go`、`discovery.go` | `Client`、`Service`（poller+syncer）、`NewService` |
| `internal/service` | 系统服务安装（systemd user / launchd / Windows 自启） | `service.go`（Platform 接口）、`systemd.go`、`launchd.go`、`startup.go` | `ForOS`、`Install/Uninstall/Status` |
| `internal/exitcode` | 退出码常量与错误类型（0/1/2/3/4/5） | `exitcode.go` | `Error`、`New/Usagef/NotFoundf/…`、`Code(err)` |
| `internal/version` | 版本号（ldflags 注入） | `version.go` | `Version` |
| `internal/fakeproc` | 假长驻子进程（测试共用，TestMain 构建，不进产品） | `main.go` | `-fake-*` 控制 flag（见文件头注释） |

### Web 前端（`web/src/`）

| 文件 | 职责 |
|---|---|
| `api.ts` | 统一 API 客户端：token 头注入、401 处理、`APIError`；schema 与后端 `internal/api/schema.go` 同构 |
| `App.tsx` / `router.ts` / `main.tsx` | 应用外壳与 hash 路由 |
| `pages/` | `Overview`（总览双状态列）、`Detail`（详情+实时日志）、`AddImport`（粘贴命令添加/导入）、`Settings`、`Login` |
| `components/` | `LogViewer`（ANSI 渲染+跟随滚动）、`StatusDot`、`Dialog`、`Toast` |
| `i18n.ts` | 文案集中管理（v1 全中文，按 key 预留多语言） |
| `styles/tokens.css` | `--awem-*` 设计令牌；`base.css`/`components.css` 只引用令牌 |

## 3. "改哪里"路由表

| 要做什么 | 动的文件（按顺序） | 别漏了 |
|---|---|---|
| 加/改 CLI 子命令 | `internal/cli/<cmd>.go` → `commands.go` 注册 → `command.go` usage | `--json` 输出走 `internal/api/schema.go` 视图；退出码用 `exitcode` 构造器 |
| 加/改启动命令字段（如新增 `--xxx`） | `internal/launchcmd/fields.go`（解析+校验）→ `render.go`（roundtrip）→ `matrix_test.go` | 未知 flag 必须保序透传（`passthrough`），不许解析失败 |
| 加 API 端点 | `internal/api/server.go` 路由 → handler → `schema.go` 视图 | 鉴权/CSRF 中间件默认覆盖；静态资源免鉴权 |
| 改 Web 页面 | `web/src/pages|components/` → `api.ts`（如契约变）→ `i18n.ts` 文案 | 颜色只用 `styles/tokens.css` 的 `--awem-*`；新组件先入 [ui-guidelines](design/ui-guidelines.md) |
| 加/改配置字段 | `internal/config/model.go` → `validate.go` → `default_config.go` → `toml_io.go` | 热加载路径（`watch.go`）要同步验证；design.md §5 是字段基准 |
| 改进程守护行为 | `internal/supervisor/supervisor.go` / `state.go` | 平台相关进程操作在 `proc_unix.go`/`proc_windows.go`、`npx_*.go` 成对改 |
| 改平台联动 | `internal/platform/`（poller/syncer/drift） | 只读红线：MCP 工具白名单在代码层强制（`whitelist_test.go`） |
| 改日志行为 | `internal/logs/manager.go` / `sink.go` / `events.go` | 输出与日志中的令牌恒掩码（`MaskSecret`） |

跨层改动（CLI flag → API → Web）按依赖顺序逐层改，每层跑该层测试。

## 4. 隐性契约（违反即 bug）

1. **退出码固定**：`0` 成功 / `2` 用法 / `3` 未找到 / `4` 已存在 / `5` daemon 未运行 / `1` 其他。
   新错误一律经 `internal/exitcode` 构造器产生，禁止 `os.Exit` 裸数字。
2. **`--json` schema 向后兼容**：只加字段，不改名/删字段；`passthrough` 恒为数组（可为空，不为 null）。
3. **掩码语义**：读取一律掩码（`abcd***wxyz`）；编辑时提交掩码原值 = 保持不变；日志/`--json` 输出绝不回显完整令牌。
4. **幂等**：重复 add 同名 → 退出码 4 不覆盖；start 已运行 → no-op 成功；破坏性操作（remove、stop 全部）必须 `--yes` 或交互确认。
5. **API 安全**：仅绑 127.0.0.1；token 头 + Host/Origin 校验防 CSRF；`data/api-token` 0600。
6. **平台只读**：只允许 `list_projects` / `list_executors` / `get_executor_launch_config` 三个 MCP 工具。
7. **测试隔离**：单测/冒烟不依赖真实平台与真实 npx——子进程一律用 `internal/fakeproc` 假二进制；CLI 测试用 httptest 假 daemon（`cli.Deps` 注入）。
8. **平台分文件**：OS 差异代码按 `_unix.go` / `_windows.go` 成对维护；`GOOS=windows`/`darwin` 交叉编译必须过。

## 5. 验证与排障

提交前（三绿 + 冒烟）：

```bash
go build ./... && go vet ./... && go test ./...
make build                       # 含前端构建 + 版本注入
scripts/smoke.sh                 # 全链路（无 systemd 环境用 --skip-service）
```

改 Web 后：`cd web && npm run build`（含 `tsc --noEmit` 类型门）；
UI 规范自检 `node scripts/verify-ui.mjs`（或走 smoke.sh）。

运行时排障路径（从外到内）：

1. `awem status <名>`：三层状态（本地进程态 / 连接状态 / 平台状态）+ 漂移。
2. `awem log <名> --lines=200`：执行器日志（崩溃前的输出）。
3. `data/logs/events.log`：管理事件时间线（启停/崩溃/拉起/配置变更，JSONL）。
4. `awem daemon` 前台运行：daemon 自身报错（拉起失败时 stderr 在 `data/logs/daemon.stderr`）。
5. 复现进测试：能用 fakeproc 复现的进程行为，补进对应包的 `_test.go`。

## 6. 常见坑

- **`web/dist/` 是构建产物且被 .gitignore**：`go:embed` 不读 .gitignore，本地存在即可嵌入；
  fresh clone 由 `make build` 从 `web/placeholder/` 复制占位页。不要手改 `web/dist/`。
- **daemon 自动拉起**：任一 CLI 命令都会惰性拉起 daemon——测试里不注意会连上真实
  daemon；CLI 测试必须注入 `Deps.Connect`。
- **编辑掩码字段**：`awem edit` 收到掩码值（`abcd***wxyz`）语义是"保持不变"，
  别在实现里把它当新值写回。
- **配置并发写**：API 并发写配置有测试守护（`TestAPI并发写不丢配置`）；
  改 config 写入路径后必跑 `internal/api` 与 `internal/config` 全量测试。
- **Windows 差异**：npx 解析为 `npx.cmd`、进程树终止用 `taskkill /T`、
  管道需 UTF-8 处理；只在 Linux 验证过的进程改动，至少要过 `GOOS=windows go build ./...`。
- **文档同步**：改了 AGENTS.md / design.md / ui-guidelines.md 描述的行为，
  同步更新对应文档本身（文档优先原则，见 AGENTS.md）。
