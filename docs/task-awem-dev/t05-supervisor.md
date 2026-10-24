# T05 · Supervisor 进程守护

- **子任务状态**：🔄 进行中
- **前置依赖**：T02（配置与热加载）、T03（命令解析）、T04（日志与事件）
- **设计依据**：[design.md](../design/design.md) §6（执行器生命周期）、§13（跨平台注意事项）、决策 #3/#5
- **产物范围**：`internal/supervisor/`（spawn、守护状态机、停止、autostart、本地状态判定）+ 假子进程冒烟测试

## 规范速查（开工前逐条确认；完整规范见 [conventions.md](conventions.md)）

1. 开工前通读 conventions.md 与 design.md §6/§13、附录 A（上游运行时事实）。
2. 文档优先：守护参数（1s 起步 ×2 上限 5m、稳定 10m 重置、宽限 10s 强杀）以 §6 为准；要改先改文档。
3. 红线：只 spawn `npx -y autowonder@<ver> connect …` 长驻进程，**不做一次性任务、不做派发**（§2）；不依赖 autowonder 自带 daemon（决策 #5）。
4. 测试用**假长驻子进程**（自编测试程序/脚本），不依赖真实平台、npx 与外网；冒烟测试是本任务验收核心。
5. 注释文档中文、标识符英文；功能点完成前 `go build/vet/test` 三绿。
6. 状态即改即记：本文件与 [index.md](index.md) 同步 + 双方变更记录各加一行。
7. 本任务专属：状态枚举、连接状态日志关键字等实现细节自定后**必须记入变更记录**（后续 CLI/Web/事件都引用它）。

## 功能点

### F1 · 子进程 spawn（跨平台 npx 解析）

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-25 自检）
- **开发描述**：按配置命令（T03 解析结果重建 argv）spawn 子进程：Linux/macOS 解析 `npx`（`exec.LookPath`，必要时经 login shell 环境或绝对路径——策略记录）；Windows 用 `npx.cmd`。环境变量：全量透传 + 注入 `AUTOWONDER_WORKSPACE_ROOT`（默认 `~/autowonder_workspaces_<provider>_<executor-id>`，可按执行器覆盖）。工作目录、附加 `--debug`（T04 F5 的开关）。stdout/stderr 接 T04 LogSink。
- **验收要点**：
  - 用假命令（如 `sleep`/自编长驻程序）单测 spawn 成功、pid 记录、环境变量注入生效（假进程回显环境）；
  - npx 不存在时错误清晰（中文、指向 doctor）；
  - Windows 分支代码路径存在（真机验证归 T10）。

### F2 · 守护状态机（崩溃自动拉起）

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-25 自检）
- **开发描述**：每执行器一个 goroutine 监管：子进程退出且**非人为停止** → 指数退避自动拉起（1s → ×2 → 上限 5m）；**稳定运行 10m 后计数重置**；`restart.enabled=false` 则退出即退出不拉起。状态机枚举（如 Starting/Running/Backoff/Stopping/Stopped/Failed）自定并记录；状态迁移全部发事件（T04 F4）。可测试性：时钟与退避参数可注入。
- **验收要点**：
  - 假子进程冒烟：启动 → 杀掉 → 自动拉起（退避间隔按注入的快时钟缩短验证）；
  - 稳定 10m（注入时钟快进）后崩溃 → 退避从 1s 重新起步；
  - `restart.enabled=false` 崩溃后不拉起且状态正确；
  - 人为 stop 不触发拉起（与 F3 联测）。

### F3 · 停止与进程树终止

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-25 自检）
- **开发描述**：stop = 优雅终止**整棵进程树**（npx → node → CLI 子进程）：POSIX 用进程组（spawn 时 `setpgid`，对 `-pgid` 发 SIGTERM）；Windows 用 `taskkill /T`（或 Job Object 方案，T10 定稿，本任务先实现 taskkill 路径并留接口）；宽限 10s 后强杀（SIGKILL / `taskkill /T /F`）。停止语义与 F2 联动：人为停止不拉起。
- **验收要点**：
  - 假进程树（父进程再起子进程）冒烟：stop 后**无残留进程**（遍历 pgid 断言）；
  - 优雅期退出则不强杀（事件里能区分）；强杀路径用"忽略 SIGTERM 的假进程"验证；
  - 已停止再 stop = no-op 成功（幂等）。

