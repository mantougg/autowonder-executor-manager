# 排障手册

按现象查。所有「平台说 A、页面说 B」的问题先确认：**API 与浏览器访问的是同一个服务入口**。

## 执行器显示离线

按序排查：

1. **本地进程**：`awem status <名称>`（三层状态：本地进程 / 连接 / 平台在线）；`state=backoff` 说明在崩溃退避拉起中。进程死了看日志：`awem log <名称> --lines=200`；启停/崩溃/拉起时间线看管理事件日志 `<awem配置目录>/data/logs/events.log`
2. **心跳节奏**：平台 `get_executor` / `list_executors` 看 `lastHeartbeat`——间隔约 **2 分钟**，两次查询间隔不够长看到「没变」是正常的；连续 5 分钟不动才是真死。awem 侧还叠加自身轮询（`poll_interval`，默认 60s）：执行器刚启动 1 分钟内 `awem list` 的平台列为空属正常，别当失败
3. **本地日志关键字**：`ws connected` 有且无 `disconnect/reconnect/error` → 连接健康
4. **平台侧判定**：API 返回 `status=ONLINE` 且心跳在刷新 → 服务端是在线的。用户看到离线通常是：
   - **页面旧**：执行器上线前打开的页面没刷新 → 让用户强刷（Ctrl+Shift+R）
   - **入口不一致**：浏览器走公网域名、执行器连内网地址（或反之）→ 统一入口
5. 真死了：`awem restart <名称>`（awem 默认已带崩溃自动拉起，反复进入 backoff 说明启动本身有问题，看日志定位）；反复断连考虑网络中间件（NAT 空闲超时）或换入口

## 启动命令连不上（Failed to fetch executor environment / 连接挂起）

- 启动命令里的 `--ws-url` 来自服务端配置，常为公网域名；执行器机器在内网时公网入口可能半残（响应头 200 但正文挂起超时）
- 向用户要内网地址替换，scheme 同步降级：`wss://→ws://`、`https://→http://`。执行器已用 awem 纳管的：`awem edit <名称> --ws-url <内网地址>` 后 `awem restart <名称>`
- 验证方式：替换后 `awem log <名称>` 出现 `ws connected`，平台侧 ONLINE

## 宿主 MCP 读接口报 -32602

宿主客户端按 output schema 校验响应，**空列表**和步骤里的 `null` 字段会被判非法——这不是「对象不存在」。降级为原始 JSON-RPC 直连（`scripts/aw_rpc.py`）重读。

## 原始 JSON-RPC 报「MCP工具不存在」

工具名是**点号前缀** `autowonder.create_sdlc`；`autowonder_create_sdlc` 是宿主注册名。`tools/list` 可确认。

## 写接口返回成功但数据没落库

含中文的大请求体（约 8KB 起）经 shell 命令行内联（`curl --data '{...}'`）会被静默截断。请求体写临时文件 `--data @file`，或用 Python（`urllib` 直发，不经 shell）。**每次批量写入后必须回读断言**（onboarding.md §5）。

## 绑定仓库/技能后数字人行为没变

绑定写入的是数字人**编辑版本**。必须重新 `submit_agent_for_review` + `publish_agent`，在线版本才更新；回读在线版本 `repoPerms`/`skills` 确认。

## 模型参数被拒绝（invalid model）

模型目录会变，写死的模型 ID 会被显式拒绝而不是被替换。每次 `get_executor_launch_options` 现取。

## 交付不动了

1. 执行器 ONLINE？（dispatch 需要「该数字人」的执行器在线接单）
2. 数字人 `status=ONLINE`？（被下线的数字人不接派发）
3. SDLC `status=ENABLED`？
4. `get_dispatch_activities` 看最后事件；`get_dispatch_runtime_trace` 看执行轨迹
5. 卡在门禁（checklist 不过）→ 看工单评论与证据目录
6. 需要人工干预/重试 → `control_delivery`

## QA 步骤总在测试处失败

对接时写进「部署方式」变量的构建/测试命令如果在本机本来就有预存失败，会成为每个工单的门禁拦路虎。对接前先本机跑一遍这些命令，红的先修掉或与用户明确处理策略。

## 派发后数字人说连不上 AutoWonder MCP

正常不应发生（平台自动注入）。若发生：确认服务端版本不旧于自动注入能力的引入版本；老版本按帮助中心旧流程在个人中心配 MCP 并绑定数字人。

## 遇到旧 executor-manager.sh 管理的机器

老机器可能还在用 `executor-manager.sh`（`EXECUTORS` 数组 + nohup + pidfile，日志在 `~/.autowonder-executor/`）。不要再维护它：装 awem 后
`awem import --file=<旧脚本路径> --with-logs --yes` 导入全部执行器（中文名自动改 `exec-<executor-id>`），`awem list` 核对、`awem start` 验证 ONLINE 后停用旧脚本。
