# T03 · 启动命令解析与导入

- **子任务状态**：🔄 进行中（2026-10-25 开工，无人值守会话）
- **前置依赖**：T01（与 T02 的名称/校验规则衔接：解析结果供 T02 F3 校验使用）
- **设计依据**：[design.md](../design/design.md) §5.3（命令解析与结构化字段）、§8.1（add/import 命令）、决策 #8/#15
- **产物范围**：`internal/launchcmd/`（纯函数库：解析、重建、导入解析）+ 单测矩阵

## 规范速查（开工前逐条确认；完整规范见 [conventions.md](conventions.md)）

1. 开工前通读 conventions.md 与 design.md §5.3。
2. 文档优先：字段集合以 §5.3 为准（runtime 版本、ws-url、token、executor-id、provider、model、reasoning-effort、context-window、memory-mode、max-tasks、--token-aware-enable、--debug + 未知 flag passthrough）；要支持新 flag 先改文档。
3. 本任务全部是纯函数库（不碰进程、不碰网络），**必须单测驱动**，测试矩阵写在 F6。
4. 注释文档中文、标识符英文；功能点完成前 `go build/vet/test` 三绿。
5. 状态即改即记：本文件与 [index.md](index.md) 同步 + 双方变更记录各加一行。
6. 本任务专属：未知 flag **原样保序保留**是硬要求（roundtrip 不丢任何信息）；旧脚本名称常为中文，与 D8 字符集冲突，导入必须走改名流程。

## 功能点

### F1 · 命令 tokenize 与前缀识别

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-25 自检）
- **开发描述**：shell 风格拆词（处理单/双引号、转义、多空格/Tab），识别 `npx [-y] autowonder@<ver> connect …` 前缀结构，提取 runtime 版本 `<ver>`；容忍用户粘贴时带前导 `nohup`、尾部 `&` 等旧脚本残留（ stripping 规则记录到变更记录）。非 `autowonder connect` 命令报"不支持的命令"用法错误（退出码语义归 T07，本层返回结构化错误）。
- **验收要点**：
  - 单测：引号包裹的 token、多空格、`nohup … &` 包装、版本号提取、非 npx/autowonder 前缀报错；
  - 拆词结果与 shell 语义一致（引号内容不成词边界）。

### F2 · 结构化字段解析

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-25 自检）
- **开发描述**：从 connect 参数解析出 §5.3 全部已知字段：`--ws-url`、`--token`、`--executor-id`、`--provider`、`--model`、`--reasoning-effort`、`--context-window`、`--memory-mode`、`--max-tasks`、布尔 `--token-aware-enable`、`--debug`；未知 flag 进 passthrough 列表（**保序**）。校验：executor-id 数字、token 非空、provider 枚举（qoder/qodercn/claude/codex）；错误逐字段定位（哪个 flag、什么问题）。
- **验收要点**：
  - 单测覆盖每个字段正常/缺失/非法三种情况；
  - `--flag=value` 与 `--flag value` 两种写法都支持；
  - 未知 flag（如未来的新参数）完整保留在 passthrough。

### F3 · 命令重建（roundtrip）

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-25 自检）
- **开发描述**：结构化字段 + passthrough → 重新生成完整命令串（编辑视图保存时用）。保证：`parse(render(parse(x))) == parse(x)`（幂等）；未知 flag 不丢；渲染顺序稳定（已知字段按固定顺序，passthrough 按原顺序追加）；token 等含特殊字符的值正确加引号。
- **验收要点**：
  - 幂等性单测（含 passthrough 的复杂样例）；
  - 编辑任一已知字段后重建，passthrough 原样保留；
  - 含空格/引号的值 roundtrip 不变形。

### F4 · 旧脚本 import 解析

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：解析旧 `executor-manager.sh` 的 `EXECUTORS` 数组（真实形态见 `~/.agents/skills/autowonder-project-onboarding/templates/executor-manager.sh`：每行 `"名称|启动命令"`，可能混入注释行/空行/示例占位符）。产出 `[]Imported{Name, Command}`；名称不合规（中文等，违反 D8 字符集）→ 生成合规建议名（如拼音/编号方案，策略记录）并标记"需改名确认"；命令部分复用 F1/F2 解析。
- **验收要点**：
  - 用真实模板文件做单测 fixture（拷入 `testdata/`）；
  - 注释行、空行、含 `<runtimeVersion>` 等未替换占位符的示例行被跳过或标记，不静默导入坏数据；
  - 中文名触发改名建议而非直接失败。

### F5 · 旧日志归档（--with-logs 数据准备）

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：提供归档函数：把旧脚本布局 `~/.autowonder-executor/<name>.log` 拷贝到 awem 的 `data/logs/executors/<name>.log`（名称以导入时确认的新名为准）；源不存在则跳过不报错；拷贝记录结构化返回（供 CLI 汇总输出）。不删源文件。
- **验收要点**：
  - 单测：存在/不存在/改名映射三种情况；
  - 只读源、只写目标，不产生副作用于旧目录。

