//go:build !windows

package apply

import (
	"context"
	"fmt"
)

func (nativeDNS) SetLoopback(context.Context) error {
	return fmt.Errorf("automatic system DNS change is only implemented on Windows; set the adapter to 127.0.0.1 manually")
}
