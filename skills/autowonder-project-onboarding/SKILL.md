---
name: autowonder-project-onboarding
description: 将当前项目接入 AutoWonder 平台（初始化小队/数字人/SDLC、配置仓库与权限、注册执行器并上线验证），以及接入后的日常使用（创建工单、需求澄清、启动交付、跟进排障）。当用户说「接入/接入 AutoWonder」「配置 AutoWonder 项目」「初始化交付小队」「注册执行器」「AutoWonder 工单怎么交付」等时使用。
---

# AutoWonder 项目接入与使用

把一个项目接入 AutoWonder 平台并跑通自动化交付，分**对接（onboarding）**与**使用（delivery）**两部分。
本 skill 的所有服务器地址、令牌、空间、模型等信息均在运行时向用户收集，**不内置任何环境信息**。

## 硬性原则

1. **零硬编码**：服务地址、MCP 配置、token、工作空间名/ID、模型 ID 一律来自用户或平台动态接口（模型列表必须调 `get_executor_launch_options` 现取，不许写死）。
2. **先问后写**：任何平台写操作前，把「信息收集清单」中缺失的项用卡片式提问（AskUserQuestion 或等价能力）补齐。
3. **写后必验**：每次批量写入后必须回读逐项断言（见 onboarding.md §7），接口返回成功 ≠ 数据落库。
4. **失败即停**：平台无跨对象事务。任一步失败停止后续写入，报告已创建/未完成清单，不承诺回滚、不自动删除。

## 前置：AutoWonder MCP 通道

执行本 skill 需要 AutoWonder MCP。宿主已注册 AutoWonder MCP 时（如宿主配置文件里的
`mcp.servers.autowonder`），直接复用其同一 URL 作为原始通道端点，不必向用户要；未配置时才
向用户要配置（通常形如
`{"autowonder": {"type": "http", "url": "<服务地址>/api/mcp/<令牌>/", "timeoutMs": 30000}}`），
按宿主方式注册后再继续。

两个通道、两套工具名：

| 通道 | 工具名前缀 | 何时用 |
|---|---|---|
| 宿主 MCP 客户端 | `autowonder_create_sdlc`（下划线） | 单次调用优先用它 |
| 原始 JSON-RPC（`POST <mcp-url>`，Streamable HTTP） | `autowonder.create_sdlc`（**点号**） | 批量写入；宿主客户端报 `-32602`（output schema 校验把空列表/null 判非法）时降级使用 |

批量写入用本 skill 自带的 `scripts/bulk_init.py`（内部走原始 JSON-RPC），**禁止**把含中文的大请求体（≥8KB）经 shell `--data` 内联传参——会被静默截断表现为「成功但未落库」。

## 阶段 0：信息收集清单

逐项核对，缺失的向用户询问（能从平台**或本机环境**读到的不要问用户）：

**A. 平台与空间**
- AutoWonder Web 地址（用户浏览器里打开的那个，如 `http://<host>:<port>`；帮助中心在 `<地址>/help?chapter=project-onboarding`）
- AutoWonder MCP 配置（上面格式）
- 目标工作空间：先调 `list_projects`，把 READ_WRITE/ADMIN 的空间做成卡片让用户选；没有可写空间时停止（**不要**替用户创建空间，除非用户明确要求）

**B. 模板**（从 AutoWonder 源码取，见 onboarding.md §2）
- 初始化哪些模板：常规项目默认「标准自动化交付」+「快速交付」两份；「AutoWonder自迭代交付小队」是 AutoWonder 自身研发专用，除非用户明确要。做成卡片确认。

**C. 项目变量**（模板占位符实例化的输入，缺一不可）
- 数据库配置：有没有外部数据库？没有则明确「无外部数据库，相关步骤记不适用」（桌面应用/纯前端项目常见）
- 部署前合并的目标分支（通常 = 仓库默认分支）
- 部署方式：桌面应用（本机构建+测试即部署，无 HTTP 健康检查）还是服务应用（环境/目录/命令/端口/健康检查/成功判定）。**写进配置前先在本机把构建和测试命令跑一遍确认是绿的**——预存失败的测试会成为交付门禁的拦路虎。
- 前端测试地址（模板含该变量时才问）

