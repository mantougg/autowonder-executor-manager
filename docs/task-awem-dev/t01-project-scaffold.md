# T01 · 工程骨架与构建基础

- **子任务状态**：✅ 已检查（2026-10-24 全部功能点自检通过）
- **前置依赖**：无（首个子任务）
- **设计依据**：[design.md](../design/design.md) §4（总体架构）、§16（工程约定）、§8.2（退出码）
- **产物范围**：`go.mod`、`cmd/awem/`、`internal/` 各包占位、`web/` 占位、`Makefile`

## 规范速查（开工前逐条确认；完整规范见 [conventions.md](conventions.md)）

1. 开工前通读 conventions.md 与 design.md §4/§16。
2. 文档优先：实现与 design.md 冲突 → 停下，先改设计文档征得用户同意。
3. 注释与文档中文、标识符英文；目录名用设计规定的小写英文。
4. 功能点完成以 `go build ./...`、`go vet ./...`、`go test ./...` 三绿为前提。
5. 状态即改即记：本文件与 [index.md](index.md) 同步更新 + 双方变更记录各加一行。
6. 本任务专属：选型（CLI 框架、module 名）必须写入"变更记录"，写清选了什么、为什么。

## 功能点

### F1 · Go module 与目录骨架

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-24 自检）
- **开发描述**：`go mod init awem`（module 名 `awem`，本地无远程仓库路径；后续开源时改 import path 成本可控，理由记变更记录）。按 design.md §16 建目录：`cmd/awem/`（入口）、`internal/config/`、`internal/launchcmd/`（启动命令解析，T03 主场）、`internal/logs/`、`internal/supervisor/`、`internal/platform/`、`internal/api/`、`internal/cli/`，每个包放一个说明用途的中文包注释占位文件；`web/` 放 `dist/index.html` 占位。加 `.gitignore`（二进制、`web/node_modules/`、`web/dist/` 是否忽略取决于构建策略：embed 需要产物入库或构建时生成，**本点先约定"构建时生成、仓库忽略"并写进变更记录**）。
- **验收要点**：
  - 目录结构与 design.md §16 一致，各包有中文包注释；
  - `go build ./...` 三绿（build/vet/test）通过；
  - 变更记录里有 module 名与 dist 策略的选型说明。

### F2 · 入口 main 与子命令分发框架

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-24 自检）
- **开发描述**：`cmd/awem/main.go` 实现子命令注册与分发骨架：`usage()` 输出中文用法；未知子命令/缺参数 → 退出码 2；框架层支持全局 flag `--json`、`--yes` 的解析与透传（各子命令自己消费）。本期只注册 `version` 一个真实子命令（T01 内可用），其余子命令占位注册、调用即报"未实现"（退出码 1），保证后续任务往框架里填肉不改骨架。选型建议：标准库 `flag` 自举（零依赖、退出码语义自控）；若改用 cobra 等需在变更记录说明。
- **验收要点**：
  - `awem version` 输出版本号（可先为 `0.0.0-dev`，ldflags 注入位留好）；
  - `awem 不存在命令` 退出码 2 且有中文用法提示；
  - `--json` 全局 flag 能被子命令读到。

### F3 · 退出码常量与错误分类

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-24 自检）
- **开发描述**：在 `internal/cli/`（或独立 `internal/exitcode` 包，执行者定并记录）定义退出码常量（0 成功/1 一般/2 用法/3 未找到/4 已存在/5 daemon 未运行）与对应错误类型（用法错误/未找到/已存在/daemon 未运行），main 统一把错误类型映射到退出码并打印中文错误信息（`--json` 时输出 `{"error": …}` 结构）。后续所有子任务报错只允许用这些类型。
- **验收要点**：
  - 常量值与 design.md §8.2 表完全一致；
  - 单测覆盖 错误类型 → 退出码 的映射；
  - `--json` 模式下错误输出是合法 JSON。

### F4 · Makefile 与开发工作流

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-24 自检）
- **开发描述**：`Makefile` 目标：`build`（含 web 前端构建钩子，`web/` 未建时跳过并提示）、`test`（go test 全量）、`vet`、`clean`；版本注入变量（`-ldflags "-X …version=…"`）预留。README 增补"开发者快速开始"一节（中文：环境要求 Go ≥ 当前稳定版、make build/test）。
- **验收要点**：
  - `make build` 产出可执行 `awem`，`make test`/`make vet` 绿；
  - 版本号能通过 ldflags 注入并被 `awem version` 读到。

### F5 · go:embed 静态资源骨架

- **开发状态**：🟠 已完成
- **检查状态**：✅ 通过（2026-10-24 自检）
- **开发描述**：`web/dist/` 占位页（含 `awem` 字样的最小 HTML）通过 `go:embed` 打进二进制；`internal/api/` 提供"返回嵌入的 index.html"最小 handler（本任务不要求鉴权/路由，T06 再完善）。目的：从第一天起保证"单二进制含 Web"的形态成立，后续 T09 只是替换 dist 内容。
- **验收要点**：
  - 构建产物单文件可运行，访问占位路径能拿到嵌入页（可用 go 单测 httptest 验证 handler 返回 200）；
  - embed 路径与 `web/dist` 约定一致。

## 完成与检查判定

- 全部功能点 🟠 且三绿 → 子任务状态改 🟠 已完成。
- 检查环节逐功能点跑"验收要点"（实际执行命令/读代码对照），全 ✅ → 子任务 ✅；任一 ❌ → 该功能点打回 🔄 并登记"检查记录"。

## 检查记录

