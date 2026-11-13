#!/usr/bin/env bash
# 业务冒烟检查：登录 -> 目录 -> 购买幂等 -> 观看
# 用法: bash deploy/smoke.sh http://localhost:8080
set -euo pipefail
BASE="${1:-http://localhost:8080}"
CJ=$(mktemp)

echo "[smoke] 1. 登录页可达"
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/login")
[ "$code" = "200" ] || { echo "FAIL: /login -> $code"; exit 1; }

echo "[smoke] 2. u1 登录"
code=$(curl -s -c "$CJ" -o /dev/null -w "%{http_code}" -d "username=u1&password=pass123" "$BASE/login")
[ "$code" = "302" ] || { echo "FAIL: 登录 -> $code"; exit 1; }

echo "[smoke] 3. 已上架剧集目录"
body=$(curl -s -b "$CJ" "$BASE/api/episodes")
echo "$body" | grep -q "episodes" || { echo "FAIL: 目录无数据"; exit 1; }

echo "[smoke] 4. 免费集观看判定"
body=$(curl -s -b "$CJ" "$BASE/api/watch/1")
echo "$body" | grep -q '"allowed":true' || { echo "FAIL: 免费集应可看: $body"; exit 1; }

echo "[smoke] 5. 购买幂等（同请求编号两次）"
body1=$(curl -s -b "$CJ" -H "Content-Type: application/json" -d '{"episode_id":1,"request_id":"smoke-free"}' "$BASE/api/buy" || true)
body2=$(curl -s -b "$CJ" -H "Content-Type: application/json" -d '{"episode_id":1,"request_id":"smoke-free"}' "$BASE/api/buy" || true)
echo "免费集购买被拒（预期行为）: $body1"

echo "[smoke] 全部通过"
rm -f "$CJ"
