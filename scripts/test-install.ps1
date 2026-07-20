[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    (Join-Path $PSScriptRoot 'install.ps1'),
    [ref]$tokens,
    [ref]$parseErrors
)
if ($parseErrors.Count -gt 0) {
    throw "install.ps1 contains parse errors: $($parseErrors.Message -join '; ')"
}

$taskNameAssignment = $ast.FindAll({
        param($node)
        $node -is [System.Management.Automation.Language.AssignmentStatementAst] -and
            $node.Left.VariablePath.UserPath -eq 'taskName'
    }, $true) | Select-Object -First 1
if ($null -eq $taskNameAssignment) {
    throw 'install.ps1 does not assign taskName'
}
$taskNameExpression = $taskNameAssignment.Right
if ($taskNameExpression -is [System.Management.Automation.Language.CommandExpressionAst]) {
    $taskNameExpression = $taskNameExpression.Expression
}
if ($taskNameExpression -isnot [System.Management.Automation.Language.StringConstantExpressionAst]) {
    throw 'install.ps1 taskName must be a string literal'
}
$taskName = $taskNameExpression.Value
if ($taskName.IndexOfAny([IO.Path]::GetInvalidFileNameChars()) -ge 0) {
    throw "install.ps1 uses an invalid Scheduled Task name: $taskName"
}

foreach ($command in 'Get-ScheduledTask', 'New-ScheduledTask', 'New-ScheduledTaskAction', 'New-ScheduledTaskPrincipal', 'New-ScheduledTaskTrigger', 'Register-ScheduledTask', 'Unregister-ScheduledTask') {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
        throw "$command is unavailable; cannot run the Scheduled Task integration test"
    }
}
$temporaryTaskName = "$taskName Integration Test $([guid]::NewGuid().ToString('N'))"
try {
    $action = New-ScheduledTaskAction -Execute (Join-Path $env:SystemRoot 'System32\cmd.exe') -Argument '/c exit 0'
    $trigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(5)
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    $principal = New-ScheduledTaskPrincipal -UserId $identity -LogonType Interactive -RunLevel Limited
    $task = New-ScheduledTask -Action $action -Trigger $trigger -Principal $principal
    Register-ScheduledTask -TaskName $temporaryTaskName -InputObject $task -Force | Out-Null
    if ($null -eq (Get-ScheduledTask -TaskName $temporaryTaskName -ErrorAction Stop)) {
        throw 'temporary Scheduled Task was not found after registration'
    }
}
finally {
    Unregister-ScheduledTask -TaskName $temporaryTaskName -Confirm:$false -ErrorAction SilentlyContinue
}

foreach ($name in 'ConvertTo-VbScriptStringLiteral', 'New-TaskLauncherContent') {
    $definition = $ast.FindAll({
            param($node)
            $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name
        }, $true) | Select-Object -First 1
    if ($null -eq $definition) {
        throw "install.ps1 does not define $name"
    }
    Invoke-Expression $definition.Extent.Text
}

$binary = 'C:\Users\Test User\AppData\Local\Programs\akef-skport-claim\bin\akef-claim.exe'
$config = 'C:\Users\Test User\AppData\Local\akef-skport-claim\config.toml'
$launcher = New-TaskLauncherContent -Binary $binary -Config $config
$requiredFragments = @(
    'FileExists("C:\Users\Test User\AppData\Local\Programs\akef-skport-claim\bin\akef-claim.exe")',
    'command = """C:\Users\Test User\AppData\Local\Programs\akef-skport-claim\bin\akef-claim.exe"" --silent run --config ""C:\Users\Test User\AppData\Local\akef-skport-claim\config.toml"""',
    'shell.Run(command, 0, True)',
    'If exitCode = 30 Then',
    'WScript.Quit 0'
)
foreach ($fragment in $requiredFragments) {
    if (-not $launcher.Contains($fragment)) {
        throw "generated launcher is missing: $fragment"
    }
}

Write-Output 'Windows launcher generation test passed.'
