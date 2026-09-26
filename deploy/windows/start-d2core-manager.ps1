param([Parameter(Mandatory=$true)][string]$Root)
$ErrorActionPreference = 'Stop'
$configPath = Join-Path $Root 'config\node-controller.json'
$config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
$binary = Join-Path $Root 'bin\d2core.exe'
$min = [int]$config.network.localPortMin
$max = [int]$config.network.localPortMax
if ($min -lt 1 -or $max -gt 65535 -or $min -gt $max) { throw 'Invalid Controller local port range' }
if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { throw 'Fixed d2core binary missing' }
$logDir = Join-Path $Root 'logs'
New-Item -ItemType Directory -Path $logDir -Force | Out-Null
Start-Transcript -LiteralPath (Join-Path $logDir 'd2core-manager.log') -Append | Out-Null
try {
    & $binary serve --data-dir $config.d2coreDataDir --port-min $min --port-max $max
    exit $LASTEXITCODE
} finally { Stop-Transcript | Out-Null }
