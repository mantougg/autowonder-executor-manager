# AutoWonder 日常使用手册（对接完成后）

前提：已完成对接（小队/数字人/SDLC/执行器就绪，执行器 ONLINE）。

## 1. 创建工单

- `create_workitem`：workType 三选一——REQ（需求）/ BUG（缺陷）/ TASK（任务）
- **先建工单、传附件，最后才指派**。要指派给数字人的工单，contentMd 要写全：背景/目标与非目标/范围/验收标准/约束与依赖/预期交付物。信息不足先和用户澄清补齐，不要拿一句话需求直接派发
- 附件（需求文档/设计稿等）走 CLI 上传，不走 MCP 传文件：
  1. `workitem_cli_upload_token`（id=工单号）换 30 分钟上传令牌
  2. `AUTOWONDER_UPLOAD_TOKEN=<token> npx -y autowonder@<runtimeVersion> workitem upload --server-url <服务地址> --workitem-id <id> --file <路径> --json`
  3. `list_workitem_documents` 确认已传
- 状态流转：工单状态由 SDLC 步骤/真人验收驱动，创建时不手工改状态

## 2. 需求澄清（第一个工单建议先走这步）

- 小队选**包含「需求澄清」数字人的小队**（标准自动化交付），数字人选**需求澄清**
- 澄清数字人会结合最新代码与提出人确认需求，最终产出 `spec.md` / `plan.md` 上传工单附件；方案有问题继续反馈，满意后再确认
- 澄清确认后回到工单，进入启动交付

## 3. 启动交付

| 场景 | 小队 | 数字人 |
|---|---|---|
| 轻量需求/小修复（快） | 快速交付 | 快速全栈开发（完成后自动转快速部署工程师） |
| 大需求（全门禁） | 标准自动化交付 | 全栈开发（→代码评审→测试与部署 自动流转） |

- `assign_workitem(assigneeType=AGENT, assigneeRef=<数字人ID>)`；**不要**手动指定 sdlcId（服务端自动按数字人默认 SDLC 绑定）
- 数字人将按 SDLC 自动推进：建 feature/fix 分支 → 实现 → 验证 → 合并到部署分支（对接时配置的目标分支）→ 构建测试验证 → 交人工验收

## 4. 跟进与监控

- 工单详情：`get_workitem`；执行轨迹：`get_dispatch_runtime_trace`（dispatchId 维度）；评论：`list_workitem_comments`
- 固定时间窗批量对账：`list_workitem_executions(createdFrom/createdTo)`，running 的 dispatch 需要结论时看其活动流 `get_dispatch_activities`
- 产物下载：`workitem_cli_download_token` → `npx -y autowonder@<runtimeVersion> workitem download --server-url <服务地址> --workitem-id <id> --file <名称或id> --output-dir <目录> --json`
- 交付恢复（卡死/失败重试）：`control_delivery`（cancel/close/reopen/retry）

## 5. 验收

- QA 部署验证通过后工单交真人待决策验收；验收通过由真人流转状态，数字人不擅自完成工单
- 验收不通过 → 工单评论写明问题 → 数字人沿原分支返工（只处理反馈及受影响范围）

## 6. 团队协作

`成员管理` 加入团队成员，权限三档：**只读**（仅查看）/ **读写**（建工单、澄清、启动交付）/ **管理员**（小队/数字人/SDLC/仓库/执行器等配置）。项目接入完成后，普通成员的日常就是：建工单 → 需求澄清 → 启动交付。

## 7. 定时任务（可选）

需要 7×24 周期任务（如每夜回归）时用 `create_scheduled_task`（CRON/ONCE + 时区 + 执行小队与起始数字人），文档同样走 CLI 上传（sourceType=SCHEDULED_TASK）。
