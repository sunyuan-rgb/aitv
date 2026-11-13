#!/usr/bin/env bash
# 本地教学环境部署脚本：构建 -> 停旧 -> 起新 -> 健康检查 -> 失败回退
# 用法: bash deploy/deploy_local.sh [端口] [数据目录]
set -euo pipefail
PORT="${1:-8080}"
DATA_DIR="${2:-data}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "[deploy] 1. 构建制品"
go build -o deploy/aitv-server ./cmd/server

echo "[deploy] 2. 停止旧版本进程"
pkill -f "aitv-server -addr" 2>/dev/null || true
sleep 1

echo "[deploy] 3. 启动新版本（后台，日志 deploy/app.log）"
nohup ./deploy/aitv-server -addr ":$PORT" -db "$DATA_DIR/aitv.db" > deploy/app.log 2>&1 &
APP_PID=$!

echo "[deploy] 4. 健康检查"
for i in $(seq 1 30); do
  if curl -sf "http://localhost:$PORT/login" > /dev/null; then
    echo "[deploy] 健康检查通过 (pid=$APP_PID)"
    bash deploy/smoke.sh "http://localhost:$PORT"
    echo "[deploy] 部署成功：提交 $(git rev-parse --short HEAD)，端口 $PORT"
    exit 0
  fi
  sleep 1
done

echo "[deploy] 健康检查失败，回退：终止新进程"
kill "$APP_PID" 2>/dev/null || true
echo "[deploy] 回退完成（旧版本已停止；如需恢复旧制品，重新构建上一提交并执行本脚本）"
exit 1
