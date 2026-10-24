#!/usr/bin/env bash
# smoke.sh：awem 全链路冒烟（T10-F6）。一键全绿（set -e 失败即停并报告步骤）。
# 层次：
#   A. Go 冒烟：go build/vet/test 三绿（supervisor 假子进程冒烟在内）+
#      三平台交叉编译 + 前端构建 + UI 规范自检；
#   B. 服务链路（仅 Linux 真机）：service install → daemon 由 systemd 拉起 →
#      CLI/API 可查 → Web 壳 401/200 → uninstall 无残留；
#   C. 执行器守护语义由 go test 覆盖（假子进程，不依赖平台/网络）。
# 用法：scripts/smoke.sh [--skip-service]（无 systemd 的环境跳过 B 层）
set -euo pipefail
cd "$(dirname "$0")/.."
export PATH="$HOME/.local/share/mise/installs/go/1.26.4/bin:$PATH"

step() { echo; echo "==> $1"; }

step "A1 go build/vet/test 三绿"
go build ./...
go vet ./...
go test ./... -count=1

step "A2 三平台交叉编译"
GOOS=windows go build ./...
GOOS=darwin go build ./...

step "A3 前端构建与 UI 自检"
npm --prefix web run build
node web/scripts/verify-ui.mjs

if [ "${1:-}" = "--skip-service" ]; then
  echo; echo "✅ 冒烟通过（--skip-service：跳过服务层）"
  exit 0
fi

if [ "$(uname -s)" != "Linux" ]; then
  echo "服务层冒烟仅支持 Linux 真机（当前 $(uname -s)），跳过"
  exit 0
fi

BIN="$HOME/.local/bin/awem"
mkdir -p "$HOME/.local/bin"
go build -o "$BIN" ./cmd/awem
UNIT="$HOME/.config/systemd/user/awem.service"

step "B1 清场（可能的旧安装）"
"$BIN" service uninstall >/dev/null 2>&1 || true
rm -f "$HOME/.config/awem/data/daemon.json"

step "B2 service install → systemd 拉起 daemon"
"$BIN" service install >/dev/null
for i in $(seq 1 20); do
  [ "$(systemctl --user is-active awem)" = "active" ] && break
  sleep 0.5
done
[ "$(systemctl --user is-active awem)" = "active" ] || { echo "❌ 服务未 active"; exit 1; }
[ -f "$UNIT" ] || { echo "❌ unit 文件缺失"; exit 1; }

step "B3 daemon.json 报活 + CLI 查询"
for i in $(seq 1 20); do
  [ -f "$HOME/.config/awem/data/daemon.json" ] && break
  sleep 0.5
done
[ -f "$HOME/.config/awem/data/daemon.json" ] || { echo "❌ daemon.json 未生成"; exit 1; }
"$BIN" status --json | grep -q '"executors"' || { echo "❌ CLI 查询失败"; exit 1; }

step "B4 Web 壳：/ 200 + API 无 token 401"
PORT=$(python3 -c "import json;print(json.load(open('$HOME/.config/awem/data/daemon.json'))['port'])")
TOKEN=$(cat "$HOME/.config/awem/data/api-token")
curl -sf -o /dev/null "http://127.0.0.1:$PORT/" || { echo "❌ SPA 首页不可达"; exit 1; }
CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/api/status")
[ "$CODE" = "401" ] || { echo "❌ API 无 token 应 401（实际 $CODE）"; exit 1; }
CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/status")
[ "$CODE" = "200" ] || { echo "❌ API 对 token 应 200（实际 $CODE）"; exit 1; }

step "B5 uninstall 无残留"
"$BIN" service uninstall >/dev/null
sleep 1.5
[ ! -f "$UNIT" ] || { echo "❌ unit 未删除"; exit 1; }
[ "$(systemctl --user is-active awem 2>/dev/null || true)" != "active" ] || { echo "❌ 服务仍 active"; exit 1; }
if pgrep -x awem >/dev/null 2>&1; then
  echo "❌ 仍有 awem 进程残留"; pgrep -ax awem; exit 1
fi
[ ! -f "$HOME/.config/awem/data/daemon.json" ] || { echo "❌ daemon.json 未清理"; exit 1; }

echo
echo "✅ 冒烟全部通过（$(date '+%H:%M:%S')）"
