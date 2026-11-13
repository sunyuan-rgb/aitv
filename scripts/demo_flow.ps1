# Title04 demo flow: full business loop via API/pages (publish -> buy -> stop-sales -> pause -> refund)
# Usage: powershell -File scripts/demo_flow.ps1 -BaseUrl http://localhost:8090
param([string]$BaseUrl = "http://localhost:8090")

$ErrorActionPreference = "Continue"

function Login($user) {
    $sess = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $body = @{ username = $user; password = "pass123" }
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/login" -Method Post -Body $body -WebSession $sess -MaximumRedirection 5 | Out-Null
    return $sess
}
function ApiPost($sess, $path, $obj) {
    try {
        $json = $obj | ConvertTo-Json
        $r = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl$path" -Method Post -Body $json -ContentType "application/json" -WebSession $sess
        return @{ code = [int]$r.StatusCode; body = $r.Content }
    } catch {
        $resp = $_.Exception.Response
        $code = if ($resp) { [int]$resp.StatusCode } else { 0 }
        $msg = ""
        try { $reader = New-Object IO.StreamReader($resp.GetResponseStream()); $msg = $reader.ReadToEnd() } catch {}
        return @{ code = $code; body = $msg }
    }
}
function ApiGet($sess, $path) {
    try {
        $r = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl$path" -WebSession $sess
        return @{ code = [int]$r.StatusCode; body = $r.Content }
    } catch {
        $resp = $_.Exception.Response
        $code = if ($resp) { [int]$resp.StatusCode } else { 0 }
        return @{ code = $code; body = "" }
    }
}
function FormPost($sess, $path, $form) {
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl$path" -Method Post -Body $form -WebSession $sess -MaximumRedirection 5 | Out-Null
}

Write-Host "== 1. publish and consume =="
$editor = Login "editor"
foreach ($ep in 1..6) {
    $form = @{ episode_id = "$ep"; title = "Drama EP$ep"; play_url = "/static/demo/ep$ep.mp4" }
    if ($ep -le 2) { $form["is_free"] = "1" }
    FormPost $editor "/editor/submit" $form
}
$reviewer = Login "reviewer"
foreach ($ep in 1..6) {
    FormPost $reviewer "/reviewer/decision" @{ target = "episode"; id = "$ep"; decision = "approve" }
}
$ops = Login "ops"
foreach ($ep in 1..6) {
    FormPost $ops "/ops/publish" @{ episode_id = "$ep" }
}
$u1 = Login "u1"
$r = ApiPost $u1 "/api/buy" @{ episode_id = 3; request_id = "demo-buy-ep3" }
Write-Host "u1 buy ep3: $($r.code) $($r.body)"
$r = ApiGet $u1 "/api/watch/3"
Write-Host "u1 watch ep3 (paid, allowed): $($r.body)"
$r = ApiGet $u1 "/api/watch/1"
Write-Host "u1 watch ep1 (free): $($r.body)"

Write-Host "== 2. stop-sales branch =="
$r = ApiPost $u1 "/api/buy" @{ episode_id = 5; request_id = "demo-buy-ep5" }
Write-Host "u1 buy ep5 before stop-sales: $($r.code)"
$null = ApiPost $ops "/api/ops/action" @{ action = "stop_sales"; episode_id = 5; reason = "schedule adjusted, stop selling" }
$u2 = Login "u2"
$r = ApiPost $u2 "/api/buy" @{ episode_id = 5 }
Write-Host "u2 buy stopped ep5 (should FAIL): $($r.code) $($r.body)"
$u1b = Login "u1"
$watch = ApiGet $u1b "/api/watch/5"
Write-Host "u1 watch stopped ep5 (already bought, allowed): $($watch.body)"
$null = ApiPost $ops "/api/ops/action" @{ action = "resume"; episode_id = 5; reason = "resume selling" }

Write-Host "== 3. pause and refund branch =="
$null = ApiPost $ops "/api/ops/action" @{ action = "pause"; episode_id = 3; reason = "content data error, pause watching" }
$watch = ApiGet $u1b "/api/watch/3"
Write-Host "u1 watch paused ep3 (should DENY): $($watch.body)"
$support = Login "support"
FormPost $u1b "/refund" @{ episode_id = "3"; reason = "paid but cannot watch, request refund" }
FormPost $support "/support/refund/decide" @{ refund_id = "1"; decision = "approve" }
$me = ApiGet $u1b "/api/me"
Write-Host "u1 records after refund: $($me.body.Substring(0, [Math]::Min(200, $me.body.Length)))"

Write-Host "== 4. refunded vs non-refunded user =="
$watch = ApiGet $u1b "/api/watch/3"
Write-Host "u1 (refunded) watch paused ep3: $($watch.body)"
$null = ApiPost $ops "/api/ops/action" @{ action = "pause"; episode_id = 4; reason = "data under review" }
$r = ApiPost $u2 "/api/buy" @{ episode_id = 4; request_id = "demo-buy-ep4-u2" }
Write-Host "u2 buy paused ep4 (should FAIL, no purchase): $($r.code)"
# u2 cannot buy while paused -> use u1 (u1 balance 400) to buy ep4 before pause? ep4 already paused.
# Use a fresh user purchase: pause ep6 instead with u2 buying first.
$null = ApiPost $ops "/api/ops/action" @{ action = "resume"; episode_id = 4; reason = "re-open for demo" }
$r = ApiPost $u2 "/api/buy" @{ episode_id = 4; request_id = "demo-buy-ep4-u2" }
Write-Host "u2 buy ep4: $($r.code)"
$null = ApiPost $ops "/api/ops/action" @{ action = "pause"; episode_id = 4; reason = "data under review" }
FormPost $u2 "/refund" @{ episode_id = "4"; reason = "same request refund" }
FormPost $support "/support/refund/decide" @{ refund_id = "2"; decision = "reject"; reason = "evidence not enough" }
$null = ApiPost $ops "/api/ops/action" @{ action = "resume"; episode_id = 4; reason = "review done" }
$watch = ApiGet $u2 "/api/watch/4"
Write-Host "u2 (refund rejected) watch resumed ep4 (allowed): $($watch.body)"

Write-Host "== 5. idempotency =="
$u2b = Login "u2"
$r1 = ApiPost $u2b "/api/buy" @{ episode_id = 5; request_id = "dup-req" }
$r2 = ApiPost $u2b "/api/buy" @{ episode_id = 5; request_id = "dup-req" }
Write-Host "same request_id twice: $($r1.code) / $($r2.code)"
Write-Host "demo flow done"
