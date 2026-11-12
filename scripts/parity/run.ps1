param(
    [string]$Suite = "",
    [string]$Case = "",
    [string]$N8NBase = "http://187.77.113.218:5678",
    [string]$M9MBase = "http://187.77.113.218:8080",
    [switch]$DriftCheck,
    [switch]$NoBodyCompare,
    [switch]$Offline
)

# Windows wrapper around scripts/parity/runner.py + scripts/parity/report.py.
# Exits non-zero when any parity case fails so it can be used as a CI gate.

$ErrorActionPreference = "Stop"

$runnerArgs = @("scripts/parity/runner.py", "--n8n-base", $N8NBase, "--m9m-base", $M9MBase)
if ($Suite) { $runnerArgs += @("--suite", $Suite) }
if ($Case) { $runnerArgs += @("--case", $Case) }
if ($NoBodyCompare) { $runnerArgs += "--no-body-compare" }
if ($Offline) { $runnerArgs += "--offline" }

Write-Host "== parity runner =="
& python @runnerArgs
$runnerExit = $LASTEXITCODE

Write-Host "== parity report =="
$reportArgs = @("scripts/parity/report.py")
if ($DriftCheck) { $reportArgs += "--drift-check" }
& python @reportArgs
$reportExit = $LASTEXITCODE

if ($runnerExit -ne 0) { exit $runnerExit }
exit $reportExit