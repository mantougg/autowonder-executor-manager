# T08 · 平台状态联动（只读）

- **子任务状态**：✅ 已检查
- **前置依赖**：T02（[platform] 配置）、T03（本地解析字段）、T06（注入 API/状态数据）
- **设计依据**：[design.md](../design/design.md) §7（平台状态联动）、决策 #2/#9、附录 A（MCP 通道事实）
- **产物范围**：`internal/platform/`（MCP 只读客户端、归属发现、状态轮询、漂移检测）

## 规范速查（开工前逐条确认；完整规范见 [conventions.md](conventions.md)）

1. 开工前通读 conventions.md 与 design.md §7、附录 A。
2. **红线中的红线**：对平台**只读**。MCP 客户端代码层硬白名单：仅 `autowonder.list_projects` / `autowonder.list_executors` / `autowonder.get_executor_launch_config` 三个工具，其余调用一律在客户端层拒绝（F5 强制）。任何情况下不发起平台写操作。
3. 未配置 `[platform]` 段 → 联动整体禁用、静默降级（本地功能完全不受影响），**不许报错打扰**。
4. 权限不足（如令牌看不到某空间/某执行器）→ 跳过该项不报错（design §7.4）。
5. 注释文档中文、标识符英文；功能点完成前 `go build/vet/test` 三绿；测试不依赖真实平台（假 HTTP 服务）。
6. 状态即改即记：本文件与 [index.md](index.md) 同步 + 双方变更记录各加一行。

## 功能点

### F1 · MCP 只读客户端

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：Streamable HTTP JSON-RPC 客户端：`POST <mcp-url>`（mcp-url 由 base_url + 令牌推导，规则对齐平台帮助/附录 A，实施时以真实环境验证并记录）；工具名为**点号前缀**（`autowonder.list_projects`）；支持 initialize 握手与会话头（按 MCP 规范 Streamable HTTP，实现细节记录）；超时、重试与错误分类（网络失败/权限不足/工具不存在）。mcp_token 与 base_url 来自 T02 配置，热加载生效。
- **验收要点**：
  - 假 MCP 服务（httptest）单测：正常响应、超时、错误响应三路径；
  - 请求体/工具名与附录 A 一致（点号前缀断言）；
  - 白名单外工具在客户端 API 上不可达（编译期约束或运行时硬拒）。

### F2 · 执行器归属自动发现

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：`list_projects`（READ 及以上空间）→ 逐空间 `list_executors(workspaceId)` → 按本地解析出的 `executor-id` 匹配归属，建立并缓存 `executor_id -> workspace_id` 映射（缓存落盘 `data/platform-cache.json` 还是纯内存：选定并记录，建议落盘减少冷启动轮询面）；触发时机：平台配置首次生效、执行器集合变化、缓存未命中；执行器在平台删除/迁移（轮询查不到或归属变化）→ 重新发现。发现不了（本地 id 平台不存在）→ 该执行器平台状态显示"未归属"，不算错误。
- **验收要点**：
  - 假服务单测：多空间多执行器 → 映射正确；空间无权限 → 跳过；平台侧删除后重发现；
  - 缓存命中避免重复全量扫描（断言调用次数）。

### F3 · 状态轮询

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：按 `poll_interval`（默认 60s，可配）轮询各已归属执行器实时状态 ONLINE/BUSY/OFFLINE + 心跳时间；数据注入 `/api/executors` 响应（T06 预留位）；轮询失败（网络/平台挂）→ 平台状态列显示"不可达"并记事件，不阻塞本地功能、不雪崩重试（退避策略记录）；daemon 空闲期轮询开销可控。
- **验收要点**：
  - 假服务单测：状态变化反映到快照；服务中断 → "不可达"态 + 恢复后自动恢复轮询；
  - 未配置 [platform] 时轮询完全不启动。

### F4 · 配置漂移检测

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：对比本地解析字段（model / reasoning-effort / context-window / memory-mode / max-tasks）与平台 `get_executor_launch_config`：不一致 → 产出 drift 结构（哪些字段、本地值 vs 平台值），供 CLI status 与 UI 黄标展示，文案按 design #9："需 restart 或平台配置已变"（v1 不回写）；权限不足 → 跳过该项不报错；漂移检测频率与轮询同周期或更低（选定记录）。
- **验收要点**：
  - 假服务单测：构造每个字段的不一致 → drift 字段与双侧值正确；全部一致 → 无 drift；
  - `get_executor_launch_config` 失败/权限不足 → 该项跳过且无错误传播。

### F5 · 只读白名单强制

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：把 F1 的白名单做成显式机制：客户端仅暴露三个只读方法的类型化封装（`ListProjects` / `ListExecutors` / `GetExecutorLaunchConfig`），底层 call 函数私有且带工具名白名单校验（双保险）；代码里不存在任何写工具字符串（加注释说明红线来源 design §2/§7.5）。
- **验收要点**：
  - 单测：绕道传入写工具名（如 `autowonder.create_workitem`）→ 客户端直接拒绝、无 HTTP 发出；
  - 代码检索检查：`internal/platform/` 内无平台写动词的工具调用（检查方式记录）。