| 日期 | 功能点 | 结果 | 问题与处理 |
|---|---|---|---|
| 2026-10-24 | F1 | ✅ | 实跑 `go mod init awem` 后 `go build ./... && go vet ./... && go test ./...` 三绿（输出 no test files，无报错）；目录树与 design.md §16 一致（cmd/awem + internal/{config,launchcmd,logs,supervisor,platform,api,cli} 各含中文包注释 doc.go，web/ 含占位页）；变更记录已含 module 名与 dist 策略选型 |
| 2026-10-24 | F2 | ✅ | 实测真实二进制：`awem version`→`awem 0.0.0-dev`、`awem --json version`→`{"version":"0.0.0-dev"}`；`awem 不存在命令`与无参数均退出码 2 且输出中文用法；`awem add`（占位）→`错误：子命令 "add" 尚未实现` 退出码 1；单测 `go test ./internal/cli/ -v` 全 PASS（含全局 flag 透传、`--flag=false` 形式、退出码用例） |
| 2026-10-24 | F3 | ✅ | 单测 `go test ./internal/exitcode/ ./internal/cli/` 全 PASS：常量断言 0/1/2/3/4/5 与 design.md §8.2 一致；5 种构造器 → Code()/Type() 映射、Wrap+嵌套 errors.As 穿透、裸错误回退 1 均有用例；捕获 stdout 验证 writeError JSON 输出合法可解析；实测二进制 `awem --json 不存在命令`→`{"error":{"code":2,…,"type":"usage"}}` 退出码 2、`awem --json add`→code 1 |
| 2026-10-24 | F4 | ✅ | 实跑：`make build VERSION=v0.0.1-accept` 产出 `./awem` 且 `awem version`→`awem v0.0.1-accept`（ldflags 注入生效）；`make test`、`make vet` 退出码 0；`make clean` 后二进制消失；删除 web/dist/ 再 `make build` 自动从 web/placeholder/ 重建占位页；README 增补"开发者快速开始"一节（环境要求 + make 目标 + dist 策略说明） |
| 2026-10-24 | F5 | ✅ | 实跑 `go test ./internal/api/ -v`：TestIndexHandler返回内嵌页 PASS（GET /、/index.html、/任意未定义路径 均 200、body 含 awem、Content-Type text/html）；`make build` 产出单文件 `./awem`（ELF 静态链接 2.8MB）；embed 路径核对：web/embed.go `//go:embed all:dist` 与 web/dist 约定一致；三绿通过 |

## 变更记录

| 日期 | 动作 | 说明 |
|---|---|---|
| 2026-10-24 | 创建 | 由任务计划生成 |
| 2026-10-24 | 开工 ⬜→🔄 | 无人值守开发会话开始执行 T01 |
| 2026-10-24 | F1 选型：module 名 | `go mod init awem`。选短名而非仓库名全路径：设计要求二进制/包名即 `awem`，本地无远程 module path；后续开源改 import path 仅涉及各 doc.go 无外部引用，成本可控（任务文档预留的方案） |
| 2026-10-24 | F1 选型：web/dist 策略 | **构建时生成、仓库忽略**（.gitignore 忽略 web/dist/）。占位页源文件入库为 `web/placeholder/index.html`，Makefile build 在 `web/dist/` 缺失时自动复制占位页（F4 实现），保证 fresh clone 走 make 可构建；go:embed 不读 .gitignore，本地有 dist 即可嵌入（F5） |
| 2026-10-24 | F1 ✅ | 目录骨架 + module 初始化完成，三绿通过 |
| 2026-10-24 | F2 选型：CLI 框架 | **不用 cobra，标准库自举**（internal/cli 手写 Registry+Dispatch）。理由：退出码 0/1/2/3/4/5 语义需完全自控（cobra 的 SilenceUsage/SilenceErrors 处理退出码绕）、零依赖符合单二进制极简形态、子命令规模小（14 个）不需要自动补全等重特性。全局 flag `--json/--yes` 由分发器从 argv 任意位置抽出（支持 `--flag=false` 形式）透传给子命令；版本号变量放 `internal/version`（ldflags 注入点） |
| 2026-10-24 | F2 ✅ | 分发框架 + version 子命令 + 13 个占位命令，验收通过 |
| 2026-10-24 | F3 选型：包位置 | 独立 `internal/exitcode` 包（非放 internal/cli）。理由：internal/api、supervisor 等非 CLI 层也要构造带退出码语义的错误，独立包避免反向依赖 CLI 框架。错误类型 `*exitcode.Error{Code,msg,Err}` + 构造器（Usagef/NotFoundf/AlreadyExistsf/DaemonNotRunningf/Generalf/Wrap）+ `Code(err)`/`Type(err)` 提取；--json 错误 schema 定为 `{"error":{"code":N,"type":"…","message":"…"}}`（type 为机器可读名：general/usage/not_found/already_exists/daemon_not_running，只增不改） |
| 2026-10-24 | F3 ✅ | 退出码常量 + 错误类型 + Dispatch 集成，单测与实测通过 |
| 2026-10-24 | F4 ✅ | Makefile（build/test/vet/clean + web-dist 钩子 + VERSION ldflags）与 README 开发者快速开始 |
| 2026-10-24 | F5 选型：embed 承载位置 | embed Go 源码放 `web/embed.go`（package web，`//go:embed all:dist`）。理由：go:embed 只能嵌入本包目录子树，`web/` 是唯一能直达 dist/ 且语义正确的位置；web/ 同时作为 React 工程根（T09 落位 package.json），Go 工具链会忽略非 .go 文件，互不干扰。api 层经 `web.DistFS()/IndexHTML()` 消费，SPA 路由/鉴权留给 T06 |
| 2026-10-24 | F5 ✅ | embed 骨架 + api.IndexHandler，httptest 与构建产物验证通过 |
| 2026-10-24 | 子任务 ✅ | 5/5 功能点检查通过，三绿，子任务完成 |
