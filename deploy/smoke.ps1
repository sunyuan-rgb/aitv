# Business smoke check (Windows): bootstrap data (submit -> review -> publish) -> free watch -> idempotent buy -> paid watch
# Tolerates pre-existing data: bootstrap calls are best-effort, balance is compared before/after duplicate buy.
# Usage: powershell -File deploy/smoke.ps1 -BaseUrl http://localhost:8080
param([string]$BaseUrl = "http://localhost:8080")
$ErrorActionPreference = "Stop"

function Fail($msg) { Write-Host "[smoke] FAIL: $msg"; exit 1 }
function Login($user) {
    $sess = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $r = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/login" -Method Post `
        -Body @{ username = $user; password = "pass123" } -WebSession $sess
    if ($r.StatusCode -ne 200) { Fail "$user login $($r.StatusCode)" }
    return $sess
}
function TryPostJson($sess, $path, $obj) {
    try {
        $r = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl$path" -Method Post `
            -Body ($obj | ConvertTo-Json -Compress) -ContentType "application/json" -WebSession $sess
        return $r.Content
    } catch { return "" }
}
function GetApi($sess, $path) {
    return (Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl$path" -WebSession $sess).Content
}
function BalanceOf($body) { if ($body -match '"balance":(\d+)') { return [int64]$Matches[1] } return -1 }

Write-Host "[smoke] 1. login page reachable"
$r = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/login"
if ($r.StatusCode -ne 200) { Fail "/login -> $($r.StatusCode)" }

Write-Host "[smoke] 2. bootstrap: submit -> review -> publish (best effort)"
$editor = Login "editor"
$null = TryPostJson $editor "/api/content/submit" @{ episode_id = 1; title = "EP1"; play_url = "/static/demo/ep1.mp4"; is_free = $true }
$null = TryPostJson $editor "/api/content/submit" @{ episode_id = 3; title = "EP3"; play_url = "/static/demo/ep3.mp4"; is_free = $false }
$reviewer = Login "reviewer"
$null = TryPostJson $reviewer "/api/review/decision" @{ target = "episode"; id = 1; approve = $true }
$null = TryPostJson $reviewer "/api/review/decision" @{ target = "episode"; id = 3; approve = $true }
$ops = Login "ops"
$null = TryPostJson $ops "/api/ops/action" @{ action = "publish"; episode_id = 1 }
$null = TryPostJson $ops "/api/ops/action" @{ action = "publish"; episode_id = 3 }

Write-Host "[smoke] 3. free-episode watch decision"
$u1 = Login "u1"
$body = GetApi $u1 "/api/watch/1"
if ($body -notmatch '"allowed":true') { Fail "free watch: $body" }

Write-Host "[smoke] 4. purchase idempotency (balance must not change on retry)"
# 确保内容可售（若处于暂停/停售则恢复）
$null = TryPostJson $ops "/api/ops/action" @{ action = "resume"; episode_id = 3; reason = "smoke check" }
$r1 = TryPostJson $u1 "/api/buy" @{ episode_id = 3; request_id = "smoke-buy-1" }
$r2 = TryPostJson $u1 "/api/buy" @{ episode_id = 3; request_id = "smoke-buy-1" }
$b1 = BalanceOf $r1
$b2 = BalanceOf $r2
if ($b1 -lt 0 -or $b1 -ne $b2) { Fail "duplicate buy changed balance: $b1 -> $b2; resp=$r1 / $r2" }
Write-Host "  balance after duplicate purchase: $b2"

Write-Host "[smoke] 5. paid-episode watch decision"
$body = GetApi $u1 "/api/watch/3"
if ($body -notmatch '"allowed":true') { Fail "paid watch: $body" }

Write-Host "[smoke] 6. non-buyer denied"
$u2 = Login "u2"
$body = GetApi $u2 "/api/watch/3"
if ($body -notmatch '"allowed":false') { Fail "non-buyer should be denied: $body" }

Write-Host "[smoke] ALL PASS"
