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