### F4 · autostart 拉起

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-25 自检）
- **开发描述**：daemon 启动与配置热加载后新增条目时，拉起所有 `autostart=true` 的执行器（新增条目即时拉起、存量未运行的不动——语义记录）；失败不阻塞其他执行器，逐条记事件。
- **验收要点**：
  - 假进程单测：autostart=true 在 Supervisor 启动时被拉起，false 不动；
  - 某条启动失败不影响其余条目。

### F5 · 本地状态判定与连接状态解析

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：对外提供每执行器本地状态快照：进程存活、pid、uptime、重启次数、当前退避阶段 + **连接状态**（从 connect 日志输出解析：已连接 / 断线重连中 / 未知；关键字基于上游日志实标——开发时跑一次真实 connect 采样关键字并记入变更记录，附录 A 协议要点可旁证：PING/PONG 15s、平台心跳超时约 2 分钟，本地进程活着 ≠ 平台 ONLINE）。解析挂在 LogSink 的写入路径上（行级正则/前缀匹配，开销可控）。
- **验收要点**：
  - 单测：注入模拟日志行 → 状态迁移正确（连接→断线→重连）；
  - 快照字段齐（pid/uptime/重启次数/连接状态），uptime 与重启次数随冒烟过程正确变化。

### F6 · 与日志/事件联动收口

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：全生命周期事件落 events.log：启动（含 debug 标记）、人为停止、崩溃（含退出码）、自动重启（含第几次/退避时长）、启动失败。stdout/stderr 全量进对应 LogSink。确保无"静默"路径：任何状态迁移都有对应事件或状态可见。
- **验收要点**：
  - 冒烟场景（启/停/崩溃/拉起）后 events.log 事件序列完整且顺序合理；
  - 事件详情含足够排障信息（退出码、错误摘要），不含完整令牌。

### F7 · 假长驻子进程冒烟测试套件

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：沉淀可重复的冒烟套件（`internal/supervisor/smokey` 或 `_test` 构建）：用假长驻子进程（仓库自带小工具：可配置"运行/崩溃/忽略信号/打印日志行"）串起：启动 → 日志收割 → 崩溃自动拉起 → 优雅停 → 强杀路径 → 进程树无残留 → 事件序列断言。作为 T06/T10 回归基线。
- **验收要点**：
  - `go test ./internal/supervisor/... -run Smoke` 一键可跑、全绿、无真实网络/npx 依赖；
  - 套件耗时可控（注入时钟/快参数），不留僵尸进程（测试后 pgid 清扫断言）。

## 完成与检查判定

- 全部功能点 🟠 且三绿 → 子任务状态改 🟠 已完成。
- 检查环节逐功能点跑"验收要点"（重点实际执行冒烟套件、检查无残留进程），全 ✅ → 子任务 ✅；任一 ❌ → 打回 🔄 并登记"检查记录"。

## 检查记录

| 日期 | 功能点 | 结果 | 问题与处理 |
|---|---|---|---|
| 2026-10-25 | F4 | ✅ | `go test ./internal/supervisor/ -run 'TestApplyConfig' -v -count=3` 稳定通过：autostart=true 应用配置即拉起、false 不动；注入"首次 npx 解析失败"的解析器——first=Failed 且有 executor_spawn_fail 事件，second 照常 Running（失败不阻塞）；热加载四阶段——新增 autostart 即时拉起、存量未运行不动、运行中进程配置重放不重启（pid 前后一致断言）、消失条目停止+移除且有 executor_stop 事件 |
| 2026-10-25 | F3 | ✅ | `go test ./internal/supervisor/ -run 'TestStop' -v -count=3` 稳定通过：假进程树（tree 模式父起子进程）stop 后父/子进程均退出、`kill(-pgid, 0)` 探测进程组为空（无残留断言）；优雅期退出事件="人为停止（优雅退出…）"且无强杀字样；ignore-term 假进程等就绪文件（SIGTERM 忽略已生效）后 stop → 等满注入宽限 300ms 才 SIGKILL，事件含"宽限超时强杀"，耗时断言 ≥250ms；从未启动/已停止重复 stop 均 no-op 成功，未知执行器报"未找到" |
| 2026-10-25 | F2 | ✅ | `go test ./internal/supervisor/ -run 'TestCrash|TestStable|TestManual' -v` 4 用例通过且 `-count=5` 稳定：崩溃自动拉起（假进程 exit 7，事件含退出码；第 1/2 次退避 20ms/40ms ×2 序列断言）；稳定运行 150ms（注入 StableReset，快进等价）后崩溃 → 退避从 20ms 重新起步且事件含"退避计数已重置"；restart.enabled=false 崩溃后 Failed 不拉起（150ms 观察窗，restarts=0）；人为 stop 不拉起 + 重复 stop 幂等 |
| 2026-10-25 | F1 | ✅ | `go test ./internal/supervisor/ -v` 6 用例通过：spawn 成功且 pid 记录并存活（TestStartExecutorSpawnRecordsPid）；`AUTOWONDER_WORKSPACE_ROOT` 默认值注入经假进程回显验证（TestSpawnInjectsWorkspaceRootEnv）；workspace_root 覆盖走 env+argv 双通道（TestSpawnWorkspaceRootOverride，argv 文件断言含 `--workspace-root <path>`）；`--debug` 原样透传（TestSpawnDebugFlagPassthrough）；npx 缺失错误含中文与 awem doctor 指引、状态 Failed、events 有 executor_spawn_fail（TestSpawnNpxMissingClearError）；默认解析器本机实测可用。`GOOS=windows/darwin go build ./...` 通过（npx.cmd→cmd.exe /c 包装与 taskkill /T 分支编译在位，真机验证归 T10）。三绿通过，测试后无 fakeproc 残留进程 |

