# AutoWonder 项目对接手册（六步）

前置：已完成 SKILL.md 阶段 0 的信息收集。下文 `<mcp-url>` 指用户提供的 AutoWonder MCP 地址。

## 1. 确定工作空间

调 `list_projects`，与用户选定的空间核对：访问级别必须 READ_WRITE 或 ADMIN。
空间应已存在（对接不负责创建空间）；空间内先读 `list_squads` / `list_agents` / `list_sdlcs` 做预检：
同名且配置可确认一致 → 复用并只补缺失；同名但关键字段不同或无法确认 → 停止报告冲突，不覆盖不删除。

## 2. 获取模板

稀疏检出 AutoWonder 源码（公开仓库，与服务器版本解耦）：

```bash
git clone --filter=blob:none --no-checkout --depth 1 \
  https://github.com/aliyun/alibabacloud-landing-zone.git
cd alibabacloud-landing-zone
git sparse-checkout init --no-cone
git sparse-checkout set /ai-sdlc/auto-wonder/
git checkout master
# 模板在 ai-sdlc/auto-wonder/skills/initialize-autowonder-harness/harness-config.json
```

### 配置契约（写入前一次性校验，失败即停）

- 顶层是非空数组，每项结构 `template + squad + agents[].sdlc?.steps[]`
- `template.name/description`、`squad.name`、`agents` 存在；`template.squadSize === agents.length`
- 数字人 `name`、`roleCode` 模板内唯一
- **多份模板同初始化时，跨模板 `name`/`roleCode` 也必须唯一**（bulk_init 按 roleCode 全局索引，冲突会静默漏建数字人并把错误 SDLC 绑到共享数字人上）：「标准自动化交付」与「AutoWonder自迭代交付小队」共享 AW_FS_DEV/AW_CR/AW_QA/AW_REQ_CLARIFIER 四个 roleCode，实例化时把后者改写为 `AWSELF_` 前缀——两模板步骤的 handlerRoleRef 只引用各自模板内编码，无跨引用，改写安全；`load_instances` 会断言拦截
- `sdlc` 可为 null；非空时必须有 `name` 和非空 `steps`
- 步骤 `order` 严格升序不重复，数组首项即入口
- `checklist` 是数组或 null；`gatePolicy` 是对象或 null
- `isDefault: true` → 当前无接口支持，停止并报告；false → 不传
- 配置中的平台 ID / 租户 / 审计 / 版本字段不参与初始化

### 模板清单（截至撰写时）

| 模板 | 规模 | 适用 |
|---|---|---|
| 标准自动化交付 | 5 人 3 SDLC（开发 7 步/评审 1 步/QA 8 步）+ 需求澄清 + 项目经理 | 全链路质量门禁 |
| 快速交付 | 2 人 2 SDLC（各 3 步） | 轻量快交付，首次接入默认配 |
| AutoWonder自迭代交付小队 | 8 人 | AutoWonder 自身研发专用，普通项目不初始化 |

## 3. 实例化模板（变量替换）

前两份模板含 3 个中文占位符（`{{请填写…}}`）；AutoWonder 自迭代模板用 `{{CUSTOMER.*}}` 三变量。替换规则：

- **在解析后的 JSON 字符串字段里按完整 token 替换**，同一变量全部出现位置用同一值；不要在序列化后的整段 JSON 上做字符串替换
- 替换值必须已向用户收集（见 SKILL.md 阶段 0.C）；用户明确「无数据库/无前端」时，把「记不适用并跳过」写进替换值本身
- 实例副本单独保存，**不回写模板母版**
- 写入前递归扫描：无 `{{请填写`、无 `{{CUSTOMER.` 残留；缺失值/未知变量先补齐

三块替换值的填写范式：

```markdown
### 数据库配置
（无外部数据库时）本项目无外部数据库。<项目形态一句话>。本步骤一律记「不适用」并继续：不连接、不备份、不执行任何数据库变更。
（有数据库时）数据库类型/版本/地址/端口/库名 + 配置读取方式（环境变量名或配置文件路径）+ 凭据引用方式（凭据本体不进配置）。

### 合并目标分支
<分支名>（通常为仓库默认分支）

### 部署方式
（服务应用）环境/部署目录/构建与启动命令/服务访问地址/健康检查/成功判定标准。
（桌面或纯前端应用）无服务器与 HTTP 服务地址，「部署」定义为本机构建与测试验证：环境依赖、目录、构建与测试命令、成功判定；GUI 启动/打包类命令标注「默认不执行，工单明确要求时才执行」。
```

## 4. 写入平台

**顺序不可乱**：先 SDLC 后数字人（数字人绑定需要 SDLC ID）。批量执行用 `scripts/bulk_init.py`（推荐），或按下表手工调用。

