# T07 · CLI 子命令集

- **子任务状态**：🔄 进行中
- **前置依赖**：T03、T05、T06（API 客户端 + 惰性拉起）
- **设计依据**：[design.md](../design/design.md) §8（CLI 设计）、§5.3（add/edit 语义）、D1（命令集/退出码）
- **产物范围**：`internal/cli/` 各子命令（add/list/status/start/stop/restart/edit/remove/log/import/doctor/reload/version）

## 规范速查（开工前逐条确认；完整规范见 [conventions.md](conventions.md)）

1. 开工前通读 conventions.md 与 design.md §8。
2. 红线：不做派发；平台写操作不存在；CLI 全部走本地 API（与 Web 同一套），**不绕过 API 直改状态**。
3. CLI 约定（conventions.md §4）：`--json` 稳定 schema、`--yes` 跳过确认、幂等、退出码 0/1/2/3/4/5、破坏性操作必须确认。
4. 人机输出：默认中文人读格式（表格/摘要），`--json` 机器格式；两者信息一致。
5. 注释文档中文、标识符英文；功能点完成前 `go build/vet/test` 三绿。
6. 状态即改即记：本文件与 [index.md](index.md) 同步 + 双方变更记录各加一行。
7. 本任务专属：每个子命令的 `--json` 输出结构在本文件变更记录汇总成 schema 表（Agent 使用说明 T11 直接引用）。

## 功能点

### F1 · add

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：`awem add "<整条启动命令>"`（或 `--file <路径>` 读命令）；开关：`--autostart`/`--no-autostart`、`--restart`/`--no-restart`、`--debug`、`--start`（添加后立即启动，默认行为定为"添加后不自动启动除非 autostart 显式给"并记录）；走 API add 链路（解析→校验→落盘）；成功输出解析摘要（名称、executor-id、model、provider——**token 掩码**）。同名已存在 → 退出码 4；解析失败 → 退出码 2 + 逐字段错误。
- **验收要点**：
  - 集成测试（httptest 假 daemon 或真拉起）：正常添加、重名（4）、坏命令（2）、`--json` 输出字段稳定且 token 掩码；
  - `--file` 与参数两种输入等价。

### F2 · list

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：`awem list`：表格输出名称、本地进程态（含连接状态）、平台状态（T08 前显示 `—`）、model、autostart、漂移标记（T08 前 `—`）；`--json` 输出同构数据。空列表给一句中文引导（如何 add）。
- **验收要点**：
  - 有/无执行器两种输出形态正确；
  - `--json` schema 与 API `GET /api/executors` 一致（复用结构体）。

### F3 · status

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：`awem status [名称]`：单个=详细（运行态、pid、uptime、重启次数、连接状态、平台状态、漂移详情）；无名称=全部摘要；未找到 → 退出码 3。漂移详情字段 T08 前占位。
- **验收要点**：
  - 单个/全部/不存在三态输出与退出码正确；
  - `--json` 字段覆盖 design §8.1 列举的全部信息项。

### F4 · start / stop / restart

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：`awem start|stop|restart [名称]`，无名称=全部；stop 全部是破坏性操作需确认（`--yes` 跳过，确认文案写具体动作如"停止全部 3 个执行器"）；幂等：start 已运行 → no-op 成功、stop 未运行 → no-op 成功；操作结果逐条汇报（成功/失败/跳过）。
- **验收要点**：
  - 幂等三态（运行中 start、已停止 stop、不存在 → 3）测试；
  - stop 全部无 `--yes` 时交互确认生效（测试可注入 stdin）；
  - 部分失败时整体退出码 1 且逐条结果可见。

### F5 · edit

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：`awem edit <名称> --model <m> --reasoning-effort <r> --context-window <n> --memory-mode <m> --max-tasks <n> --autostart[=bool] --debug[=bool]`（结构化字段直改）；`--command "<新命令>"` 整条替换（走解析校验）；保存后**重新生成整条命令**（T03 F3，未知 flag 保留）；运行中的执行器提示"需 restart 生效"。
- **验收要点**：
  - 改单字段 → 配置落盘且命令串对应段变化、passthrough 不丢（单测断言 roundtrip）；
  - `--command` 坏命令 → 退出码 2，原配置不变；
  - 未找到 → 3。

### F6 · remove

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：`awem remove <名称>`：删除配置；运行中先停（确认提示说明）；`--purge` 同时删本地日志文件；确认交互（`--yes` 跳过）；未找到 → 3。
- **验收要点**：
  - remove 后 list 不再出现、日志文件随 `--purge` 删除（不带 purge 保留）；
  - 运行中 remove 走"先停后删"，进程无残留（冒烟）。

### F7 · log

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：`awem log <名称> [-f] [--lines N]`：默认尾部 N 行（默认值定 100 并记录）；`-f` 跟随（复用 Follow，Ctrl-C 退出干净）；无日志文件给空态提示；未找到 → 3。输出为原始日志行（ANSI 保留）。
- **验收要点**：
  - tail 行数正确；`-f` 能收到新行并 Ctrl-C 无残留（冒烟）；
  - `--json` 时输出结构化行数组（非流式场景）。

