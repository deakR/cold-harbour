<#
.SYNOPSIS
    ColdHarbor dispatch benchmark and chaos probe.
.DESCRIPTION
    Dispatches N jobs of mixed task types to the control plane REST API and
    reports dispatch latency (p50/p95/max). When a Go worker is live, it also
    polls Dead Drop availability to report completion rate. Uses no Python.
.PARAMETER Count
    Number of jobs to dispatch (default: 50).
.PARAMETER ApiKey
    Optional X-API-Key value when the control plane requires key auth.
#>
[CmdletBinding()]
param(
    [string]$TargetHost = "localhost",
    [int]$TargetPort = 8080,
    [int]$Count = 50,
    [string]$ApiKey = ""
)

$ErrorActionPreference = "Stop"
$taskTypes = @("DATA_REDUCTION", "CIPHER_STREAM", "ARCHIVE_SEAL")
$latencies = New-Object System.Collections.Generic.List[double]
$ids = New-Object System.Collections.Generic.List[string]
$failures = 0

$headers = @{ "Content-Type" = "application/json" }
if ($ApiKey -ne "") { $headers["X-API-Key"] = $ApiKey }
else { $headers["X-Context-Clearance"] = "SYSTEM" }

Write-Host "Dispatching $Count jobs to http://${TargetHost}:${TargetPort}/api/v1/compartments ..."
for ($i = 0; $i -lt $Count; $i++) {
    $task = $taskTypes[$i % $taskTypes.Count]
    $body = @{
        context = "INNIE"
        ownerId = "usr_bench"
        taskType = $task
        payload = @{ batchSize = 200; inputValues = @(1, 2, 3, 4) }
    } | ConvertTo-Json -Compress -Depth 5
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $resp = Invoke-RestMethod -Uri "http://${TargetHost}:${TargetPort}/api/v1/compartments" `
            -Method Post -Headers $headers -Body $body -TimeoutSec 10
        $sw.Stop()
        $latencies.Add($sw.Elapsed.TotalMilliseconds)
        $ids.Add($resp.compartmentId)
    } catch {
        $sw.Stop()
        $failures++
        Write-Warning "Dispatch $i failed: $_"
    }
}

if ($latencies.Count -eq 0) {
    Write-Error "No jobs dispatched successfully. Is the control plane running?"
    exit 1
}

$sorted = $latencies | Sort-Object
$p50 = $sorted[[int](0.50 * ($sorted.Count - 1))]
$p95 = $sorted[[int](0.95 * ($sorted.Count - 1))]
$max = $sorted[-1]
$avg = ($latencies | Measure-Object -Average).Average

Write-Host "Dispatched : $($latencies.Count)/$Count (failures: $failures)"
Write-Host ("Latency ms : avg={0:N1} p50={1:N1} p95={2:N1} max={3:N1}" -f $avg, $p50, $p95, $max)

# Completion probe: poll first 10 Dead Drops for up to 60s (requires live worker)
$probeIds = $ids | Select-Object -First 10
$deadline = (Get-Date).AddSeconds(60)
$completed = 0
foreach ($id in $probeIds) {
    while ((Get-Date) -lt $deadline) {
        try {
            $dd = Invoke-RestMethod -Uri "http://${TargetHost}:${TargetPort}/api/v1/compartments/$id/deaddrop" `
                -Method Get -Headers $headers -TimeoutSec 5
            if ($dd.checksum) { $completed++; break }
        } catch { Start-Sleep -Milliseconds 1000 }
    }
}
Write-Host "Completion : $completed/$($probeIds.Count) dead drops sealed within 60s (requires live Go worker)"
if ($completed -lt $probeIds.Count) {
    Write-Host "Note: incomplete seals mean the worker was offline or slow, not a dispatch failure."
}

$result = [PSCustomObject]@{
    dispatched = $latencies.Count
    failures = $failures
    latencyAvgMs = [math]::Round($avg, 1)
    latencyP50Ms = [math]::Round($p50, 1)
    latencyP95Ms = [math]::Round($p95, 1)
    latencyMaxMs = [math]::Round($max, 1)
    completed = $completed
    probed = $probeIds.Count
}
$result | ConvertTo-Json -Compress | Out-File -FilePath (Join-Path $PSScriptRoot "benchmark_last.json") -Encoding utf8
