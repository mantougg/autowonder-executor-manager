# awem Agent 使用说明

面向 AI Agent 的执行器维护指南：用 `awem` CLI（`--json` 机器格式）完成执行器的
添加、启停、排障与清理。人是第一读者的文档见 [README](../README.md)；
命令面/schema 基准见 [T07 变更记录的 --json 汇总表](../task-awem-dev/t07-cli.md)。

## 0. 心智模型

- 单机单 daemon：所有 CLI 命令打本地 API（127.0.0.1），daemon 未运行时**自动拉起**
  （无需关心 daemon 生命周期；服务化用 `awem service install`）。
- 执行器 = 一条 `npx -y autowonder@<版本> connect …` 命令的结构化配置；粘贴整条
  命令添加，字段可编辑，未知 flag 保留。
- 一切破坏性操作（remove、stop 全部）需 `--yes` 或交互确认；Agent 一律加 `--yes`。

## 1. 新机器从零到执行器 ONLINE

```bash
# ① 安装（任选其一；或 GitHub Releases 手动下载）
curl -fsSL <repo>/scripts/install.sh | sh -s -- v0.1.0

# ② 环境体检（Node≥20 / npx / qoder CLI / 磁盘；任一 ❌ 退出码 1）
awem doctor --json

# ③（推荐 7×24）注册系统服务：daemon 随系统常驻
awem service install

# ④ 添加执行器：粘贴平台生成的整条启动命令（名称缺省 exec-<executor-id>）
awem add "npx -y autowonder@0.3.22 connect --ws-url ws://<内网地址>/ws/executor \
  --token <executor-token> --executor-id 10086 --provider qoder --model qfmodel \
  --reasoning-effort medium --context-window 1000000 --token-aware-enable" --json

# ⑤ 启动并确认（本地进程 running + 日志连接状态 connected + 平台 ONLINE）
awem start exec-10086
awem status exec-10086
awem log exec-10086 --lines=50

# ⑥（可选）随 daemon 自动拉起
awem edit exec-10086 --autostart --json
```

`status` 的三层状态语义：**本地进程态**（running/stopped/backoff…）与**平台状态**
（ONLINE/BUSY/OFFLINE，需配置 `[platform]` 段）并列展示；本地进程活着 ≠ 平台
ONLINE（心跳超时判定在平台侧）。

## 2. 全部子命令速查

| 命令 | 说明 | 关键参数 |
|---|---|---|
| `awem add "<命令>"` | 粘贴整条命令添加 | `--name`、`--autostart`、`--start`、`--no-restart`、`--debug`、`--file <路径>`、`--workspace-root` |
| `awem list` | 全部执行器表格 | `--json`（与 API 视图同构） |
| `awem status [名称]` | 单个详情 / 全部摘要 | 无名称=全部 |
| `awem start/stop/restart [名称]` | 动作（无名称=全部） | stop 全部需 `--yes` |
| `awem edit <名称>` | 结构化字段编辑 | `--model`、`--reasoning-effort`、`--context-window`、`--memory-mode`、`--max-tasks`、`--token`、`--ws-url`、`--executor-id`、`--provider`、`--runtime-version`、`--autostart[=bool]`、`--restart[=bool]`、`--debug[=bool]`、`--command "<新命令>"` |
| `awem remove <名称>` | 删除配置（运行中先停） | `--purge`（同时删日志）、`--yes` |
| `awem log <名称>` | 日志尾部/跟随 | `-f`（WS 实时）、`--lines=N`（默认 100） |
| `awem import` | 旧 executor-manager.sh 导入 | `--file=<路径>`、`--with-logs`、`--yes` |
| `awem doctor` | 环境体检（5 项） | 任一 ❌ 退出码 1 |
| `awem service install/uninstall/status` | 系统服务 | Linux systemd user / macOS launchd / Windows 登录自启 |
| `awem reload` | 手动触发配置重载 | 正常由文件监视自动完成 |
| `awem version` | 版本与构建信息 | |
| `awem daemon` | 前台运行 daemon（排障用） | CLI 会自动拉起，一般无需手跑 |

