[CmdletBinding()]
param(
    [switch]$Purge,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$taskName = 'Arknights Endfield SKPORT Daily Claim'
$legacyTaskNames = @('AKEF SKPort Daily Claim')
$appDirectory = 'akef-skport-claim'

if ($Help) {
    Write-Output 'Usage: powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\uninstall.ps1 [-Purge]'
    exit 0
}

try {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        throw 'LOCALAPPDATA is not defined'
    }

    foreach ($scheduledTaskName in @($taskName) + $legacyTaskNames) {
        $existingTask = Get-ScheduledTask -TaskName $scheduledTaskName -ErrorAction SilentlyContinue
        if ($null -ne $existingTask) {
            try {
                Stop-ScheduledTask -TaskName $scheduledTaskName -ErrorAction SilentlyContinue
                Unregister-ScheduledTask -TaskName $scheduledTaskName -Confirm:$false
            }
            catch {
                throw "The scheduled task cannot be removed by the current user. Remove '$scheduledTaskName' from an elevated Task Scheduler or PowerShell session. $($_.Exception.Message)"
            }
            Write-Output "Removed Task Scheduler task: $scheduledTaskName"
        }
        else {
            Write-Output "Task Scheduler task already absent: $scheduledTaskName"
        }
    }

    $installDirectory = Join-Path $env:LOCALAPPDATA "Programs\$appDirectory\bin"
    $installedBinary = Join-Path $installDirectory 'akef-claim.exe'
    $installedLauncher = Join-Path $installDirectory 'akef-claim-scheduled.vbs'
    if (Test-Path -LiteralPath $installedLauncher -PathType Leaf) {
        Remove-Item -LiteralPath $installedLauncher -Force
        Write-Output "Removed scheduler launcher: $installedLauncher"
    }
    else {
        Write-Output "Scheduler launcher already absent: $installedLauncher"
    }

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
