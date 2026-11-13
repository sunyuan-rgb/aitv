# Business smoke check (Windows): login page -> login -> catalog -> free-episode watch
# Usage: powershell -File deploy/smoke.ps1 -BaseUrl http://localhost:8080
param([string]$BaseUrl = "http://localhost:8080")
$ErrorActionPreference = "Stop"

Write-Host "[smoke] 1. login page reachable"
$r = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/login"
if ($r.StatusCode -ne 200) { Write-Host "[smoke] FAIL /login"; exit 1 }

Write-Host "[smoke] 2. u1 login"
$sess = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$r = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/login" -Method Post `
    -Body @{ username = "u1"; password = "pass123" } -WebSession $sess
if ($r.StatusCode -ne 200) { Write-Host "[smoke] FAIL login"; exit 1 }

Write-Host "[smoke] 3. published catalog"
$r = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/api/episodes" -WebSession $sess
if ($r.Content -notmatch "episodes") { Write-Host "[smoke] FAIL catalog"; exit 1 }

Write-Host "[smoke] 4. free-episode watch decision"
$r = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/api/watch/1" -WebSession $sess
if ($r.Content -notmatch '"allowed":true') { Write-Host "[smoke] FAIL free watch: $($r.Content)"; exit 1 }

Write-Host "[smoke] ALL PASS"
