[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$WebRoot = Join-Path $ProjectRoot 'web'
$WorkRoot = Split-Path -Parent $ProjectRoot

Push-Location $WebRoot
try {
    npm install --cache (Join-Path $WorkRoot 'npm-cache')
    npm run build
} finally {
    Pop-Location
}

$env:GOCACHE = Join-Path $WorkRoot 'go-cache'
$env:GOPATH = Join-Path $WorkRoot 'go-path'
$env:GOMODCACHE = Join-Path $WorkRoot 'go-mod-cache'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'

$OutputDirectory = Join-Path $ProjectRoot 'dist'
New-Item -ItemType Directory -Force $OutputDirectory | Out-Null

Push-Location $ProjectRoot
try {
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
    # Build as a Windows GUI-subsystem executable so the background controller
    # never creates an empty console window when Task Scheduler launches it.
    go build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $OutputDirectory 'workbench.exe') ./cmd/workbench
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
} finally {
    Pop-Location
}

Write-Host "Built $OutputDirectory\workbench.exe"
