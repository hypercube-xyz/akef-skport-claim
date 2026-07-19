[CmdletBinding()]
param(
    [switch]$Purge,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$taskName = 'AKEF SKPort Daily Claim'
$appDirectory = 'akef-skport-claim'

if ($Help) {
    Write-Output 'Usage: powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\uninstall.ps1 [-Purge]'
    exit 0
}

try {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        throw 'LOCALAPPDATA is not defined'
    }

    $existingTask = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($null -ne $existingTask) {
        try {
            Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
            Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
        }
        catch {
            throw "The scheduled task cannot be removed by the current user. Remove '$taskName' from an elevated Task Scheduler or PowerShell session. $($_.Exception.Message)"
        }
        Write-Output "Removed Task Scheduler task: $taskName"
    }
    else {
        Write-Output "Task Scheduler task already absent: $taskName"
    }

    $installDirectory = Join-Path $env:LOCALAPPDATA "Programs\$appDirectory\bin"
    $installedBinary = Join-Path $installDirectory 'akef-claim.exe'
    if (Test-Path -LiteralPath $installedBinary -PathType Leaf) {
        Remove-Item -LiteralPath $installedBinary -Force
        Write-Output "Removed binary: $installedBinary"
    }
    else {
        Write-Output "Binary already absent: $installedBinary"
    }

    if ($Purge) {
        $configPath = Join-Path $env:LOCALAPPDATA "$appDirectory\config.toml"
        $cacheDirectory = Join-Path $env:LOCALAPPDATA $appDirectory
        Remove-Item -LiteralPath $cacheDirectory -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $configPath -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath (Split-Path -Parent $configPath) -Force -ErrorAction SilentlyContinue

        if (-not [string]::IsNullOrWhiteSpace($env:APPDATA)) {
            $legacyConfig = Join-Path $env:APPDATA "$appDirectory\config.toml"
            Remove-Item -LiteralPath $legacyConfig -Force -ErrorAction SilentlyContinue
            Remove-Item -LiteralPath (Split-Path -Parent $legacyConfig) -Force -ErrorAction SilentlyContinue
        }
        Write-Output 'Removed configuration, scheduled logs, lock, and notification state.'
    }
    else {
        Write-Output 'Retained configuration, scheduled logs, lock, and notification state.'
    }
}
catch {
    Write-Error $_.Exception.Message
    exit 1
}