## 3. `--json` schema（稳定契约）

- 顶层结构：成功 = 各命令的视图对象（见下）；失败 = `{"error":{"code":<int>,"type":"<机器码>","message":"<中文>"}}`。
- **executor 视图**（list/status/add/edit/start 等共用）：
  `{name, command, autostart, workspace_root, restart:{enabled,backoff},
  parsed:{runtime_version, ws_url, token(掩码), executor_id, provider, model,
  reasoning_effort, context_window, memory_mode, max_tasks, token_aware, debug,
  passthrough[]}, local:{state,pid,started_at,uptime_ms,restarts,backoff_delay_ms,
  conn,last_exit}, platform:{status,last_heartbeat}|null, drift:{fields:[{field,
  local,remote}],message}|null}`
- 其他：`status`（daemon）= `{version,pid,port,started_at,uptime_ms,platform:{configured,connected}}`；
  `remove` = `{removed,stopped,purged}`；`log` = `{name,tail,lines[]}`；
  `reload` = `{reloaded,changed}`；`doctor` = `[{name,ok,detail,advice,skipped}]`。
- **掩码语义**：读取一律掩码（`abcd***wxyz`）；编辑时提交掩码原值 = 保持不变。
- 完整字段与示例见 [T07 变更记录](../task-awem-dev/t07-cli.md)（本表基准）。

## 4. 退出码

| 码 | 含义 | Agent 处置 |
|---|---|---|
| 0 | 成功 | — |
| 1 | 一般错误（含部分失败、doctor 不过） | 读 stderr/`error.message` 定位 |
| 2 | 用法错误（坏命令/参数/字段校验） | 修正输入重试，不要原样重放 |
| 3 | 未找到（执行器名不存在） | 先 `awem list` 对齐名称 |
| 4 | 已存在（add 同名） | 换名或先 `awem remove` |
| 5 | daemon 未运行且无法自动拉起 | 按提示跑 `awem daemon` 前台排障（stderr 见 data/logs/daemon.stderr） |

## 5. 典型工作流

**排障一个执行器**：`awem status <名>`（看本地态/重启次数/连接状态）→
`awem log <名> --lines=200`（崩溃前的日志）→ 事件日志
`~/.config/awem/data/logs/events.log`（启停/崩溃/拉起时间线）→ 平台侧不
ONLINE 时查 `platform.status` 与漂移。

**改模型**：`awem edit <名> --model <新模型> --json`（运行中的会提示需
`awem restart <名>` 生效；passthrough 未知 flag 不丢）。

**清理**：`awem stop --yes`（全部）→ `awem remove <名> --purge --yes`（配置+日志）。

**导入存量**：`awem import --file=<旧脚本> --yes --json`（中文名自动改
`exec-<id>` 并在 results 标 `renamed`；`--with-logs` 归档旧日志）。

## 6. 常见错误与处置

| 现象 | 处置 |
|---|---|
| 退出码 5 | daemon 拉起失败：`awem daemon` 前台看报错（常见：config.toml 校验失败）；持续需要则 `awem service install` |
| `平台连通 ❌` | 检查 `[platform]`（base_url/mcp_token）：`awem status` 看 platform.configured；改配置后 daemon 热加载生效 |
| `drift` 漂移黄标 | 本地解析字段与平台 launchConfig 不一致：`awem edit` 对齐后 `restart`，或平台配置已变按平台为准（v1 不回写） |
| `local.conn=reconnecting` | connect 断线重连中（ws-url 不通/平台重启）；持续则查网络与 `--ws-url` |
| `state=backoff` | 崩溃自动拉起退避中（指数 1s→5m，稳定 10m 重置）；频繁崩溃看日志 |

## 7. 安全边界（Agent 须知）

- 本地 API 仅 127.0.0.1；token 在 `<配置目录>/data/api-token`（0600）。
- awem 对平台**只读**（list_projects / list_executors / get_executor_launch_config
  白名单）；写平台操作请走平台自己的通道。
- 日志与 `--json` 输出中的执行器 token / MCP 令牌恒为掩码；明文只在 config.toml
  （0600）与 data/api-token。
