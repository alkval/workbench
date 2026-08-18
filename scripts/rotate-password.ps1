[CmdletBinding()]
param(
    [string]$InstallDirectory = ''
)

$ErrorActionPreference = 'Stop'
$TaskName = if (Get-ScheduledTask -TaskName 'Workbench' -ErrorAction SilentlyContinue) { 'Workbench' } else { 'Alexander Workbench' }

if ([string]::IsNullOrWhiteSpace($InstallDirectory)) {
    $legacyDirectory = 'C:\ProgramData\Alexander\Workbench'
    $InstallDirectory = if (Test-Path $legacyDirectory) { $legacyDirectory } else { 'C:\ProgramData\Workbench' }
}

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Open PowerShell as Administrator, then run this script again.'
}

$secretPath = Join-Path $InstallDirectory 'password.txt'
if (-not (Test-Path $secretPath)) {
    throw "Workbench password file was not found at $secretPath"
}

$firstSecure = Read-Host 'Enter your new Workbench password' -AsSecureString
$secondSecure = Read-Host 'Confirm your new Workbench password' -AsSecureString

function ConvertFrom-WorkbenchSecureString([Security.SecureString]$Value) {
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Value)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    }
}

$password = ConvertFrom-WorkbenchSecureString $firstSecure
$confirmation = ConvertFrom-WorkbenchSecureString $secondSecure
if ($password.Length -lt 10) {
    throw 'The Workbench password must contain at least 10 characters.'
}
if ($password -cne $confirmation) {
    throw 'The passwords did not match. No changes were made.'
}
Set-Content -NoNewline -Encoding utf8 $secretPath $password

$currentUser = "$env:USERDOMAIN\$env:USERNAME"
& icacls.exe $secretPath /inheritance:r /grant:r "${currentUser}:(R,W)" 'SYSTEM:(F)' 'BUILTIN\Administrators:(F)' | Out-Null

Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
$installedExecutable = Join-Path $InstallDirectory 'workbench.exe'
Get-CimInstance Win32_Process -Filter "Name = 'workbench.exe'" |
    Where-Object { $_.ExecutablePath -eq $installedExecutable } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
Start-Sleep -Milliseconds 500
Start-ScheduledTask -TaskName $TaskName

$healthy = $false
for ($attempt = 0; $attempt -lt 15; $attempt++) {
    Start-Sleep -Seconds 1
    try {
        $response = Invoke-RestMethod -Uri 'http://127.0.0.1:8787/healthz' -TimeoutSec 2
        if ($response.ok) { $healthy = $true; break }
    } catch {}
}
if (-not $healthy) {
    throw "Workbench did not become healthy. Check $InstallDirectory\workbench.log"
}

Write-Host ''
Write-Host 'Workbench password rotated successfully.' -ForegroundColor Green
Write-Host 'Your custom dashboard password is now active.' -ForegroundColor Yellow
