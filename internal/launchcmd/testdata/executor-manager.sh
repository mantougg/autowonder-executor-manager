#!/usr/bin/env bash
# autowonder-executor — 执行器管理脚本模板
#
# 由 AutoWonder 项目接入流程生成。使用前：
# 1. 把下方 EXECUTORS 数组替换为 build_executor_launch_command 生成的实际命令
#    （每个执行器一行，格式：名称|启动命令）。
# 2. 检查 --ws-url：启动命令默认是服务端配置的入口（常为公网域名）。
#    执行器机器走内网时，向管理员要内网地址替换，并把 wss:// 改为 ws://。
# 3. 前置：本机已装 qoder / qodercn CLI 并完成登录；Node.js ≥ 16（npx）。
#
# 用法：autowonder-executor {start|stop|status|log [名称]}
set -u

RUN_DIR="$HOME/.autowonder-executor"
mkdir -p "$RUN_DIR"

# 名称|启动命令（示例条目，请整体替换）
EXECUTORS=(
  "NO1示例数字人|npx -y autowonder@<runtimeVersion> connect --ws-url ws://<内网地址>/ws/executor --token <executor-token> --executor-id <id> --provider qoder --memory-mode platform --max-tasks 5 --model <model> --reasoning-effort medium --context-window 1000000 --token-aware-enable"
)

pidfile() { echo "$RUN_DIR/${1}.pid"; }
logfile() { echo "$RUN_DIR/${1}.log"; }

is_alive() {
  local pf; pf="$(pidfile "$1")"
  [ -f "$pf" ] && kill -0 "$(cat "$pf")" 2>/dev/null
}

start_one() {
  local name="$1" cmd="$2"
  if is_alive "$name"; then
    echo "[skip] $name 已在运行 (pid $(cat "$(pidfile "$name")"))"
    return
  fi
  nohup bash -c "$cmd" >>"$(logfile "$name")" 2>&1 &
  echo $! >"$(pidfile "$name")"
  echo "[start] $name pid $(cat "$(pidfile "$name")")（日志: $(logfile "$name")）"
}

stop_one() {
  local name="$1" pf pid
  pf="$(pidfile "$name")"
  if ! is_alive "$name"; then
    echo "[skip] $name 未在运行"
    rm -f "$pf"
    return
  fi
  pid="$(cat "$pf")"
  # 终止整组进程（npx → node → CLI 子进程树）
  kill -- -"$pid" 2>/dev/null || kill "$pid" 2>/dev/null
  sleep 2
  kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null
  rm -f "$pf"
  echo "[stop] $name (pid $pid) 已停止"
}

status_one() {
  if is_alive "$1"; then
    echo "[online ] $1 (pid $(cat "$(pidfile "$1")"))"
  else
    echo "[offline] $1"
  fi
}

case "${1:-}" in
  start)
    for e in "${EXECUTORS[@]}"; do IFS='|' read -r n c <<<"$e"; start_one "$n" "$c"; done
    ;;
  stop)
    for e in "${EXECUTORS[@]}"; do IFS='|' read -r n _ <<<"$e"; stop_one "$n"; done
    ;;
  status)
    for e in "${EXECUTORS[@]}"; do IFS='|' read -r n _ <<<"$e"; status_one "$n"; done
    ;;
  log)
    tail -n 50 "$(logfile "${2:-${EXECUTORS[0]%%|*}}")"
    ;;
  *)
    echo "用法: $0 {start|stop|status|log [执行器名称]}"
    echo "执行器: $(printf '%s ' "${EXECUTORS[@]%%|*}")"
    exit 1
    ;;
esac
