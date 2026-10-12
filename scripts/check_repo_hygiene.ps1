# check_repo_hygiene.ps1 — Windows / PowerShell mirror of
# scripts/check_repo_hygiene.sh. Fails the build if stray build
# artefacts, browser downloads, or Python venv leaks are present
# in the working tree. Wired into the local pre-commit flow on
# Windows; the bash version is the source of truth for the GHA
# harbor-image.yml pipeline.
#
# Origin: BUG-2026-10-12-001..005. Keep the two scripts in sync
# when adding new hygiene cases.

$ErrorActionPreference = 'Stop'
$fail = 0

function Note($s) { Write-Host "  - $s" }
function Ok($s)   { Write-Host "v $s" -ForegroundColor Green }
function Err($s)  { Write-Host "x $s" -ForegroundColor Red; $script:fail = 1 }

Set-Location (git rev-parse --show-toplevel)

# 1. *.exe at the repo root
Write-Host "Checking for .exe binaries at repo root..."
$exes = Get-ChildItem -Path . -Filter '*.exe' -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Name
if ($exes) {
  Err "found .exe binaries at repo root (should be in .gitignore or bin/):"
  $exes | ForEach-Object { Note $_ }
} else { Ok "no .exe binaries at repo root" }

# 2. *.crdownload
Write-Host "Checking for .crdownload files..."
$cr = Get-ChildItem -Path . -Recurse -Filter '*.crdownload' -ErrorAction SilentlyContinue | Where-Object { $_.FullName -notmatch '\\.git\\' } | Select-Object -ExpandProperty FullName
if ($cr) {
  Err "found .crdownload artefacts (browser leak):"
  $cr | ForEach-Object { Note $_ }
} else { Ok "no .crdownload artefacts" }

# 3. pyvenv.cfg at the repo root
Write-Host "Checking for repo-root pyvenv.cfg..."
if (Test-Path pyvenv.cfg) {
  Err "pyvenv.cfg found at repo root - bind the venv to a per-project dir (e.g. sync-service/.venv)"
} else { Ok "no repo-root pyvenv.cfg" }

# 4. ,env typo
Write-Host "Checking for the typo'd ',env' file..."
if (Test-Path ',env') {
  Err "found ',env' (typo'd .env) at repo root - rotate any secrets it contains then delete"
} else { Ok "no ',env' file" }

# 5. supervisord.conf
Write-Host "Checking for stray supervisord.conf..."
$rootHasSup = Test-Path supervisord.conf
$deployHasSup = Test-Path deploy/docker/supervisord.conf
if ($rootHasSup -and -not $deployHasSup) {
  Err "supervisord.conf is at repo root - move it to deploy/docker/"
} elseif (-not $rootHasSup -and -not $deployHasSup) {
  Err "supervisord.conf is missing entirely (m9m-backend container cannot start without it)"
} else { Ok "supervisord.conf is at deploy/docker/supervisord.conf" }

if ($fail -ne 0) {
  Write-Host ""
  Write-Host "Hygiene check FAILED - see BUG-2026-10-12-001..005." -ForegroundColor Red
  exit 1
}
Write-Host ""
Write-Host "Hygiene check OK." -ForegroundColor Green
