[CmdletBinding(SupportsShouldProcess)]
param(
    [switch]$RemoveData,
    [string]$InstallDirectory = ''
)

$ErrorActionPreference = 'Stop'
$TaskNames = @('Workbench', 'Alexander Workbench')
$FirewallName = 'Workbench (private networks)'
if ([string]::IsNullOrWhiteSpace($InstallDirectory)) {
    $legacyDirectory = 'C:\ProgramData\Alexander\Workbench'
    $InstallDirectory = if (Test-Path $legacyDirectory) { $legacyDirectory } else { 'C:\ProgramData\Workbench' }
}

foreach ($taskName in $TaskNames) {
if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) {
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
}
}
Get-NetFirewallRule -DisplayName 'Alexander Workbench (private networks)' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
Get-NetFirewallRule -DisplayName $FirewallName -ErrorAction SilentlyContinue | Remove-NetFirewallRule
if ($RemoveData -and $PSCmdlet.ShouldProcess($InstallDirectory, 'Remove Workbench application data')) {
    Remove-Item -LiteralPath $InstallDirectory -Recurse -Force
}
Write-Host 'Workbench startup task and firewall rule removed.'
