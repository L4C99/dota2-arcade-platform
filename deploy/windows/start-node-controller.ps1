param([Parameter(Mandatory=$true)][string]$Root)
$ErrorActionPreference = 'Stop'
$binary = Join-Path $Root 'bin\node-controller.exe'
$configPath = Join-Path $Root 'config\node-controller.json'
if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { throw 'Controller binary missing' }
if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) { throw 'Controller config missing' }
$logDir = Join-Path $Root 'logs'
New-Item -ItemType Directory -Path $logDir -Force | Out-Null
Start-Transcript -LiteralPath (Join-Path $logDir 'node-controller.log') -Append | Out-Null
try {
    & $binary run --config $configPath
    exit $LASTEXITCODE
} finally { Stop-Transcript | Out-Null }