### F8 · import

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：`awem import [--file <旧脚本>] [--with-logs]`：默认路径 `~/executor-manager.sh`？——**实施时与用户确认常见路径**（先支持 `--file`，缺省找当前目录与家目录的 `executor-manager.sh`，规则记录）；解析（T03 F4）→ 逐条展示导入预览（名称映射/改名建议）→ 确认 → 批量 add；`--with-logs` 归档旧日志（T03 F5）；`--json` 输出汇总（导入成功/失败/改名明细）。名称冲突（含导入条目之间）处理策略：后缀去重建议 + 确认。
- **验收要点**：
  - 用真实模板样例做集成测试：含中文名（走改名确认）、坏行（跳过并报告）；
  - `--with-logs` 后日志出现在新位置；
  - 汇总统计与实际落盘一致。

### F9 · doctor

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：环境体检（design §8.1）：Node ≥ 20（qoder/qodercn 要求）、`npx` 可用、`qoder`/`qodercn` CLI 存在（按将用的 provider 检查）、磁盘剩余空间（数据目录）、平台连通（**T08 完成前此项输出"未实现/跳过"占位，T08 F6 补全**）；逐项 ✅/❌ + 修复建议（中文）；任一 ❌ 退出码 1。
- **验收要点**：
  - 本机运行输出全部检查项；构造缺失（PATH 里去掉 npx）→ ❌ 且退出码 1；
  - 平台连通项有占位且不误报。

### F10 · reload / version 与全局行为收口

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：`awem reload`（接 T06 F7）、`awem version`（含构建信息）；全局行为收口：所有子命令的 `--json` schema 汇总表（变更记录）、错误信息统一中文统一格式、退出码全表复核（对照 design §8.2 逐命令过一遍）、`awem` 无参数显示用法（退出码 2）。
- **验收要点**：
  - 退出码矩阵单测/集成测试：每命令 × 成功/用法错/未找到/已存在/daemon 挂（5）全覆盖；
  - `--json` 汇总表与实际输出一致（抽查 3 个命令比对）。

## 完成与检查判定

- 全部功能点 🟠 且三绿 → 子任务状态改 🟠 已完成。
- 检查环节逐功能点跑"验收要点"（重点：退出码矩阵、幂等、确认交互、token 掩码），全 ✅ → 子任务 ✅；任一 ❌ → 打回 🔄 并登记"检查记录"。

## 检查记录

| 日期 | 功能点 | 结果 | 问题与处理 |
|---|---|---|---|
| 2026-10-25 | F6 | ✅ 通过 | 实跑 `go test ./internal/cli/ -run TestRemove -count=1`：TestRemove确认与落盘（n 取消配置保留、y 删除、--yes 跳过、不存在 3）、TestRemovePurge与日志保留（不带 purge 日志文件保留；--purge 文件删除断言）、TestRemove运行中先停后删（确认文案含"将先停止其进程"、结果"已停止进程"、删除后无残留 pid；拒绝后进程仍 running）。全仓三绿 + 交叉编译。 |
| 2026-10-25 | F5 | ✅ 通过 | 实跑 `go test ./internal/cli/ -run TestEdit -count=1`：TestEdit单字段与命令重建（--model dfmodel --reasoning-effort low → 命令重建、--token-aware-enable 与原 token 保留）、TestEdit布尔与autostart（--debug 追加、--no-autostart 生效、--debug=false 移除 flag）、TestEdit整条替换与坏命令（替换生效；not-a-command → 退出码 2 且原配置字节不变；不存在 → 3；无修改项 → 2）、TestEdit运行中提示restart（"下次 restart 后生效"）。 |
| 2026-10-25 | F4 | ✅ 通过 | 实跑 `go test ./internal/cli/ -run "TestActions|TestStop全部|TestStart全部|Test部分失败" -count=1`：TestActions单名称三态（start→running、重复 start 幂等 0、stop→stopped、重复 stop 幂等 0、restart 未运行=启动、start/stop 不存在→3）、TestStop全部确认交互（拒绝 n→"已取消"退出 0 且文案含"将停止全部 2 个执行器"；确认 y→✓ s1/s2 已停止；--yes 跳过确认）、TestStart全部逐条汇报（✓ g1 ✓ g2）。注：多名称部分失败整体退出码 1 的路径在当前命令面（至多一个名称参数/全部名单恒有效）无法自然构造，代码路径已实现（failed>0 → General），遗留到 F10 退出码矩阵统一复核。 |
| 2026-10-25 | F3 | ✅ 通过（含一次返工） | 首次提交时 go test 输出经管道 grep/tail 吞掉退出码，cli 包实际 FAIL（未启动执行器 local.conn 为空串，--json 断言失败）仍被提交——违反"提交前三绿"。随即修复：api 视图层对空 conn 兜底为 unknown（未启动/未见过连接日志统一语义），实跑全仓 go build/vet/test 三绿后补提交。教训：验证命令不得经管道掩盖退出码。 | 实跑 `go test ./internal/cli/ -run TestStatus -count=1`：TestStatus单个详细（经 supervisor 直启后详情含 运行状态：running、pid、uptime、连接状态、ID/provider/autowonder@0.3.22、平台 —、漂移 —）、TestStatus全部摘要（共 2 个执行器）、TestStatus不存在退出码3（文本与 --json 均 3）、TestStatusJSON字段覆盖设计8_1（state/pid/started_at/uptime_ms/restarts/conn/model/platform/drift 全在）。 |
| 2026-10-25 | F2 | ✅ 通过 | 实跑 `go test ./internal/cli/ -run TestList -count=1`：TestList有执行器表格（e1/e2、stopped、unknown 连接态、qfmodel 模型、共 2 个、平台占位 —）、TestList空列表引导（"还没有执行器" + awem add 提示，退出码 0）、TestListJSON与APISchema一致（executors[0] 含 name/command/autostart/restart/parsed/local/platform/drift 全字段）。 |
| 2026-10-25 | F1 | ✅ 通过 | 实跑 `go test ./internal/cli/ -count=1`（6 用例）：TestAdd正常添加与人读摘要（exec-10086 自动命名、摘要含 ID/provider/版本、**明文 token 不出现**、config.toml 落盘）、TestAdd默认名与自定义名（--name 生效；重名退出码 4）、TestAdd坏命令退出码2（executor-id 非数字/非 npx 命令/缺命令 → 2）、TestAdd文件输入与参数等价（--file 落盘一致）、TestAddJSON输出掩码（合法 JSON、token 含 ***、--start 后 local.state=running 真子进程）、TestAdd开关落盘（--autostart/--no-restart/--debug 逐项断言）。全仓三绿 + 交叉编译。 |

