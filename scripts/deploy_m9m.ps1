# deploy_m9m.ps1 — commit, push and restart the m9m-backend container
# on the remote server 187.77.113.218 via SSH port 2212.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/deploy_m9m.ps1 `
#       -CommitMessage "fix: ..."
#
# Requires:
#   - git in PATH and a configured origin remote
#   - ssh in PATH with key auth configured (C:\Users\mahar\.ssh\id_rsa)
#   - the Harbor image for the pushed commit to be built already
#     (the harbor-image.yml workflow does this on every push to main)
#   - the compose project to live in /home/mhr/gabungan on the remote
#
# The remote operator account is `mhr` (passwordless sudo + docker group).
# Use `sudo -n` for the root-owned paths.
#
# The server pulls the image from Harbor — it does NOT build from source.
# The legacy /home/mhr/bin/build script is only a fallback and tags
# `m9m:latest`, which the compose stack no longer references.
#
# This script intentionally does NOT touch sync.py inside the container;
# sync-service/sync.py is mounted from /root/sync-service and is updated
# only when the image is rebuilt.

param(
    [string]$CommitMessage = "chore: deploy m9m parity orchestrator",
    [string]$ServerHost = "187.77.113.218",
    [int]$ServerPort = 2212,
    [string]$ServerUser = "mhr",
    [string]$ComposeDir = "/home/mhr/gabungan",
    [switch]$SkipPush,
    [switch]$SkipBuild,
    [switch]$VerifyOnly
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot

function Step($msg) {
    Write-Host "[deploy] $msg" -ForegroundColor Cyan
}

function Fail($msg, $code = 1) {
    Write-Host "[deploy][FAIL] $msg" -ForegroundColor Red
    exit $code
}

Step "repo root: $repoRoot"
Set-Location $repoRoot

# 1. Git status check
$status = git status --porcelain 2>&1
if ($LASTEXITCODE -ne 0) {
    Fail "git status failed"
}

# 2. Stage + commit (if there are local changes)
$hasChanges = ($status | Where-Object { $_ -match '^\?\? ' -or $_ -match '^( M|M |A |D ) ' }).Count -gt 0
if ($hasChanges -and -not $SkipPush) {
    Step "staging changes"
    git add -A
    if ($LASTEXITCODE -ne 0) { Fail "git add failed" }
    $tmpMsg = [System.IO.Path]::GetTempFileName()
    Set-Content -Path $tmpMsg -Value $CommitMessage -Encoding UTF8
    git commit -F $tmpMsg
    if ($LASTEXITCODE -ne 0) { Fail "git commit failed" }
    Remove-Item $tmpMsg -ErrorAction SilentlyContinue
} else {
    Step "no local changes to commit"
}

# 3. Push
if (-not $SkipPush) {
    Step "pushing to origin main"
    git push origin main
    if ($LASTEXITCODE -ne 0) { Fail "git push failed" }
}

if ($VerifyOnly) {
    Step "VerifyOnly mode — skipping remote deploy"
    exit 0
}

# 4. SSH + pull + restart
Step "ssh $ServerUser@$ServerHost`:$ServerPort — pulling and restarting"
# Build the remote script as an array so it is joined with LF only — a
# here-string inherits the file's CRLF endings and the remote bash then
# chokes on `set -e\r` / `cd /path\r`.
$remoteLines = @(
    "set -e"
    "cd $ComposeDir"
    "echo '[remote] pulling m9m image from Harbor'"
    $(if (-not $SkipBuild) { "docker compose pull m9m-backend" } else { "echo '[remote] skipping pull (SkipBuild)'" })
    "echo '[remote] restarting m9m-backend'"
    "docker compose up -d m9m-backend"
    "echo '[remote] done'"
)
$remoteCmd = ($remoteLines -join "`n")

ssh -p $ServerPort -o StrictHostKeyChecking=accept-new -o BatchMode=yes `
    -i "$HOME\.ssh\id_rsa" "$ServerUser@$ServerHost" $remoteCmd
if ($LASTEXITCODE -ne 0) { Fail "remote deploy failed" }

# 5. Verify healthz
Step "verifying healthz"
# `${ServerHost}` (not `$ServerHost`) — PowerShell reads `$ServerHost:8080`
# as a drive-qualified variable and silently expands it to empty.
$m9mHealth = curl.exe -sS -o NUL -w "%{http_code}" -m 10 "http://${ServerHost}:8080/healthz"
if ($m9mHealth -ne "200") {
    Fail "m9m /healthz returned $m9mHealth (expected 200)"
}
Step "deploy OK — m9m /healthz = $m9mHealth"
exit 0
