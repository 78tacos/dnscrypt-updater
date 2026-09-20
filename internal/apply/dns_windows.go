//go:build windows

package apply

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const windowsDNSScript = `
$ErrorActionPreference = 'Stop'
$cfgs = Get-NetIPConfiguration | Where-Object { $_.IPv4DefaultGateway -ne $null -and $_.NetAdapter.Status -eq 'Up' }
if (-not $cfgs) { Write-Output 'no-adapters'; exit 0 }
foreach ($c in $cfgs) {
  Set-DnsClientServerAddress -InterfaceIndex $c.InterfaceIndex -ServerAddresses @('127.0.0.1')
  Write-Output ("set:" + $c.InterfaceAlias)
}
`

func (nativeDNS) SetLoopback(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", windowsDNSScript)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	hideConsole(cmd)
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(buf.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("set system DNS: %s", msg)
	}
	return nil
}