**D. 仓库**
- 仓库清单：Git URL、默认分支、描述（技术栈/职责边界）
- 仓库间关系（多仓库时）；单仓库无关系
- 绑定权限（常规全员 WRITE）

**E. 执行器**
- 执行器机器上装的是 `qoder` 还是 `qodercn`（决定 clientKind：QODER_CLI / QODER_CN_CLI）；
  本机已有执行器时（`awem list` 的 provider 字段 / 旧管理脚本）直接读出，不必问
- 哪些数字人需要执行器：交付链路上绑了 SDLC 的数字人 + 需求澄清；不绑 SDLC 的角色（如项目经理）不需要
- 模型偏好：需求澄清一般用旗舰模型，其余用 Flash 档（目录无旗舰时让用户统一挑一档）——但**用 `get_executor_launch_options` 现取列表**做卡片让用户挑，不硬编码
- 网络入口：启动命令默认返回服务端配置的（常为公网域名）。本机已有执行器且连接正常
  （`awem log` 有 `ws connected`）→ 直接复用它在连的入口；否则**先问用户执行器机器走公网还是内网地址**；
  公网连不上（典型报错 `Failed to fetch executor environment`）就换内网地址，scheme 同步降级 `https→http`、`wss→ws`

## 对接流程（六步）

按 [references/onboarding.md](references/onboarding.md) 执行，概要：

1. 确定工作空间（复用已有，见阶段 0.A）
2. 稀疏克隆 AutoWonder 源码取 harness 模板，做结构契约校验
3. 按项目实况实例化模板（占位符替换 + 残留校验，不回写母版）
4. 写入平台：SDLC→步骤→启用→数字人→SDLC 绑定→审核→发布→小队→成员，然后回读断言
5. 配置仓库 + 绑定全员权限 + **重新提交审核并发布**（绑定会进编辑版本，不重发布则在线版本不带权限）
6. 创建执行器 → 生成启动命令（按需换内网入口）→ awem 纳管启动（`awem add` + `awem start`）→ 验证 ONLINE

完成后输出：对象清单（小队/数字人/SDLC/执行器及 ID）、交付流程图（mermaid）、自动化链路与人工决策点划分。

## 使用流程

对接完成后，日常交付按 [references/delivery-usage.md](references/delivery-usage.md)：

- 建工单（REQ/BUG/TASK + 结构化 contentMd + 附件上传）→ 需求澄清（标准小队·需求澄清数字人，确认后产出 spec/plan）→ 启动交付（快速交付小队·快速全栈开发，或标准小队·全栈开发）
- 跟进：dispatch 状态、执行轨迹、评论、产物下载
- 团队协作：成员管理三档权限

## 排障速查

执行器离线/心跳、连接报错、写入静默失败、交付不动了——先查 [references/troubleshooting.md](references/troubleshooting.md)。

## 高频坑速查表

| 坑 | 正确做法 |
|---|---|
| 原始通道工具名找不到（`MCP工具不存在`） | 点号前缀 `autowonder.xxx`，不是下划线 |
| 含中文大请求体 curl 内联 | 写临时文件 `--data @file`，或用 Python 直发 |
| 绑定仓库后权限不生效 | 绑定写入编辑版本，必须再 submit + publish |
| 手动给数字人配 AutoWonder MCP | 不需要：平台每次派发自动注入内置 MCP（dispatch 级鉴权），显式同名绑定会被跳过 |
| 执行器启动命令连不上 | 公网入口不通时向用户要内网地址，wss→ws / https→http |
| 误判执行器离线 | 心跳间隔约 2 分钟；先 `awem log <名称>` 看有无断线，再让用户强刷页面、确认访问的是同一个入口 |
| 模型 ID 写死 | `get_executor_launch_options` 现取，目录变更后旧 ID 会被拒绝 |
| `create_sdlc` 传了空 workType | 模板 workType 为空就整个省略该参数 |
| `isDefault: true` 的模板 | 当前接口不支持设全局默认 SDLC，停止并报告 |
| 读接口 `-32602` | 是宿主客户端 output schema 校验失败，不是「对象不存在」；降级原始 JSON-RPC |
