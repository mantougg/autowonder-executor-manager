package config

// defaultConfigTOML 是首次运行生成的默认配置模板（design.md §5.2 字段集，
// 中文注释头说明热加载语义）。
const defaultConfigTOML = `# awem 配置文件
# 本文件人可读人可改；daemon 会监视变更并自动热加载。
# 注意：写入过程中或内容非法时，旧配置继续生效，错误会记入事件日志。

[server]
bind = "127.0.0.1"
port  = 0                 # 0 = 自动挑选空闲端口并写回

# [platform]              # 平台状态联动（可选；不配则只显示本地状态）
# base_url       = "http://10.144.144.1:7001"
# mcp_token      = "<个人 MCP 令牌>"
# poll_interval  = "60s"

[log]
max_size_mb   = 50
retention_days = 7

# [[executor]]            # 执行器示例（整条启动命令为主入口，awem add 可自动生成）
# name      = "my-executor"
# command   = "npx -y autowonder@0.3.22 connect --ws-url ws://... --token ... --executor-id 10086 --provider qoder --model qfmodel --reasoning-effort medium --context-window 1000000 --token-aware-enable"
# autostart = true
# [executor.restart]
# enabled    = true
# backoff    = "exponential"   # 1s 起步 ×2 上限 5m；稳定运行 10m 后重置
`
