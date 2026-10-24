# T06 · daemon 与本地 API

- **子任务状态**：🔄 进行中
- **前置依赖**：T02、T04、T05
- **设计依据**：[design.md](../design/design.md) §4（总体架构）、§9（Web UI 后端面）、§10（安全）、D2（惰性启动）、D4（鉴权/防 CSRF）
- **产物范围**：`internal/api/`（REST + WebSocket + 静态资源 + 鉴权）、daemon 生命周期（单实例、端口写回、CLI 惰性拉起）、`awem reload`

## 规范速查（开工前逐条确认；完整规范见 [conventions.md](conventions.md)）

1. 开工前通读 conventions.md 与 design.md §4/§9/§10。
2. 红线：**API 只绑 127.0.0.1**，不提供多用户/远程访问；CLI 与 Web 打同一套 API。
3. 安全（§10）：token 头校验、Host/Origin 校验防 CSRF、token 文件 0600；不监听任何外部接口。
4. REST 响应 schema 即 CLI `--json` 的数据源，**字段命名稳定、向后兼容**（conventions.md §4）。
5. 注释文档中文、标识符英文；功能点完成前 `go build/vet/test` 三绿。
6. 状态即改即记：本文件与 [index.md](index.md) 同步 + 双方变更记录各加一行。
7. 本任务专属：API 路由与 JSON schema 定稿后记入变更记录（T07 CLI 与 T09 Web 都依赖它）。

## 功能点

### F1 · daemon 生命周期与单实例锁

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：daemon 以常驻进程运行（`awem daemon` 内部子命令或等价入口，可被 service/systemd 托管）；启动即取**单实例锁**（文件锁或锁 socket，方案记录），第二个实例拒绝启动（明确报"已有 daemon 运行于 port N"）；写 daemon.json（pid/port，T02 F6）；正常退出清理锁与 daemon.json；装配 config + logs + supervisor + platform（platform 为 T08 预留接口，先空实现）。
- **验收要点**：
  - 单测/冒烟：起两个 daemon，第二个失败且信息含已占端口；
  - 强杀第一个后（锁随进程释放方案）能重新启动；daemon.json 随启停更新/清理。

### F2 · 端口绑定与自动写回

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：监听 `127.0.0.1:port`；`port=0` → 挑空闲端口并**写回 config.toml**（保持文件其余内容与注释不破坏——写回用 T02 的结构化写，注释头保留策略记录）；写回引发的热加载自触发需抑制（跳过"内容未变"的重载，方案记录）。
- **验收要点**：
  - 单测：port=0 启动 → 实际端口写回、二次启动用该端口；
  - config 其余字段与格式在写回后不变（roundtrip 断言）；
  - 绑定地址硬编码 127.0.0.1（配置校验已在 T02 F3 保证）。

### F3 · REST API（执行器与设置全集）

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：实现资源化路由（标准库 `net/http` 或轻量路由，选型记录）：`GET /api/executors`（含本地状态/平台状态占位/漂移占位）、`POST /api/executors`（add：body 为整条命令 + 开关，服务端调 T03 解析与 T02 校验，错误逐字段返回）、`GET/PUT/DELETE /api/executors/{name}`（PUT 为结构化编辑，DELETE 支持 purge 参数；删除运行中的先停——语义与确认归 CLI/Web 层，API 层 stop 参数化）、`POST /api/executors/{name}/start|stop|restart`、`GET /api/executors/{name}/log?tail=N&follow=0`（非流式拉取）、`GET /api/status`（daemon 版本/uptime/平台连接占位）、`GET/PUT /api/settings`（platform/log 段读写）、`POST /api/reload`（手动触发热加载）。错误响应统一 JSON：`{"error":{"code":"…","message":"…","field":"…"}}`。
- **验收要点**：
  - httptest 单测覆盖每个路由的正常 + 主要错误路径（404 名称不存在、401 未带 token、409 已存在）；
  - add 走完整链路（解析→校验→落盘→可选启动），返回结构化结果；
  - schema 与变更记录一致。

### F4 · WebSocket 日志实时流

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：`GET /ws/logs/{name}`：升级为 WebSocket 后持续推送新日志行（复用 T04 F6 Follow）；先推尾部 N 行（query 参数）；客户端断开即取消订阅；连接与鉴权同 F5。状态推送（总览自动刷新）v1 允许前端轮询 `/api/executors`，WS 状态通道留接口不实现（记录到变更记录，UI 走轮询）。
- **验收要点**：
  - 单测（假 WS 客户端）：连接 → 收尾部行 → 持续写入持续收到 → 断开无泄漏（goroutine/文件句柄回收断言）；
  - 未授权连接被拒。

