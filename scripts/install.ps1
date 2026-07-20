[CmdletBinding()]
param(
    [string]$Time = '00:05',
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$taskName = 'Arknights: Endfield SKPORT Daily Claim'
$legacyTaskName = 'AKEF SKPort Daily Claim'
$appDirectory = 'akef-skport-claim'

function Invoke-AkefClaim {
    param(
        [Parameter(Mandatory = $true)][string]$Binary,
        [Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments
    )

    & $Binary @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "akef-claim exited with code $LASTEXITCODE"
    }
}

function Test-PathOnUserPath {
    param([Parameter(Mandatory = $true)][string]$Path)

    foreach ($entry in ($env:PATH -split ';')) {
        if ($entry.TrimEnd('\') -ieq $Path.TrimEnd('\')) {
            return $true
        }
    }
    return $false
}

function ConvertTo-VbScriptStringLiteral {
    param([Parameter(Mandatory = $true)][string]$Value)

    return '"' + $Value.Replace('"', '""') + '"'
}

function New-TaskLauncherContent {
    param(
        [Parameter(Mandatory = $true)][string]$Binary,
        [Parameter(Mandatory = $true)][string]$Config
    )

    $binaryLiteral = ConvertTo-VbScriptStringLiteral -Value $Binary
    $command = '"' + $Binary + '" --silent run --config "' + $Config + '"'
    $commandLiteral = ConvertTo-VbScriptStringLiteral -Value $command
    return @"
Option Explicit
Dim command, exitCode, fileSystem, shell

Set fileSystem = CreateObject("Scripting.FileSystemObject")
If Not fileSystem.FileExists($binaryLiteral) Then
    WScript.Quit 1
End If

command = $commandLiteral
Set shell = CreateObject("WScript.Shell")
exitCode = shell.Run(command, 0, True)

If exitCode = 30 Then
    WScript.Quit 1
End If
WScript.Quit 0
"@
}

function Install-ScheduledClaim {
    param(
        [Parameter(Mandatory = $true)][string]$Launcher,
        [Parameter(Mandatory = $true)][datetime]$At
    )

    foreach ($command in 'Get-ScheduledTask', 'Export-ScheduledTask', 'Register-ScheduledTask', 'Unregister-ScheduledTask') {
        if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
            throw "$command is unavailable; Task Scheduler PowerShell commands are required"
        }
    }

    $wscript = Join-Path $env:SystemRoot 'System32\wscript.exe'
    if (-not (Test-Path -LiteralPath $wscript -PathType Leaf)) {
        throw 'Windows Script Host was not found'
    }
    $action = New-ScheduledTaskAction `
        -Execute $wscript `
        -Argument "//B //NoLogo `"$Launcher`""
    $trigger = New-ScheduledTaskTrigger -Daily -At $At
    $settings = New-ScheduledTaskSettingsSet `
        -AllowStartIfOnBatteries `
        -DontStopIfGoingOnBatteries `
        -StartWhenAvailable `
        -Hidden `
        -ExecutionTimeLimit ([TimeSpan]::FromMinutes(35)) `
        -MultipleInstances IgnoreNew `
        -Priority 7 `
        -RestartCount 3 `
        -RestartInterval ([TimeSpan]::FromMinutes(30))
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    $principal = New-ScheduledTaskPrincipal -UserId $identity -LogonType Interactive -RunLevel Limited
    $task = New-ScheduledTask `
        -Action $action `
        -Trigger $trigger `
        -Settings $settings `
        -Principal $principal `
        -Description 'Run the local Arknights: Endfield SKPORT attendance claim once per day.'

    $existingTask = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    $existingXml = $null
    if ($null -ne $existingTask) {
        $existingXml = Export-ScheduledTask -TaskName $taskName
    }

    try {
        Register-ScheduledTask -TaskName $taskName -InputObject $task -Force | Out-Null
    }
    catch {
        $installError = $_
        if ($null -ne $existingXml) {
            try {
                Register-ScheduledTask -TaskName $taskName -Xml $existingXml -Force | Out-Null
            }
            catch {
                throw "Failed to install the new task and restore the previous task. Install error: $($installError.Exception.Message); restore error: $($_.Exception.Message)"
            }
        }
        throw "Failed to install the scheduled task: $($installError.Exception.Message)"
    }

    $legacyTask = Get-ScheduledTask -TaskName $legacyTaskName -ErrorAction SilentlyContinue
    if ($null -ne $legacyTask) {
        try {
            Stop-ScheduledTask -TaskName $legacyTaskName -ErrorAction SilentlyContinue
            Unregister-ScheduledTask -TaskName $legacyTaskName -Confirm:$false
        }
        catch {
            throw "Installed '$taskName', but could not remove the legacy task '$legacyTaskName'. Remove the legacy task before it can run again. $($_.Exception.Message)"
        }
    }

    Get-ScheduledTask -TaskName $taskName |
        Select-Object TaskName, State, @{Name = 'DailyTime'; Expression = { $Time }} |
        Format-List
}

if ($Help) {
    Write-Output 'Usage: powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\install.ps1 [-Time HH:mm]'
    exit 0
}

$parsedTime = [datetime]::MinValue
if (-not [datetime]::TryParseExact($Time, 'HH:mm', [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::None, [ref]$parsedTime)) {
    [Console]::Error.WriteLine("Invalid schedule time '$Time'; expected HH:mm in 24-hour local time.")
    exit 2
}

try {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        throw 'LOCALAPPDATA is not defined'
    }

    $scriptDirectory = $PSScriptRoot
    $repositoryDirectory = Split-Path -Parent $scriptDirectory
    $installDirectory = Join-Path $env:LOCALAPPDATA "Programs\$appDirectory\bin"
    $installedBinary = Join-Path $installDirectory 'akef-claim.exe'
    New-Item -ItemType Directory -Path $installDirectory -Force | Out-Null

    $temporaryBinary = Join-Path $installDirectory ".akef-claim.exe.install.$([guid]::NewGuid().ToString('N'))"
    try {
        $goModule = Join-Path $repositoryDirectory 'go.mod'
        $go = Get-Command go -ErrorAction SilentlyContinue
        if ((Test-Path -LiteralPath $goModule -PathType Leaf) -and $null -ne $go) {
            Push-Location $repositoryDirectory
            try {
                & $go.Source build -trimpath -o $temporaryBinary ./cmd/akef-claim
                if ($LASTEXITCODE -ne 0) {
                    throw "go build exited with code $LASTEXITCODE"
                }
            }
            finally {
                Pop-Location
            }
        }
        else {
            $sourceBinary = @(
                (Join-Path $scriptDirectory 'akef-claim.exe'),
                (Join-Path $repositoryDirectory 'akef-claim.exe')
            ) | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } | Select-Object -First 1
            if ($null -eq $sourceBinary) {
                throw 'No release binary was found and Go is unavailable'
            }
            Copy-Item -LiteralPath $sourceBinary -Destination $temporaryBinary
        }
        Move-Item -LiteralPath $temporaryBinary -Destination $installedBinary -Force
    }
    finally {
        Remove-Item -LiteralPath $temporaryBinary -Force -ErrorAction SilentlyContinue
    }

    $configPath = (& $installedBinary config path | Select-Object -Last 1).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($configPath)) {
        throw 'Unable to resolve the configuration path'
    }

    $legacyConfig = if ([string]::IsNullOrWhiteSpace($env:APPDATA)) {
        $null
    }
    else {
        Join-Path $env:APPDATA "$appDirectory\config.toml"
    }
    $migratedLegacyConfig = $false

    if (-not (Test-Path -LiteralPath $configPath -PathType Leaf) -and
        $null -ne $legacyConfig -and
        (Test-Path -LiteralPath $legacyConfig -PathType Leaf)) {
        Invoke-AkefClaim -Binary $installedBinary -Arguments @('--config', $legacyConfig, 'config', 'validate')
        New-Item -ItemType Directory -Path (Split-Path -Parent $configPath) -Force | Out-Null
        Copy-Item -LiteralPath $legacyConfig -Destination $configPath
        $migratedLegacyConfig = $true
        Write-Output "Copied legacy Windows configuration to $configPath"
    }
    elseif ((Test-Path -LiteralPath $configPath -PathType Leaf) -and
        $null -ne $legacyConfig -and
        (Test-Path -LiteralPath $legacyConfig -PathType Leaf)) {
        $currentHash = (Get-FileHash -LiteralPath $configPath -Algorithm SHA256).Hash
        $legacyHash = (Get-FileHash -LiteralPath $legacyConfig -Algorithm SHA256).Hash
        $migratedLegacyConfig = $currentHash -eq $legacyHash
    }

    if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
        Invoke-AkefClaim -Binary $installedBinary -Arguments @('config', 'init')
        Write-Output "Created $configPath"
        Write-Output 'Edit the placeholder credentials, then run this installer again.'
        if (-not (Test-PathOnUserPath -Path $installDirectory)) {
            Write-Warning "$installDirectory is not on PATH; add it before invoking akef-claim.exe by name."
        }
        exit 0
    }

    Invoke-AkefClaim -Binary $installedBinary -Arguments @('config', 'validate')
    $installedLauncher = Join-Path $installDirectory 'akef-claim-scheduled.vbs'
    $temporaryLauncher = Join-Path $installDirectory ".akef-claim-scheduled.vbs.install.$([guid]::NewGuid().ToString('N'))"
    try {
        $launcherContent = New-TaskLauncherContent -Binary $installedBinary -Config $configPath
        [IO.File]::WriteAllText($temporaryLauncher, $launcherContent, [Text.Encoding]::Unicode)
        Move-Item -LiteralPath $temporaryLauncher -Destination $installedLauncher -Force
    }
    finally {
        Remove-Item -LiteralPath $temporaryLauncher -Force -ErrorAction SilentlyContinue
    }

    $scheduledAt = [datetime]::Today.Add($parsedTime.TimeOfDay)
    Install-ScheduledClaim -Launcher $installedLauncher -At $scheduledAt

    if ($migratedLegacyConfig) {
        Remove-Item -LiteralPath $legacyConfig -Force
        Remove-Item -LiteralPath (Split-Path -Parent $legacyConfig) -Force -ErrorAction SilentlyContinue
        Write-Output 'Removed legacy Windows configuration after scheduler installation.'
    }

    Write-Output "Installed $installedBinary"
    Write-Output "Configuration: $configPath"
    Write-Output "Daily schedule: $Time local time"
    if (-not (Test-PathOnUserPath -Path $installDirectory)) {
        Write-Warning "$installDirectory is not on PATH; add it before invoking akef-claim.exe by name."
    }
}
catch {
    Write-Error $_.Exception.Message
    exit 1
}