### F6 · 接入 API/CLI 与 doctor 补全

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：把平台状态与 drift 接进 `GET /api/executors`、`awem list/status`（替换 T06/T07 的 `—` 占位）；`awem doctor` 的平台连通检查补全（调一次 list_projects，成功/失败/未配置三态）；daemon 顶栏"平台连接状态"数据源就位（T09 消费）。
- **验收要点**：
  - 假平台端到端：配置 [platform] 后 list/status 显示 ONLINE/BUSY/OFFLINE 与漂移黄标数据；
  - doctor 三态输出正确；
  - 撤掉 [platform] 配置 → 全部功能回到本地态、无报错。

## 完成与检查判定

- 全部功能点 🟠 且三绿 → 子任务状态改 🟠 已完成。
- 检查环节逐功能点跑"验收要点"（重点：白名单拒绝写操作、未配置降级、权限不足静默跳过），全 ✅ → 子任务 ✅；任一 ❌ → 打回 🔄 并登记"检查记录"。

## 检查记录

| 日期 | 功能点 | 结果 | 问题与处理 |
|---|---|---|---|
| 2026-10-25 | F6 | ✅ 通过 + 子任务 ✅ | 实跑 `go test ./internal/cli/ -run Test端到端 -count=1`（假平台端到端）：平台状态与漂移展示（list 含 ONLINE 与 ⚠1；status 详情含 平台状态：ONLINE、漂移 1 项、--model 本地 "qfmodel" vs 平台 "dfmodel"、需 restart 文案；--json 含 platform/drift 结构；平台改 BUSY + 手动刷新后展示跟随）；撤下联动回本地态（无 ONLINE、doctor 回"— 未配置"跳过态且全部通过）；doctor 三态（可达 ✅ 退出 0；断服 ❌ 退出 1）。既有全套回归通过（doctor 本机/矩阵/status 文案更新）。全仓三绿 + 交叉编译。**T08 全部 6 功能点 ✅，子任务 ✅。** |
| 2026-10-25 | F4 | ✅ 通过 | 实跑 `go test ./internal/platform/ -run TestDrift -count=1`：字段不一致与一致（model qfmodel≠dfmodel 与 context-window 1000000≠400000 恰两处漂移且双侧值正确；reasoning-effort 一致不计；改平台配置后全部一致 → 无漂移）；查询失败静默跳过（权限不足 → Refresh 不报错、状态 ONLINE、无 drift、查询已尝试）；未归属不查询（99999 unowned 且无 launch_config 调用）。开发中修复：checkDrift 初版在状态表更新前执行导致首轮漂移写入丢失（顺序调整为状态更新后合并）。全仓三绿 + 交叉编译。 |
| 2026-10-25 | F3 | ✅ 通过 | 实跑 `go test ./internal/platform/ -run Test轮询 -count=1`：状态变化反映到 Lookup（ONLINE→改 BUSY 后刷新即 BUSY 且心跳解析；未归属 id → unowned）；不可达与恢复（清空假服务工具 → Refresh 报错且 Lookup=unreachable、FailCount=1；恢复种子后回 ONLINE、计数清零）；Start 循环（50ms 周期 2s 内 ≥3 次 list_executors；ctx 取消即退出）；未配置平台 Start 立即返回不启动。全仓三绿 + 交叉编译。 |
| 2026-10-25 | F2 | ✅ 通过 | 实跑 `go test ./internal/platform/ -run "Test发现|Test缓存|Test平台侧|Test未配置" -count=1`：多空间映射正确（10086→10000、20001→10001）且缓存落盘可读回；权限不足空间跳过（platform_skip 事件 + 可达空间归属仍建立）；缓存命中避免重复扫描（二次 ensureDiscovery list_projects 仍 1 次；新增本地 id 触发第 2 次全量）；平台侧删除后全量发现归属移除；未配置平台 ensureDiscovery no-op 且 Lookup 未命中。全仓三绿 + 交叉编译。 |
| 2026-10-25 | F5 | ✅ 通过 | 实跑 `go test ./internal/platform/ -run "Test白名单|Test平台包" -count=1`：白名单外工具（create_workitem/assign_workitem/delete_executor/transition_workitem/方法名伪装/任意串）全部在客户端层拒绝且 httpCalled=false（无 HTTP 发出）；白名单恰为只读三件套；*Client 导出方法恰为 ListProjects/ListExecutors/GetExecutorLaunchConfig；包内源码（platform.go/mcp.go）不含写工具字符串。全仓三绿 + 交叉编译。 |
| 2026-10-25 | F1 | ✅ 通过 | 实跑 `go test ./internal/platform/ -count=1`（4 用例）：TestMCP正常链路与会话头（多空间/执行器/心跳解析；工具名点号前缀经假服务调用记录断言；initialize 恰好一次）、TestMCP错误响应与权限分类（JSON-RPC permission 错误与 HTTP 401 均 IsPermissionErr；未知工具为一般错误）、TestMCP超时（ctx 超时返回错误）、TestMCP_SSE响应形态（text/event-stream data: 行可解析）。全仓三绿。 |

## 变更记录

| 日期 | 动作 | 说明 |
|---|---|---|---|
| 2026-10-24 | 创建 | 由任务计划生成 |
