package apply

// windowsDNSScript points up adapters that have an IPv4 default gateway at the
// local dnscrypt-proxy listener.
//
// IPv4 is always set to 127.0.0.1. IPv6 is set to ::1 only when the ms_tcpip6
// binding is enabled. A missing or failed IPv6 update is skipped so the IPv4
// change still stands.
const windowsDNSScript = `
$ErrorActionPreference = 'Stop'
$cfgs = Get-NetIPConfiguration | Where-Object { $_.IPv4DefaultGateway -ne $null -and $_.NetAdapter.Status -eq 'Up' }
if (-not $cfgs) { Write-Output 'no-adapters'; exit 0 }
foreach ($c in $cfgs) {
  $idx = $c.InterfaceIndex
  $alias = [string]$c.InterfaceAlias
  Set-DnsClientServerAddress -InterfaceIndex $idx -ServerAddresses @('127.0.0.1')
  $v6 = $false
  try {
    $name = [System.Management.Automation.WildcardPattern]::Escape($alias)
    $binding = @(Get-NetAdapterBinding -Name $name -ComponentID 'ms_tcpip6' -ErrorAction Stop)
    foreach ($b in $binding) {
      if ($b.Enabled) { $v6 = $true }
    }
  } catch {
    $v6 = $false
  }
  if (-not $v6) {
    Write-Output ("skip6:" + $alias)
    Write-Output ("set:" + $alias)
    continue
  }
  try {
    Set-DnsClientServerAddress -InterfaceIndex $idx -ServerAddresses @('::1')
    Write-Output ("set6:" + $alias)
  } catch {
    Write-Output ("skip6:" + $alias)
  }
  Write-Output ("set:" + $alias)
}
`