## 变更记录

| 日期 | 动作 | 说明 |
|---|---|---|
| 2026-10-24 | 创建 | 由任务计划生成 |
| 2026-10-25 | 开工 | 子任务状态 ⬜→🔄；fakeproc 从两处 testdata 提为 internal/fakeproc 共享包（supervisor/api/cli 三处测试 TestMain 统一构建）；daemon.Client 导出 NewClientAt（测试直连）；APIError 退出码映射补充 invalid_command/invalid_name/bad_request/invalid_config→2（用法错）；cli 包新增 Deps 依赖注入（Layout/Connect/In/Out）与 RegisterCommands 装配骨架 |
| 2026-10-25 | F6 实现 | 确认文案按状态组装（运行中→"将先停止其进程"；--purge→"删除全部本地日志"；恒带"不可恢复"）；运行中自动带 stop=true（API 先停后删）；--purge 映射 purge=true；结果汇报已停止/日志已清除。 |
| 2026-10-25 | F5 实现 | 字段 flag 全集：model/reasoning-effort/context-window/memory-mode/max-tasks/token/ws-url/executor-id/provider/runtime-version + 布尔 autostart[=bool]/restart[=bool]/debug[=bool] + workspace-root + --command 整条替换；值 flag 支持 --x v 与 --x=v 两种形态；服务端重建命令（passthrough 与未提交 token 不丢）；运行中（running/backoff/stopping）提示"下次 restart 后生效"；输出展示新命令全文。 |
| 2026-10-25 | F4 实现 | 三命令共用 cmdAction 工厂；无名称=全部（名单来自 GET /api/executors，空名单友好提示）；stop 全部确认文案"将停止全部 N 个执行器"（--yes 跳过；n/N/空/EOF=拒绝）；逐条 ✓/✗ 汇报（含动作后状态）；单名称失败直接返回对应退出码（3/1），多名称部分失败继续并整体退出码 1；start/restart 全部不确认（非破坏性）。 |
| 2026-10-25 | F3 实现 | 单个=详细（运行态[含退避倒计时]/pid/uptime/启动时间/连接状态/重启次数/上次退出/ID/provider/模型/runtime/autostart/平台占位/漂移占位）；无名称=全部摘要（复用 list 表格）；--json 单个直透 API executor 视图、全部直透列表响应；uptime 毫秒→秒级人读。 |
| 2026-10-25 | F2 实现 | 表格列：名称/状态/连接/平台(T08 前 —)/模型/自启(✓/—)；空列表中文引导（提示 awem add 用法）；--json 直透 API GET /api/executors 响应（复用 executor 视图，schema 严格一致）；结尾行给平台联动提示。 |
| 2026-10-25 | F1 实现 + 决策记录 | **名称策略**：默认名 `exec-<executor-id>`（--name 覆盖；design 未定，选"可从命令自动推导且无中文"方案）。**默认不启动**：--autostart=随 daemon 拉起（配置语义），--start=添加后立即启动（一次性动作）。**--debug** 以追加布尔 flag 进命令实现（§11.3）。**错误分层**：本地先 Parse+Validate（逐字段、退出码 2、不走网络）；名称显式给出时容忍解析失败交 API 复核。**--json**：直透 API 201 响应（executor 视图，token 恒掩码）。**人读摘要**：名称/ID/provider/模型/runtime 版本/token 掩码/autostart + 后续提示。 |