### F6 · 单测矩阵与边界覆盖

- **开发状态**：⬜ 未开始
- **检查状态**：⬜ 未检查
- **开发描述**：汇总建立 `internal/launchcmd` 的测试矩阵并补齐缺口：正常完整命令、缺字段、坏值、引号/空格变形、nohup 包装、未知 flag 多个、--flag=value 形式、中文/非法名、幂等 roundtrip、旧脚本全文 fixture。本功能点是"测试完备性"的收口，前五个功能点的测试若有遗漏在此补全。
- **验收要点**：
  - `go test ./internal/launchcmd/...` 全绿且覆盖上述每类场景（表驱动用例清单可在测试文件注释中索引到本节）；
  - 覆盖率不低于 85%（`go test -cover`）。

## 完成与检查判定

- 全部功能点 🟠 且三绿 → 子任务状态改 🟠 已完成。
- 检查环节逐功能点跑"验收要点"（实际执行测试、抽查用例），全 ✅ → 子任务 ✅；任一 ❌ → 打回 🔄 并登记"检查记录"。

## 检查记录

| 日期 | 功能点 | 结果 | 问题与处理 |
|---|---|---|---|
| 2026-10-25 | F1 | ✅ | 实跑 `go test ./internal/launchcmd/ -v` 5 用例全 PASS：单/双引号包裹空格、引号内另一引号、双引号转义 `\`"`、裸转义空格、相邻引号成一词（拆词与 shell 语义一致）；未闭合引号报错；版本提取（0.3.22）；nohup 叠加与尾部 & 剥离；7 种非法前缀（非 npx/无版本/@空/缺 connect/其他包/不支持 npx 参数/空命令）均返回 UnsupportedCommandError |
| 2026-10-25 | F2 | ✅ | 实跑 `go test ./internal/launchcmd/ -run 'Parse|Validate' -v` 全 PASS：全字段命令解析结果深比较相等且 Validate 通过；`--flag=value` 与 `--flag value` 两形式等价；布尔 flag 纯式/`=false`/`=true`/非法值；未知 flag（--future-flag=v1、--another 42、裸词）完整保序进 passthrough；末尾缺值与"值位置是另一个 flag"均逐 flag 定位；executor-id 非数字/缺失、token 空、provider 非法/缺失各自报对应中文错误，2 项问题聚合计数 |
| 2026-10-25 | F3 | ✅ | 实跑 `go test ./internal/launchcmd/ -run Render -v` 全 PASS：乱序输入渲染为固定顺序精确串比对；4 条含 passthrough/布尔/引号命令 assertRoundtrip（parse(render(parse))==parse 深比较）；空格/双引号值加单引号且 roundtrip 不变形；编辑 model/reasoning-effort 后 passthrough `--future-keep=v9` 原样保留；false 布尔不渲染；quoteValue 9 组边界（裸/带引号/内嵌单引号/前导横线/空串） |

## 变更记录

| 日期 | 动作 | 说明 |
|---|---|---|
| 2026-10-24 | 创建 | 由任务计划生成 |
| 2026-10-25 | 开工 ⬜→🔄 | 无人值守会话（T02 ⏸️ 转 T03，完成后回填 T02-F3） |
| 2026-10-25 | F1 选型：包装剥离规则 | 剥离规则：**前导 nohup（可叠加）+ 尾部 &**；npx 与包名间仅允许 `-y/--yes`，其余 npx 参数（如 --silent）报"不支持的命令"而非静默丢弃；`autowonder` 必须带 `@<版本>`。未闭合引号/悬挂反斜杠在 tokenize 层报错 |
| 2026-10-25 | F1 ✅ | Tokenize + parsePrefix（版本提取、包装容忍、结构化 UnsupportedCommandError）单测通过 |
| 2026-10-25 | F2 选型：解析/校验分层 | 语法层问题（缺值、布尔带值、未知裸词）解析时报 FieldErrors（逐 flag 定位）；语义层三规则（executor-id 数字 / token 非空 / provider 枚举且须显式）由 `Validate()` 单独报，便于编辑场景区分"结构坏了"与"值不合法"。重复已知 flag 后者覆盖前者；值位置出现 `-` 开头的词按缺值报错（token 等值实际不含横线前缀）；多余裸词进 passthrough 保序 |
| 2026-10-25 | F2 ✅ | Parsed 结构 + Parse/Validate，全字段/两形式/保序单测通过 |
| 2026-10-25 | F3 选型：引用规则 | 值按保守安全字符集 `[A-Za-z0-9._~:@/+=,%-]` 裸输出（含 `-`），集合外或以 `-` 开头的值用单引号包裹（内嵌 `'` 转义为 `'\''`）；已知字段固定顺序渲染（ws-url→token→executor-id→provider→model→reasoning-effort→context-window→memory-mode→max-tasks→布尔→passthrough 原序）；布尔 false 不渲染（重解析同为 false，幂等成立） |
| 2026-10-25 | F3 ✅ | Render + quoteValue，幂等/顺序/编辑保 passthrough 单测通过 |