### F5 · 鉴权与 CSRF 防护

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：所有 `/api/*` 与 `/ws/*` 校验 token 头（`Authorization: Bearer <api-token>` 或自定义头，方案记录；CLI 自动读 data/api-token，Web 由用户首次输入）；校验请求 Host 为 127.0.0.1/localhost 且 Origin 为空或同源（防 CSRF，D4）；失败 401/403；静态资源页本身免鉴权（SPA 壳），数据接口全拦。
- **验收要点**：
  - 单测：无 token 401；错 token 401；带假 Origin（http://evil.example）403；
  - 本机 curl（无 Origin）与浏览器同源请求均通过。

### F6 · CLI 惰性拉起 daemon

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：CLI 侧统一连接器：读 daemon.json → 探活 → 未运行则**后台拉起 daemon**（detach，等端口就绪，超时策略记录）→ 重试请求；拉起失败或连不上 → 退出码 5（daemon 未运行且无法自动拉起），错误文案给下一步（如执行 `awem service install`）。`awem daemon` 显式前台运行入口同 F1。
- **验收要点**：
  - 冒烟：杀掉 daemon → 跑任一 CLI 命令 → daemon 被自动拉起且命令成功；
  - 构造拉起失败（如锁死/环境坏）→ 退出码 5 + 中文指引。

### F7 · 静态资源与 reload 收口

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：`/` 挂载 go:embed SPA（T01 F5 升级：fallback 到 index.html 的 SPA 路由）；`POST /api/reload` 与配置热加载路径复用同一入口；`awem reload` CLI 子命令接通（正常由文件监视自动完成，手动入口是兜底）。
- **验收要点**：
  - 访问 `/` 与未知前端路由均返回 index.html；
  - `awem reload` 在手改 config.toml 后生效（后续命令看到新配置）。

## 完成与检查判定

- 全部功能点 🟠 且三绿 → 子任务状态改 🟠 已完成。
- 检查环节逐功能点跑"验收要点"（重点：401/403 矩阵、惰性拉起、端口写回不破坏配置），全 ✅ → 子任务 ✅；任一 ❌ → 打回 🔄 并登记"检查记录"。

## 检查记录

| 日期 | 功能点 | 结果 | 问题与处理 |
|---|---|---|---|
| 2026-10-25 | F1 | ✅ 通过 | 实跑 `go test ./internal/daemon/ -count=1`（4 用例全过）：TestDaemon单实例与重启（第二实例非零退出且报错含 port 18765 与 pid；kill -9 后锁随进程释放可重启，daemon.json 更新为新 pid；SIGTERM 优雅退出后 daemon.json 清理）、TestDaemon启动时配置无效拒绝运行、TestDaemon事件日志记录启停（events.log 含 daemon_start/daemon_stop）、TestLock互斥与释放；GOOS=windows/darwin 交叉编译通过。开发中修复：AWEM_CONFIG_DIR 曾被当作 userConfigDir 追加一层 awem/，语义改为直接作为根目录（见变更记录）。 |

## 变更记录

| 日期 | 动作 | 说明 |
|---|---|---|
| 2026-10-24 | 创建 | 由任务计划生成 |
| 2026-10-25 | 开工 | 子任务状态 ⬜→🔄，无人值守开发会话 |
| 2026-10-25 | F1 实现 + 选型记录 | **单实例锁方案**：独占文件锁 `data/daemon.lock`——Unix 用 `syscall.Flock`、Windows 用 `golang.org/x/sys/windows.LockFileEx`（标准库 syscall 无此 API，go.mod 已依赖 x/sys）；锁绑定 fd，进程退出（含 kill -9）由内核释放，无需陈锁清理。**daemon 装配**：`internal/daemon` 包 `Run(ctx)`（EnsureDir→锁→Load+Validate 拒无效→APIToken→logs.Manager→Watcher+Supervisor 订阅→WriteDaemonInfo→等 ctx→优雅收口）；`awem daemon` 子命令前台入口（signal.NotifyContext 接 SIGINT/SIGTERM）；platform 包新增 `Syncer` 接口 + `Noop` 空实现（T08 替换）。**实现细节**：新增环境变量 `AWEM_CONFIG_DIR`（值直接作为 awem 根目录，不追加 awem/；测试隔离与多实例冒烟用，config.At(root) 构造函数配套）；启动时配置无效 → daemon 拒绝启动（区别于热加载的"保留旧配置"）；daemon.json 写失败不致命（记事件）。集成测试编译真实二进制运行（build tag !windows）。 |
| 2026-10-25 | F1 ✅ | 验收要点实跑通过（见检查记录），4 用例 + 交叉编译三绿 |
