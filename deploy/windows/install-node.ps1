param(
    [Parameter(Mandatory=$true)][string]$Root,
    [switch]$ValidateOnly,
    [PSCredential]$Credential
)
$ErrorActionPreference = 'Stop'
$Root = [System.IO.Path]::GetFullPath($Root)
if ($Root -match '[^\x20-\x7E]' -or $Root -match '["*?\[\]]') { throw 'Node root must be a safe ASCII absolute path' }
$configPath = Join-Path $Root 'config\node-controller.json'
$secretPath = $null
foreach ($path in @($configPath,(Join-Path $Root 'bin\node-controller.exe'),(Join-Path $Root 'bin\d2core.exe'),(Join-Path $Root 'bin\BUILD.json'),(Join-Path $Root 'bin\start-d2core-manager.ps1'),(Join-Path $Root 'bin\start-node-controller.ps1'))) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Required deployment file missing: $path" }
}
$config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
$secretPath = [string]$config.nodeSecretFile
if (-not (Test-Path -LiteralPath $secretPath -PathType Leaf)) { throw 'Separate node Secret file missing' }
$min = [int]$config.network.localPortMin
$max = [int]$config.network.localPortMax
if ($min -lt 1 -or $max -gt 65535 -or $min -gt $max) { throw 'Invalid local port range' }
$manifest = Get-Content -LiteralPath (Join-Path $Root 'bin\BUILD.json') -Raw | ConvertFrom-Json
if ($manifest.version -ne '0.1.1' -or $manifest.gitCommit -ne '988720ad85af1f0d97bfe98ec4da4fcbb070beea') { throw 'd2core must be fixed v0.1.1' }
Write-Output "Node preflight passed: local port range $min..$max; d2core v0.1.1"
if ($ValidateOnly) { return }
if ($null -eq $Credential) { $Credential = Get-Credential -Message 'Runtime account used by both d2core and Node Controller' }
$powershell = (Get-Process -Id $PID).Path
if (-not (Test-Path -LiteralPath $powershell -PathType Leaf)) { throw 'Cannot locate the current PowerShell executable' }
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -RestartCount 100 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit (New-TimeSpan -Seconds 0)
$managerScript = Join-Path $Root 'bin\start-d2core-manager.ps1'
$controllerScript = Join-Path $Root 'bin\start-node-controller.ps1'
$managerAction = New-ScheduledTaskAction -Execute $powershell -Argument "-NoProfile -NonInteractive -File `"$managerScript`" -Root `"$Root`""
$controllerAction = New-ScheduledTaskAction -Execute $powershell -Argument "-NoProfile -NonInteractive -File `"$controllerScript`" -Root `"$Root`""
$bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Credential.Password)
try {
    $password = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
    Register-ScheduledTask -TaskName 'DotaArcade-d2core' -Action $managerAction -Trigger $trigger -Settings $settings -User $Credential.UserName -Password $password -Force | Out-Null
    Register-ScheduledTask -TaskName 'DotaArcade-Controller' -Action $controllerAction -Trigger $trigger -Settings $settings -User $Credential.UserName -Password $password -Force | Out-Null
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    $password = $null
}
Write-Output 'Startup tasks registered. Start manager first, then Controller; verify their logs and Platform node status.'