## 变更记录

| 日期 | 动作 | 说明 |
|---|---|---|---|
| 2026-10-24 | 创建 | 由任务计划生成 |
| 2026-10-25 | 开工 | 子任务状态 ⬜→🔄；开工侦察从本机 npm 缓存的真实 autowonder@0.3.22 包源码与 vendored daemon 二进制采样运行时事实：① connect shim（bin/cli.js）会自行重算 AUTOWONDER_WORKSPACE_ROOT（取 --workspace-root flag 值，缺省 ~/autowonder_workspaces_<provider>_<executorId>），外部注入的同名 env 会被覆盖——故 awem 注入 env 仅为对齐设计措辞，按执行器覆盖的实际生效途径是 spawn 时追加 `--workspace-root <path>`（已确认是 connect 的合法 flag）；② 连接状态日志关键字实标（vendored daemon 二进制 strings，与 onboarding 技能 troubleshooting.md 的“ws connected 有且无 disconnect/reconnect → 连接健康”旁证一致）：connected=`ws connected`；reconnecting=`ws disconnected` / `reconnecting` / `retry after reconnect`（大小写不敏感子串匹配，实现在 F5） |
| 2026-10-25 | F1 ✅ | 实现：`launchcmd.Argv`（命令→argv，保留原始 token 形态）；config 新增可选 `workspace_root` 字段（design §6“可按执行器覆盖”，覆盖经 argv 追加 `--workspace-root` 生效）；npx 解析策略——Unix 为 LookPath→`sh -lc 'command -v npx'` 回退（覆盖 GUI/服务启动缺 profile PATH 场景），Windows 为 LookPath(npx.cmd→npx.exe→npx) 且 spawn 经 `cmd.exe /c` 包装（Go 不能直接 exec 批处理）；子进程 spawn 时 `Setpgid=true` 自成进程组（F3 整树终止的前提）；事件枚举新增 `executor_spawn_fail`（T04 集合的增量） |
| 2026-10-25 | F2 ✅ | 状态机选型：每执行器一个 supervise 循环（人为启动换代 gen，循环内自动拉起不换 goroutine）；状态枚举 Starting/Running/Backoff/Stopping/Stopped/Failed（对外契约，RunningLike=Starting/Running/Backoff/Stopping）；快照 BackoffDelay 取崩溃时算好的 pendingDelay；稳定期判定基准=本次进程 startedAt（Clock.Now 注入点，测试用毫秒级参数+真实时钟等价快进） |
| 2026-10-25 | F3 ✅ | 实现要点：spawn 即 Setpgid（组长=自身 pid），停止=SIGTERM 整组 → 宽限 → SIGKILL 整组；Windows 走 taskkill /T（无 /F → /F 强杀），批处理 npx.cmd 经 cmd.exe /c 包装 exec；测试陷阱记录——SIGTERM 落在子进程 exec 初始化窗口会按默认处置杀死（真实场景等价于刚启动即停止=优雅退出），ignore-term 测试须等就绪文件再发信号 |
| 2026-10-25 | F4 ✅ | 语义定稿：SetConfig 升级为 ApplyConfig 统一入口（daemon 启动注入首份配置=全部条目视为新增，即 autostart 拉起；热加载增量判定）；新增(autostart)即拉 / 存量未运行不动 / 运行中进程不重启（下次启动生效）/ 消失条目停止并移除受管单元；启动失败逐条记事件不阻塞 |
