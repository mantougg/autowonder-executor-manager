# awem 设计文档（v1 定稿）

- **状态**：已定稿（2026-10-24 经 grilling 三轮问答达成共识）
- **定位**：awem 实施的唯一设计基准。实现与本文冲突时，先更新本文再改代码。
- **共识范围**：16 条核心决策 + D1–D10 次要默认项，全部经用户逐条确认。

---

## 1. 背景与问题

AutoWonder（多智能体软件交付平台，开源镜像位于
[aliyun/alibabacloud-landing-zone](https://github.com/aliyun/alibabacloud-landing-zone) 的
`ai-sdlc/auto-wonder/`）通过**执行器**（executor）在用户机器上跑交付任务：执行器本质是一个长驻进程——

```bash
npx -y autowonder@<ver> connect --ws-url wss://<host>/ws/executor --token <executor-token> \
  --executor-id <id> --provider qoder --model <model> --reasoning-effort medium \
  --context-window 1000000 --token-aware-enable
```

——通过 WebSocket 连平台、心跳保活、领取 TaskPackage 执行。

**现状痛点**（新项目接入流程第 6 步的产物）：

1. 接入时由 Agent 通过 MCP `create_executor` + `build_executor_launch_command` 生成启动命令，
   然后封装成一个 bash 脚本（`executor-manager.sh` 模板：硬编码 `名称|启动命令` 数组，
   仅支持 start/stop/status/log，nohup + pidfile）丢到用户桌面。
2. 每台机器、每次接入生成一份脚本，**分散、手工编辑、无统一视图**；改模型/换内网地址要手改数组，
   公网→内网 `wss→ws` 也要手工替换。
3. bash 脚本 **Windows 原生跑不了**；无进程守护（崩溃无人拉起）、日志无上限增长。
4. 本地进程状态与平台 ONLINE 状态没有对照，排障要两头看。

## 2. 目标与非目标

### 目标

做一个**跨平台（Windows/Linux/macOS）的本机执行器管理应用 awem**（AutoWonder Executor Manager）：

- **AI 可用**：CLI 子命令支持添加、修改、启动、停止、重启、删除、查看执行器，全程 `--json`、幂等、退出码规范。
- **人可用**：内嵌 Web 页面可视化管理局部执行器状态、查看/跟随日志、增删改查执行器。
- **统一管理**：结构化配置替代散落脚本，崩溃自动守护，本地状态与平台在线状态并列展示。

### 非目标（红线）

- ❌ 不做任务派发（dispatch 是平台调度器的事，本地越界会打架）。
- ❌ 不做多机聚合面板（看全局执行器去平台"执行器"页）。
- ❌ 不做平台**写**操作（v1 对平台只读；launchConfig 回写留 v2）。
- ❌ v1 不做内置 MCP server（v1.5 再加）。
- ❌ 不做多用户/远程访问（API 仅绑 127.0.0.1）。

## 3. 核心决策总表

| # | 决策点 | 结论 |
|---|---|---|
| 1 | 边界 | 单机自治，管本机执行器进程 |
| 2 | 平台联动 | 个人 MCP 令牌 + 平台地址，自动发现归属工作空间，60s 轮询（可配）**只读**展示 ONLINE/BUSY/OFFLINE |
| 3 | 执行语义 | start/stop/restart + 崩溃自动拉起（指数退避、可关）；不做一次性任务 |
| 4 | 架构 | 单一常驻 daemon + 内嵌 Web 页 + 同二进制 CLI 子命令（Docker 模式） |
| 5 | 进程模型 | 直接 spawn `npx -y autowonder@<ver> connect …` 自行守护；不依赖运行时自带 daemon；`install` 仅作环境体检 |
| 6 | 技术栈 | Go 单二进制，`go:embed` 打包 React SPA |
| 7 | AI 使用面 | v1 纯 CLI（`--json`、幂等、退出码规范）；v1.5 加内置 MCP server（stdio/http 双协议） |
| 8 | 录入 | 整条粘贴启动命令为一等公民（解析成结构化字段）+ 表单编辑视图 + `import` 解析旧脚本 |
| 9 | 漂移 | v1 不回写平台 launchConfig，检测到不一致标黄提示；回写留 v2 |
| 10 | 安全 | API 仅绑 127.0.0.1 + 随机 token（CLI 自动读，Web 首次输入一次）+ 配置 0600 明文落盘 |
| 11 | 日志 | 每执行器独立文件、按大小滚动、保留策略；`awem log [-f]` + Web WS 实时流；另记管理事件日志；`--debug` 默认关 |
| 12 | 自启 | 两层：`awem service install` 注册系统服务 + 每执行器 `autostart` 开关 |
| 13 | 平台支持 | Win/Linux/macOS 原生二进制；Windows 做进程树终止 + UTF-8 适配；文档推荐 7×24 用 Linux/macOS/WSL |
| 14 | 分发 | GitHub Releases + 一键安装脚本（POSIX sh + PowerShell），附 sha256 checksums |
| 15 | 命名/存储 | `awem`；OS 惯例目录 `awem/`（`config.toml` + `data/`）；`import` 从旧 `~/.autowonder-executor/` 迁移，不原地混用 |
| 16 | 本期交付 | 应用本体 + Agent 用 CLI 说明/技能 + 改造本地 `autowonder-project-onboarding` 模板（bash 脚本 → `awem add`） |

## 4. 总体架构

```
┌────────────────────────── 同一个 Go 二进制 awem ──────────────────────────┐
│                                                                          │
│  awem <子命令>（CLI） ──┐                                                │
│  awem service …（服务）  ├──► 127.0.0.1:<port> 本地 HTTP API（随机 token）│
│  Web 浏览器 ────────────┘         │                                      │
│                                   ▼                                      │
│                          ┌── daemon（常驻） ──────────────────────┐      │
│                          │  · REST + WebSocket（go:embed SPA）    │      │
│                          │  · Supervisor：每执行器一个子进程       │      │
│                          │    npx -y autowonder@<ver> connect …   │      │
│                          │  · 日志收割 + 滚动 + 事件日志           │      │
│                          │  · 平台状态轮询（MCP，只读）            │      │
│                          │  · config.toml 热加载                  │      │
│                          └────────────────────────────────────────┘      │
└──────────────────────────────────────────────────────────────────────────┘
```

- CLI 与 Web 页打同一套本地 API；CLI 在 daemon 未运行时**自动拉起** daemon（惰性启动，D2）。
- 单实例保证：文件锁/socket 锁，第二个 daemon 拒绝启动。

## 5. 数据模型与配置

### 5.1 目录布局（OS 惯例，`os.UserConfigDir()`）

| OS | 根目录 |
|---|---|
| Linux | `~/.config/awem/` |
| macOS | `~/Library/Application Support/awem/` |
| Windows | `%AppData%\awem\` |

```
awem/
├── config.toml        # 人可读人可改；文件监视热加载
└── data/
    ├── api-token      # 随机 token，0600，明文
    ├── daemon.json    # pid / port
    └── logs/
        ├── events.log                      # 管理事件日志（启停/崩溃/重启）
        └── executors/<name>.log            # 每执行器 stdout/stderr，滚动
```

### 5.2 config.toml 示例（字段为设计基准，措辞可微调）

```toml
[server]
bind = "127.0.0.1"
port  = 0                 # 0 = 自动挑选空闲端口并写回

[platform]                # 平台状态联动（可选；不配则只显示本地状态）
base_url       = "http://10.144.144.1:7001"
mcp_token      = "<个人 MCP 令牌>"
poll_interval  = "60s"
# 执行器归属自动发现后缓存：executor_id -> workspace_id

[log]
max_size_mb = 50
retention_days = 7

[[executor]]
name      = "NO1需求澄清"          # 全局唯一，[a-zA-Z0-9._-] …中文名允许？
command   = "npx -y autowonder@0.3.22 connect --ws-url ws://… --token … --executor-id 10086 --provider qoder --model qfmodel --reasoning-effort medium --context-window 1000000 --token-aware-enable"
autostart = true
[executor.restart]
enabled    = true
backoff    = "exponential"        # 1s 起步，×2，上限 5m；稳定运行 10m 后重置
```

> 名称规则见 D8：全局唯一，字符集 `[a-zA-Z0-9._-]`（用于文件名与 CLI 参数安全）。

### 5.3 命令解析与结构化字段

- `awem add` 的主入口是**整条粘贴启动命令**；解析出：runtime 版本（`autowonder@<ver>`）、
  `ws-url`、`token`、`executor-id`、`provider`、`model`、`reasoning-effort`、`context-window`、
  `memory-mode`、`max-tasks`、`--token-aware-enable`、`--debug`，以及**未知 flag 原样保留**（passthrough 段）。
- 校验：`executor-id` 为数字、`token` 非空、`provider ∈ {qoder, qodercn, claude, codex}`。
- 编辑视图改结构化字段 → 保存时**重新生成整条命令**（未知 flag 不丢）。
- 多平台实例（不同 `ws-url`）天然共存，每条配置自带完整命令。

## 6. 执行器生命周期（Supervisor）

- **spawn**：按配置直接启动 `npx -y autowonder@<ver> connect …`。
  - npx 解析按 OS：Linux/macOS 用 `npx`（经 login shell 环境或绝对路径解析），Windows 用 `npx.cmd`。
  - 环境变量透传 + 注入 `AUTOWONDER_WORKSPACE_ROOT`（默认 `~/autowonder_workspaces_<provider>_<executor-id>`，可按执行器覆盖）。
- **守护**：子进程退出且非人为停止 → 指数退避自动拉起（1s→2s→…→上限 5m；稳定运行 10m 后计数重置）；`restart.enabled=false` 则不拉起。
- **停止**：整进程树优雅终止（SIGTERM 进程组 / Windows `taskkill /T`），宽限 10s 后强杀。
- **自启两层**：`awem service install` 让 daemon 随系统启动；daemon 启动时拉起所有 `autostart=true` 的执行器。
- **状态判定**（本地）：进程存活 + 从 connect 日志解析连接状态（已连接/断线重连中），与平台轮询状态**并列展示**。

## 7. 平台状态联动（只读）

1. 用户配置一次个人 MCP 令牌 + 平台地址（`[platform]` 段，0600 明文落盘）。
2. **自动发现**：`list_projects`（READ 及以上空间）→ 逐空间 `list_executors(workspaceId)` → 按本地解析出的
   `executor-id` 匹配归属，缓存 `executor_id -> workspace_id` + `workspace_id -> 项目名` 映射
   （执行器删除/迁移时重新发现）。
3. **轮询**：默认 60s（可配）拉取各执行器实时状态 ONLINE/BUSY/OFFLINE + 心跳时间。
4. **归属展示**（2026-10-25 起）：`list_executors` 每条带执行器名（接入时的中文名）、
   `agentName`（所属数字工人中文名）、`squadNames`（所属小队）；结合项目名映射，总览表新增
   **所属项目**与**所属小队**两列、名称列中文名主显（标识名小字副显），详情页标题旁与状态条同步展示。
   未联动时两列整体隐藏、名称只显标识名；已联动但单条缺数据时该格显示 `—`。
5. **漂移检测**：对比本地解析字段（model/reasoning-effort/context-window/memory-mode/max-tasks）与平台
   `get_executor_launch_config`；不一致 → UI/CLI 标黄提示"需 restart 或平台配置已变"。权限不足则跳过该项，不报错。
6. 任何情况下**不发起平台写操作**。

## 8. CLI 设计

### 8.1 命令集（D1）

```
awem add          # 粘贴整条启动命令（或 --file）添加执行器；--autostart 等开关
awem list         # 列出全部执行器（本地+平台状态）
awem status [名称] # 单个/全部：运行态、pid、uptime、重启次数、连接状态、平台状态、漂移
awem start|stop|restart [名称]   # 无名称=全部
awem edit <名称>   # 结构化字段编辑（--model 等直接改；--command 整条替换）
awem remove <名称> # 删除配置（--purge 同时删本地日志）
awem log <名称> [-f] [--lines N]
awem import       # 解析旧 executor-manager.sh 的 EXECUTORS 数组；--with-logs 归档旧日志
awem doctor       # Node>=20 / npx / qoder|qodercn 存在 / 平台连通（配了令牌时）/ 磁盘
awem service install|uninstall|status   # systemd user / launchd / Windows 服务
awem reload       # 手动触发配置重载（正常由文件监视自动完成）
awem version
```

- 全局 flag：`--json`（稳定 schema）、`--yes`（跳过确认）。
- 幂等：重复 add 同名报"已存在"（退出码 4）而非覆盖；start 已运行 → no-op 成功。

### 8.2 退出码（D1）

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 一般错误 |
| 2 | 用法错误 |
| 3 | 未找到 |
| 4 | 已存在 |
| 5 | daemon 未运行（且无法自动拉起） |

## 9. Web UI

- React SPA（`go:embed` 打包进二进制），v1 中文，i18n 框架预留、文案走语言包（D5）。
- 页面：
  - **总览**：执行器卡片/表格——名称、本地进程态、连接状态、平台 ONLINE/BUSY/OFFLINE、模型、autostart、漂移黄标、重启次数；行内 start/stop/restart；首列多选后支持批量 start/stop/delete（串行调用单条 API，删除保留日志）。
  - **执行器详情**：WebSocket 实时日志尾部流 + 历史日志分页 + 结构化编辑表单。
  - **添加/导入**：粘贴命令 → 解析预览（高亮 token/executor-id/model）→ 保存；导入旧脚本。
  - **设置**：平台地址/MCP 令牌、轮询间隔、日志保留。
- 端口：默认自动挑空闲端口写回 config（可显式指定）；仅 127.0.0.1；API 校验 Host/Origin 防 CSRF（D4）。
- 鉴权：首次访问输入一次 token（存浏览器 localStorage），之后请求带 token 头。

## 10. 安全（Q7/D4）

- 本地 API 只绑 `127.0.0.1`；不监听任何外部接口。
- `data/api-token` 首次启动生成（crypto/rand），权限 0600（Windows 设 ACL 仅当前用户）。
- token、MCP 令牌、执行器 token 均**明文落盘**（0600）；OS keychain 留 v2。
- 威胁模型：同机同用户进程可信；同机其他用户不在 v1 范围。

## 11. 日志方案（Q13）

1. **执行器日志**：daemon 收割子进程 stdout/stderr → `data/logs/executors/<name>.log`，
   按大小滚动（默认 50MB）+ 保留策略（默认 7 天）。
2. **管理事件日志**：`data/logs/events.log` 记录启停/崩溃/重启/配置变更/服务安装等，排障时与执行器日志配合看。
3. **--debug 选项**：添加执行器时可选附加；运行时自带 rotated 诊断日志（上游文档警告长跑占磁盘），默认关闭。
4. CLI `awem log <名称> [-f]`；Web 页 WebSocket 实时尾部流。

## 12. 系统服务（service install）

| OS | 机制 | 说明 |
|---|---|---|
| Linux | systemd **user** unit（`~/.config/systemd/user/awem.service`）+ `loginctl enable-linger` 提示 | `systemctl --user enable --now` |
| macOS | launchd plist（`~/Library/LaunchAgents/com.autowonder.awem.plist`） | `launchctl load` |
| Windows | Windows 服务（`sc create`，以当前用户账户运行）或登录自启（Startup）备选 | 注意 session 0 限制：服务方式需确保 Node/qoder 环境变量为机器级；文档写明两种方式的取舍 |

## 13. 跨平台注意事项

- **Windows**：进程树终止用 Job Object 或 `taskkill /T /F`；UTF-8（控制台代码页/`chcp 65001` 或 Go 侧强制 UTF-8 管道）；`npx.cmd` 解析；路径处理。
- **Linux/macOS**：进程组（`setpgid`）+ 组信号；`kill -- -pgid`。
- 文档明确：7×24 长跑场景推荐 Linux/macOS 或 WSL（与平台最佳实践一致）。

## 14. 分发与版本（D10 / Q8）

- GitHub Releases：`awem-<os>-<arch>[.exe]`（linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64）+ `checksums.txt`。
- 一键安装：`install.sh`（POSIX）与 `install.ps1`（PowerShell）。
- 版本：semver，自 **v0.1.0** 起。

## 15. 交付边界与里程碑

### 本期（v1）

1. **awem 应用本体**：daemon + CLI + Web（本文全部范围）。
2. **Agent 使用说明**：仓库内文档（`docs/`），供 AI Agent 通过 CLI 维护执行器；后续可独立成 skill。
3. **改造本地 `autowonder-project-onboarding` 技能**（独立仓库 `~/.agents/skills/autowonder-project-onboarding`）：
   模板从生成 `executor-manager.sh` 改为生成 `awem add --launch-command …` 指引，不放入本仓库。

### v1.5

- 内置 MCP server（stdio/http 双协议，可选启动模式）。
- `awem runtime list/set <ver>` 批量升降级 runtime 版本。

### v2 待办池

- launchConfig 回写平台（需管理员凭据，双向同步）。
- OS keychain 存密。
- 上游 ai-sdlc 文档/`initialize-autowonder-harness` PR（三树校验流程重，等应用稳定后推）。

## 16. 工程约定（D10）

- Go 单 module，仓库名保持 `autowonder-executor-manager`，二进制/包名 `awem`。
- 预期目录：`cmd/awem/`（入口）、`internal/`（supervisor、config、platform、api、logs…）、`web/`（React SPA 源码，构建产物 embed）。
- 测试：单测覆盖命令解析/import/守护状态机；冒烟测试用假长驻子进程（启停/崩溃拉起/日志收割）。
- 文档：代码注释与文档中文，标识符英文。

## 附录 A：上游运行时事实速查（实施参考）

- npm 包 `autowonder` 自带子命令：`connect`（生产连平台）、`start/status/stop`（本地手工派发模式的 daemon）、
  `install [--force]`、`dispatch ./assignment.json`、`workitem upload …`、`--debug`。
  awem 只复用 `connect`（spawn 对象）与 `install`（环境体检），**不依赖**其 daemon。
- provider：`qoder`（默认）/ `qodercn`（须显式）/ `claude` / `codex`；qoder/qodercn 需 Node ≥ 20。
- 协议要点：WebSocket `wss://{server}/ws/executor?executorId=…&token=…&version=…`；PING/PONG 15s；
  信号优先级 CANCEL > PAUSE > RESUME；平台侧有心跳超时判定（约 2 分钟），本地进程活着 ≠ 平台 ONLINE。
- 平台侧执行器管理：`GET /api/agents/{agentId}/executors/{id}` 实时状态；token 仅 ADMIN 可见；
  `executor-restart-controls.md`（平台可下发远程重启）、`executor-model-upgrade-policy.md`（按任务数自动升模型）。
- MCP 通道（awem 状态联动用）：`autowonder.list_projects` / `autowonder.list_executors(workspaceId)` /
  `autowonder.get_executor_launch_config`，Streamable HTTP，`POST <mcp-url>` JSON-RPC，工具名为**点号**前缀。
  端点推导与响应形态（2026-10-25 真实平台实测）：端点 `<base_url>/api/mcp/<mcp_token>/`（令牌在 URL 路径，
  Bearer 头平台兼容可保留）；list_projects/list_executors 外层 `{"items":[…]}` 包裹；list_projects 条目键为
  `id`/`name`；list_executors 条目含 `id`/`name`（执行器中文名）/`agentId`/`agentName`/`status`/
  `lastHeartbeat`（Unix 毫秒）；launch_config 的 `contextWindow` 为字符串（如 `"1000000"`）。

## 附录 B：现状资料索引

- 旧脚本模板：`~/.agents/skills/autowonder-project-onboarding/templates/executor-manager.sh`
- 接入流程文档：平台帮助中心 `<平台地址>/help?chapter=project-onboarding`（第 6 步"注册并启动执行器"）
- 上游开源镜像：https://github.com/aliyun/alibabacloud-landing-zone/tree/master/ai-sdlc/auto-wonder
  （`docs/runtime-usage.md`、`docs/scheduler-executor-protocol.md`、`docs/executor-*.md`）