| 步 | 调用 | 字段映射与注意 |
|---|---|---|
| a | `create_sdlc` | 传 name/description；workType 为空则省略 |
| b | `add_sdlc_step` | `order→stepOrder`、`instruction→instructionMd`、`checklist→checklistJson`、`gatePolicy→gatePolicyJson`（均 JSON 序列化）；`onSuccess/onFail` 模板里是对象、接口要 JSON 字符串；`timeoutSeconds` 为 null 就省略；`statusOnEnterCode` 非 null 才传；`required` 布尔直传 |
| c | `enable_sdlc` | 模板 status=ENABLED 时调用；无需 statusTemplateId |
| d | `create_agent` | `businessBackground→soulMd`、`responsibilities→agentMd`，加 name/roleCode/roleName |
| e | `update_agent_config` | `sdlcId`（无 SDLC 显式 null）+ `evolutionMode` |
| f | `submit_agent_for_review` → `publish_agent` | 平台只有数字人版本需要评审发布；无小队发布操作 |
| g | `create_squad` → `add_agent_to_squad` | 复用小队只补缺失成员，不移除额外成员 |

## 5. 回读断言（必做）

写入可能静默失败，逐项断言：

- 每个 SDLC：`status=ENABLED`、步骤数与模板完全相等、按 stepOrder 逐个比对 `instructionMd` 全文、checklist/gatePolicy/onSuccess/onFail 解析后相等、handler 字段一致
- 每个数字人：`status=ONLINE`、在线版本 `sdlcId` 正确、soulMd/agentMd 与实例副本**全文**一致（防截断）
- 每个小队：`memberAgentIds` 与模板成员集合一致（注意：列表接口返回在 `items`，成员字段是 `memberAgentIds`）
- 任一断言失败 → 按失败项报告，不算成功

## 6. 仓库与权限

1. `create_repo`：name（单段路径）、url（Git 地址）、defaultBranch、description（技术栈/职责边界写清，数字人靠它判断去哪个仓库干活）
2. 多仓库时用 `create_repo_relation` 配置关系（如 DEPENDS_ON）
3. 每个数字人 `bind_agent_repos`（常规 WRITE）
4. **重新 `submit_agent_for_review` + `publish_agent`**——权限绑定写入的是新编辑版本，不重发布则在线版本不带权限（回读在线版本 `repoPerms` 确认）

### MCP 说明

无需为数字人手动配置 AutoWonder MCP：平台每次派发自动注入内置 MCP（名称 `autowonder`、dispatch 级鉴权、必选），显式同名绑定会被任务打包器跳过。帮助中心旧文档中的手动 MCP 配置步骤可跳过。

## 7. 执行器

1. 确认执行器机器的 CLI：`qoder` → clientKind `QODER_CLI`；`qodercn` → `QODER_CN_CLI`（`list_executor_client_kinds` 可查当前允许的类别）
2. `get_executor_launch_options(clientKind)` 取**当前**模型/推理档/上下文/记忆模式清单，做成卡片让用户选（参考档位：需求澄清用旗舰档，其余用 Flash 档；记忆模式常规选平台记忆）
3. `create_executor`：命名常规「NO1+数字人名」；只有绑 SDLC 的数字人 + 需求澄清需要执行器
4. `build_executor_launch_command` 生成启动命令（ADMIN 的 `create_executor` 响应本身已含明文 token 与全部启动参数，多执行器时命令之间只差 `--token`/`--executor-id`，可按一个 build 结果拼装其余；build 主要用于核对服务端 wsUrl 与 runtimeVersion）。**入口检查**：命令里的 `--ws-url` 来自服务端配置（常为公网域名）。本机已有连接正常的执行器时直接复用其入口；否则问用户执行器机器走公网还是内网；公网不通（报 `Failed to fetch executor environment` 或连接挂起）→ 向用户要内网地址并替换，scheme 同步降级（`wss://→ws://`、`https://→http://`）
5. 用 **awem**（AutoWonder Executor Manager）在执行器机器上纳管执行器，不再生成 bash 管理脚本：
   1. 安装：`curl -fsSL https://raw.githubusercontent.com/mantougg/awem/main/scripts/install.sh | sh`（或从 GitHub Releases 手动下载对应平台产物；Windows 用 `install.ps1`）
   2. `awem doctor`：环境体检（Node≥20 / npx / qoder CLI / 磁盘），任一 ❌ 先修再继续
   3. 每个执行器：`awem add "<第 4 步生成并替换好入口的完整启动命令>" --autostart`——名称缺省取 `exec-<executor-id>`；崩溃自动拉起、日志滚动、随 daemon 自启都内置
   4. （推荐 7×24）`awem service install`：daemon 注册为系统服务随系统常驻；已装过的机器跳过（先 `awem service status` 确认）
   5. `awem start <名称>` 启动
6. 启动后验证：`awem status <名称>` 本地进程 running；`awem log <名称>` 出现 `ws connected`；平台 `list_executors` 全部 ONLINE 且 `lastHeartbeat` 持续刷新（间隔约 2 分钟）

存量迁移：已用旧 `executor-manager.sh` 管理的机器，`awem import --file=<旧脚本路径> --with-logs --yes` 一键导入 EXECUTORS 数组（中文名自动改 `exec-<id>`，旧日志归档），验证全部 ONLINE 后停用旧脚本。

## 8. 交付输出

对接完成后向用户输出：

- 对象清单：小队/数字人/SDLC/执行器及 ID、仓库与权限
- mermaid 交付流程图（澄清→开发→评审/部署→人工验收，标注打回环路）
- 自动化链路 vs 人工决策点（澄清确认、最终验收、关键歧义上交）
- 遗留事项（如本机预存失败的测试会卡交付门禁，建议先修）
