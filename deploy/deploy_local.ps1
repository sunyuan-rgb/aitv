# Local teaching-env deployment (Windows): build -> stop old -> start new -> health -> smoke -> rollback on failure
# Usage: powershell -File deploy/deploy_local.ps1 [-Port 8080] [-DataDir data]
param(
    [int]$Port = 8080,
    [string]$DataDir = "data"
)
$ErrorActionPreference = "Continue"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

Write-Host "[deploy] 1. build artifact (commit $(git rev-parse --short HEAD))"
go build -o deploy/aitv-server.exe ./cmd/server
if ($LASTEXITCODE -ne 0) { Write-Host "[deploy] BUILD FAILED"; exit 1 }

Write-Host "[deploy] 2. stop old process"
Get-Process aitv-server -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 1

Write-Host "[deploy] 3. start new version (log: deploy/app.log)"
$proc = Start-Process -FilePath "$root/deploy/aitv-server.exe" `
    -ArgumentList "-addr",":$Port","-db","$DataDir/aitv.db" `
    -WorkingDirectory $root -NoNewWindow -PassThru `
    -RedirectStandardOutput "$root/deploy/app.log" -RedirectStandardError "$root/deploy/err.log"

Write-Host "[deploy] 4. health check"
$healthy = $false
foreach ($i in 1..30) {
    try {
        $r = Invoke-WebRequest -UseBasicParsing -Uri "http://localhost:$Port/login" -TimeoutSec 2
        if ($r.StatusCode -eq 200) { $healthy = $true; break }
    } catch { Start-Sleep 1 }
}
if (-not $healthy) {
    Write-Host "[deploy] HEALTH CHECK FAILED, rollback: stop new process"
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    exit 1
}
Write-Host "[deploy] health check OK (pid=$($proc.Id))"

Write-Host "[deploy] 5. business smoke check"
$smoke = Join-Path $root "deploy/smoke.ps1"
& powershell -ExecutionPolicy Bypass -File $smoke -BaseUrl "http://localhost:$Port"
if ($LASTEXITCODE -ne 0) {
    Write-Host "[deploy] SMOKE FAILED, rollback: stop new process (data volume kept)"
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    exit 1
}
Write-Host "[deploy] DEPLOY OK: commit $(git rev-parse --short HEAD), port $Port"
