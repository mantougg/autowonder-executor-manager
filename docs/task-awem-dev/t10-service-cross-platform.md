# T10 · 系统服务与跨平台专项

- **子任务状态**：🔄 进行中
- **前置依赖**：T06（daemon）、T07（CLI 框架）
- **设计依据**：[design.md](../design/design.md) §12（系统服务）、§13（跨平台注意事项）、决策 #12/#13
- **产物范围**：`awem service install|uninstall|status`（三平台）、Windows 专项适配验证、全链路冒烟回归

## 规范速查（开工前逐条确认；完整规范见 [conventions.md](conventions.md)）

1. 开工前通读 conventions.md 与 design.md §12/§13。
2. 文档优先：三平台服务机制以 §12 表为准；换机制（如 Windows 从 sc create 改 Startup）先改文档。
3. 开发环境是 Linux：Linux 路径完整实现并测试；macOS/Windows 路径**代码实现 + 真机验证记入检查记录**（无法真机时明确标注"未真机验证"，不许静默标 ✅）。
4. 注释文档中文、标识符英文；功能点完成前 `go build/vet/test` 三绿。
5. 状态即改即记：本文件与 [index.md](index.md) 同步 + 双方变更记录各加一行。
6. 本任务专属：服务文件模板内容（unit/plist/服务参数）记入变更记录，便于评审。

## 功能点

### F1 · service 子命令框架

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：`awem service install|uninstall|status` 骨架：按 runtime.GOOS 分发到平台实现；install = 写服务定义 + 启用 + 启动 daemon；uninstall = 停止 + 注销 + 清理定义文件；status = 已安装/运行中/未安装（含 daemon pid/port）；操作结果写事件日志（T04）；重复 install 幂等（已安装提示，不报错覆盖策略记录）。
- **验收要点**：
  - 三平台分支齐全（未实现路径明确报错而非空跑）；
  - status 三态判定正确（Linux 真机，其他平台代码走查）。

### F2 · Linux systemd user 服务

- **开发状态**：✅ 已检查
- **检查状态**：✅ 通过
- **开发描述**：写 `~/.config/systemd/user/awem.service`（模板记变更记录：ExecStart 指向当前二进制绝对路径、Restart=on-failure、日志走 journald 或沿用文件日志——沿用自身文件日志并记录理由）；`systemctl --user enable --now awem`；install 后提示 `loginctl enable-linger <用户>`（不自动开，文案说明作用：不登录也常驻）；uninstall 对应 disable/stop/rm。
- **验收要点**：
  - Linux 真机：install → daemon 常驻且 `awem status` 可查 → reboot/登出场景说明（linger 提示出现）→ uninstall 干净（unit 文件删除、daemon 停止）；
  - 二进制路径含空格/中文用户名的 quoting 正确。

### F3 · macOS launchd 服务

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：写 `~/Library/LaunchAgents/com.autowonder.awem.plist`（RunAtLoad + KeepAlive；模板记变更记录）；`launchctl load/unload`；uninstall 清理 plist。
- **验收要点**：
  - plist 字段与 §12 一致；路径按 HOME 展开；
  - 有 macOS 真机则实测安装/卸载；无则在检查记录标注"未真机验证"。

### F4 · Windows 服务

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：按 §12：`sc create` 注册 Windows 服务（以当前用户账户运行）**或**登录自启（Startup 目录/注册表 Run）备选——两方式取舍写入文档与 `--help` 文案：服务方式注意 session 0 限制（Node/qoder 环境变量须机器级）；实现选定方式 + 文档写明另一种；uninstall 对应 `sc delete`/清理。
- **验收要点**：
  - 实现 + 帮助文案含两种方式取舍说明；
  - 有 Windows 真机则实测；无则标注"未真机验证"，并把它列入 index.md 待办备注。

### F5 · Windows 专项适配验证

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：集中处理 §13 Windows 事项并与 T05 路径对齐：进程树终止（`taskkill /T /F` 或 Job Object——最终方案定稿记录）；UTF-8（控制台代码页 `chcp 65001` 提示 / Go 侧强制 UTF-8 管道，处理中文日志不乱码）；`npx.cmd` 解析确认；路径（反斜杠/空格/中文用户目录）处理。**代码在 Go 侧 build 时保证可编译（GOOS=windows 交叉编译检查）**，真机行为验证记录。
- **验收要点**：
  - `GOOS=windows GOARCH=amd64 go build` 通过；
  - 真机（若有）：启动/停止/日志中文正常、无残留进程；无真机则标注"未真机验证"。

### F6 · 全链路冒烟回归与 doctor 跨平台完善

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：Linux 本机全链路冒烟脚本（假执行器，不依赖平台）：install service → daemon 拉起 autostart 假执行器 → 崩溃自动恢复 → `awem list/status/log` 输出正确 → Web 页可访问（curl SPA 壳 + API 401/200）→ stop 全部 → uninstall 无残留（进程/文件/服务）。doctor 在目标平台的检查项复核（Node/npx/磁盘按平台差异）。文档补"7×24 长跑推荐 Linux/macOS/WSL"说明（§13）。
- **验收要点**：
  - 冒烟脚本入库（如 `scripts/smoke.sh`）一键全绿；
  - 每一步退出码/输出断言，失败即停并报告步骤；
  - README/docs 有平台推荐说明。

## 完成与检查判定

- 全部功能点 🟠 且三绿 → 子任务状态改 🟠 已完成。
- 检查环节逐功能点跑"验收要点"（重点：Linux 真机全链路、macOS/Windows 标注真机验证状态），全 ✅ → 子任务 ✅；任一 ❌ → 打回 🔄 并登记"检查记录"。**"未真机验证"的平台项不得标 ✅，标 ⏸️ 并在 index.md 备注。**

## 检查记录

| 日期 | 功能点 | 结果 | 问题与处理 |
|---|---|---|---|
| 2026-10-25 | F1 | ✅ 通过 | 实跑 `go test ./internal/service/ -count=1`（ForOS 平台分发：freebsd 明确报错、当前平台实现可用；unit 模板断言 ExecStart 引号+daemon 子命令+Restart/WantedBy）；三平台分支齐全由 GOOS=linux/darwin/windows go build 交叉编译证明（全绿）。 |
| 2026-10-25 | F2 | ✅ 通过（Linux 真机） | 本机 systemd user 实测全链路：未安装态 status 正确 → install（unit 写入、enable --now）→ systemctl --user is-active = active → `awem service status` = 已安装且运行中（daemon pid/port 显示）→ CLI `awem status` 经 API 查询成功 → 事件日志含 service_installed 与 enable-linger 提示 → 重复 install 幂等（仍 active）→ uninstall 干净（unit 文件删除、inactive、无 awem 进程、daemon.json 随优雅退出清理）→ 再 uninstall 幂等。**开发中发现并修复**：首版 ExecStart 漏 daemon 子命令 → unit 反复重启（is-active 恒 activating），根因定位（真 daemon 是 CLI 惰性拉起的干扰项）后修复复测 active。 |

## 变更记录

| 日期 | 动作 | 说明 |
|---|---|---|---|
| 2026-10-24 | 创建 | 由任务计划生成 |
