[CmdletBinding()]
param(
    [string]$InstallDirectory = ''
)

$ErrorActionPreference = 'Stop'
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Executable = Join-Path $ProjectRoot 'dist\workbench.exe'
$Config = Join-Path $ProjectRoot 'config\services.windows.json'
$TaskName = 'Workbench'
$LegacyTaskName = 'Alexander Workbench'
$FirewallName = 'Workbench (private networks)'

if ([string]::IsNullOrWhiteSpace($InstallDirectory)) {
    $legacyDirectory = 'C:\ProgramData\Alexander\Workbench'
    $InstallDirectory = if (Test-Path $legacyDirectory) { $legacyDirectory } else { 'C:\ProgramData\Workbench' }
}

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Open PowerShell as Administrator, then run this installer again.'
}
if (-not (Test-Path $Executable)) {
    throw "Missing $Executable. Run .\scripts\build.ps1 first."
}

New-Item -ItemType Directory -Force $InstallDirectory | Out-Null
New-Item -ItemType Directory -Force (Join-Path $InstallDirectory 'data') | Out-Null
foreach ($existingTaskName in @($TaskName, $LegacyTaskName)) {
if (Get-ScheduledTask -TaskName $existingTaskName -ErrorAction SilentlyContinue) {
    Stop-ScheduledTask -TaskName $existingTaskName -ErrorAction SilentlyContinue
    $installedExecutable = Join-Path $InstallDirectory 'workbench.exe'
    Get-CimInstance Win32_Process -Filter "Name = 'workbench.exe'" |
        Where-Object { $_.ExecutablePath -eq $installedExecutable } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
    Start-Sleep -Milliseconds 500
    if ($existingTaskName -eq $LegacyTaskName) {
        Unregister-ScheduledTask -TaskName $LegacyTaskName -Confirm:$false
    }
}
}
Copy-Item -Force $Executable (Join-Path $InstallDirectory 'workbench.exe')
Copy-Item -Force $Config (Join-Path $InstallDirectory 'services.json')

$SecretPath = Join-Path $InstallDirectory 'password.txt'
if (-not (Test-Path $SecretPath)) {
    $passwordBytes = New-Object byte[] 18
    $random = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $random.GetBytes($passwordBytes)
    } finally {
        $random.Dispose()
    }
    $generatedPassword = [Convert]::ToBase64String($passwordBytes).TrimEnd('=').Replace('+','-').Replace('/','_')
    Set-Content -NoNewline -Encoding utf8 $SecretPath $generatedPassword
} else {
    $generatedPassword = Get-Content -Raw $SecretPath
}
$currentUser = "$env:USERDOMAIN\$env:USERNAME"
& icacls.exe $SecretPath /inheritance:r /grant:r "${currentUser}:(R,W)" 'SYSTEM:(F)' 'BUILTIN\Administrators:(F)' | Out-Null

$action = New-ScheduledTaskAction -Execute (Join-Path $InstallDirectory 'workbench.exe') -WorkingDirectory $InstallDirectory
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $currentUser
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 5 -RestartInterval (New-TimeSpan -Minutes 1)
$taskPrincipal = New-ScheduledTaskPrincipal -UserId $currentUser -LogonType Interactive -RunLevel Limited
$task = New-ScheduledTask -Action $action -Trigger $trigger -Settings $settings -Principal $taskPrincipal -Description 'Private controller for local development and AI services.'
Register-ScheduledTask -TaskName $TaskName -InputObject $task -Force | Out-Null

Get-NetFirewallRule -DisplayName 'Alexander Workbench (private networks)' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
Get-NetFirewallRule -DisplayName $FirewallName -ErrorAction SilentlyContinue | Remove-NetFirewallRule
New-NetFirewallRule -DisplayName $FirewallName -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8787 -Profile Private -RemoteAddress LocalSubnet | Out-Null

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
Write-Host 'Workbench is installed and running.' -ForegroundColor Green
Write-Host "Dashboard password: $generatedPassword" -ForegroundColor Yellow
Write-Host 'Save this password in your password manager.'
Write-Host "Health check: http://127.0.0.1:8787/healthz"
