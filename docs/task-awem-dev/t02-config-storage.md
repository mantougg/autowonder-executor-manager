# T02 · 配置与存储

- **子任务状态**：🔄 进行中（2026-10-24 开工，无人值守会话）
- **前置依赖**：T01
- **设计依据**：[design.md](../design/design.md) §5（数据模型与配置）、§10（安全）、D8（命名规则）
- **产物范围**：`internal/config/`、data 目录布局、api-token 与 daemon.json 读写

## 规范速查（开工前逐条确认；完整规范见 [conventions.md](conventions.md)）

1. 开工前通读 conventions.md 与 design.md §5/§10。
2. 文档优先：实现与 design.md 冲突 → 停下，先改设计文档征得用户同意。config.toml 的字段集合以 §5.2 为基准，措辞可微调，增删字段须先改文档。
3. 红线相关：API 相关配置只允许 `bind = "127.0.0.1"`（§5.2 示例如此，校验时不接受其他值）；敏感令牌明文落盘仅限本文约定的两处（config.toml 与 data/api-token），权限 0600。
4. 注释文档中文、标识符英文；功能点完成前 `go build/vet/test` 三绿。
5. 状态即改即记：本文件与 [index.md](index.md) 同步 + 双方变更记录各加一行。
6. 本任务专属：TOML 库选型（建议 BurntSushi/toml，理由记录）；热加载"坏配置不生效"语义必须实现。

## 功能点

### F1 · 配置目录与 data 布局

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-24 自检）
- **开发描述**：按 §5.1 用 `os.UserConfigDir()` 定位根目录 `awem/`（Linux `~/.config/awem/`、macOS `~/Library/Application Support/awem/`、Windows `%AppData%\awem\`），创建 `config.toml`（不存在则写默认模板）与 `data/logs/executors/` 目录树；目录权限 0700。提供单一入口（如 `config.Paths()`）供全项目取路径，禁止散落拼路径。
- **验收要点**：
  - 三平台路径符合 §5.1 表（单测可注入假 UserConfigDir 覆盖拼接逻辑）；
  - 首次运行生成默认 config.toml（含中文注释头）与完整 data 树；
  - 目录权限断言（非 Windows 下 0700）。

### F2 · config.toml 模型与读写

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：定义并实现 §5.2 全部结构：`[server]` bind/port（port=0 表示自动挑空闲端口写回，写回逻辑在 T06，本任务只保证字段语义）；`[platform]` base_url/mcp_token/poll_interval（可选段，缺省=平台联动整体禁用）；`[log]` max_size_mb=50/retention_days=7；`[[executor]]` name/command/autostart/`[executor.restart]` enabled/backoff（exponential：1s 起步 ×2 上限 5m，稳定 10m 重置——字段存策略，状态机在 T05）。读：容错解析 + 明确中文错误定位（行/条目）；写：人可读格式（缩进、节注释头）。
- **验收要点**：
  - 结构体字段与 §5.2 一一对应，默认值一致；
  - 读写 roundtrip 单测：写入→读出→深比较相等；
  - 缺省段（platform 未配、executor.restart 未配）取默认值不报错。

### F3 · 配置校验

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：加载时校验（§5.3/D8）：name 全局唯一且字符集 `[a-zA-Z0-9._-]`；command 能被解析出 `executor-id`（数字）与 `token`（非空）；`provider ∈ {qoder, qodercn, claude, codex}`；`bind` 仅 127.0.0.1。命令字段解析调用 T03 的解析器（T03 未完成前可先只做 name 唯一性，其余待 T03 后补齐——若走此路径须在本文件登记）。校验失败逐条报告（哪一条 executor、什么问题），不允许半份配置生效。
- **验收要点**：
  - 单测覆盖：重名、非法字符名（含中文名）、executor-id 非数字、token 为空、provider 非法、bind 非 127.0.0.1，各自报对应中文错误；
  - 错误信息含条目定位。

### F4 · 热加载（文件监视）

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：用 fsnotify（或等价机制）监视 config.toml，变更 → 原子重读 + 校验 → 成功则发布新配置给订阅者（接口预留：supervisor/api 在 T05/T06 订阅）；**失败（语法/校验错）保留旧内存配置继续运行**，并记一条错误事件（事件日志 T04 提供，先允许记到 stderr + 结构化回调，T04 完成后接上）。编辑器"写一半"与重复触发需去抖。
- **验收要点**：
  - 单测：改文件 → 订阅者收到新配置；写入坏 TOML → 旧配置仍在、错误事件产生；
  - 短时间多次写只触发一次重载（去抖）。

### F5 · api-token 生成与权限

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：daemon 首次启动生成随机 token（`crypto/rand` ≥32 字节，hex 编码）写 `data/api-token`，权限 0600（Windows：ACL 仅当前用户，用 `icacls` 或 Go 侧等价实现）；已存在则直接读。提供读取接口供 API 鉴权（T06）与 CLI（自动读，无需用户输入）使用。
- **验收要点**：
  - 单测：首次生成（长度/随机性）、二次读取复用不重新生成；
  - 非 Windows 权限断言 0600；Windows 路径下代码分支存在（真机验证记 T10）。

### F6 · daemon.json（pid/port 元数据）

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：提供 daemon 元数据读写：启动时写 `{pid, port}`，正常退出时清理；读取方（CLI 惰性拉起 T06 用）读到 stale 记录（pid 不存活）视为无 daemon。进程被强杀留下的 stale 文件场景要能自愈（读时校验，不崩溃）。
- **验收要点**：
  - 单测：写/读/清理 roundtrip；pid 不存在时判定 stale；
  - 文件损坏（非 JSON）时读取不 panic、按无 daemon 处理。

## 完成与检查判定

- 全部功能点 🟠 且三绿 → 子任务状态改 🟠 已完成。
- 检查环节逐功能点跑"验收要点"，全 ✅ → 子任务 ✅；任一 ❌ → 打回 🔄 并登记"检查记录"。

## 检查记录

| 日期 | 功能点 | 结果 | 问题与处理 |
|---|---|---|---|
| 2026-10-24 | F1 | ✅ | 实跑 `go test ./internal/config/ -v` 3 用例 PASS：Layout 注入假目录断言 8 条路径；Linux 下 XDG_CONFIG_HOME 覆盖验证 Root=$XDG/awem（其余平台根目录由 os.UserConfigDir 标准库承担，§5.1 三平台表对应）；EnsureDir 创建 4 级目录树 + 默认 config.toml（中文注释头）、目录权限 0700 断言、二次调用不覆盖用户配置 |

## 变更记录

| 日期 | 动作 | 说明 |
|---|---|---|
| 2026-10-24 | 创建 | 由任务计划生成 |
| 2026-10-24 | 开工 ⬜→🔄 | 无人值守开发会话继续（T01 完成后） |
| 2026-10-24 | F1 ✅ | `config.Layout/DefaultLayout/EnsureDir`（路径单一入口）+ 默认模板，权限/幂等单测通过 |
