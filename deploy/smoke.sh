#!/usr/bin/env bash
# 业务冒烟检查：自建数据（提交→审核→上架）→ 免费集观看 → 购买幂等 → 观看权限
# 兼容已有数据：内容准备失败不中断，购买幂等以"前后余额不变"判定
# 用法: bash deploy/smoke.sh http://localhost:8080
set -euo pipefail
BASE="${1:-http://localhost:8080}"
TMP=$(mktemp -d)
fail() { echo "[smoke] FAIL: $1"; exit 1; }

login() {
  local user=$1 jar=$2 code
  code=$(curl -s -c "$jar" -o /dev/null -w "%{http_code}" -d "username=$user&password=pass123" "$BASE/login")
  [ "$code" = "302" ] || fail "$user 登录返回 $code"
}

post_json() {
  local jar=$1 path=$2 data=$3
  curl -s -b "$jar" -H "Content-Type: application/json" -d "$data" "$BASE$path" || true
}

balance_of() { echo "$1" | grep -o '"balance":[0-9]*' | head -1 | cut -d: -f2; }

# 0. 健康检查
echo "[smoke] 1. 登录页可达"
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/login")
[ "$code" = "200" ] || fail "/login -> $code"

# 1. 准备已上架数据（幂等：已上架时重复提交/审核的失败可忽略）
echo "[smoke] 2. 内容提交 → 审核通过 → 运营上架"
login editor "$TMP/editor.jar"
post_json "$TMP/editor.jar" /api/content/submit '{"episode_id":1,"title":"EP1","play_url":"/static/demo/ep1.mp4","is_free":true}' > /dev/null
post_json "$TMP/editor.jar" /api/content/submit '{"episode_id":3,"title":"EP3","play_url":"/static/demo/ep3.mp4","is_free":false}' > /dev/null
login reviewer "$TMP/reviewer.jar"
post_json "$TMP/reviewer.jar" /api/review/decision '{"target":"episode","id":1,"approve":true}' > /dev/null
post_json "$TMP/reviewer.jar" /api/review/decision '{"target":"episode","id":3,"approve":true}' > /dev/null
login ops "$TMP/ops.jar"
post_json "$TMP/ops.jar" /api/ops/action '{"action":"publish","episode_id":1}' > /dev/null
post_json "$TMP/ops.jar" /api/ops/action '{"action":"publish","episode_id":3}' > /dev/null

# 2. 免费集观看
echo "[smoke] 3. 免费集观看判定"
login u1 "$TMP/u1.jar"
body=$(curl -s -b "$TMP/u1.jar" "$BASE/api/watch/1")
echo "$body" | grep -q '"allowed":true' || fail "免费集应可看: $body"

# 3. 购买幂等 + 扣币一次（比较前后余额；若内容被暂停/停售先恢复）
echo "[smoke] 4. 购买幂等（同一请求编号两次余额不变）"
post_json "$TMP/ops.jar" /api/ops/action '{"action":"resume","episode_id":3,"reason":"smoke check"}' > /dev/null
r1=$(post_json "$TMP/u1.jar" /api/buy '{"episode_id":3,"request_id":"smoke-buy-1"}')
r2=$(post_json "$TMP/u1.jar" /api/buy '{"episode_id":3,"request_id":"smoke-buy-1"}')
b1=$(balance_of "$r1")
b2=$(balance_of "$r2")
[ -n "$b1" ] && [ "$b1" = "$b2" ] || fail "重复购买余额变化: $b1 -> $b2; resp=$r1 / $r2"
echo "  重复购买后余额: $b2"

# 4. 付费集观看
echo "[smoke] 5. 付费集观看判定"
body=$(curl -s -b "$TMP/u1.jar" "$BASE/api/watch/3")
echo "$body" | grep -q '"allowed":true' || fail "已购付费集应可看: $body"

# 5. 未购用户仍不可看
echo "[smoke] 6. 未购用户观看被拒"
login u2 "$TMP/u2.jar"
body=$(curl -s -b "$TMP/u2.jar" "$BASE/api/watch/3")
echo "$body" | grep -q '"allowed":false' || fail "未购用户不应可看: $body"

echo "[smoke] ALL PASS"
rm -rf "$TMP"
