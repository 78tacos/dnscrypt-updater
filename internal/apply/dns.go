package apply

import "context"

// DNSSetter changes OS DNS to 127.0.0.1 and ::1 so the system uses the local proxy.
// Adapters without IPv6 are left on 127.0.0.1 only.
type DNSSetter interface {
	SetLoopback(ctx context.Context) error
}

type nativeDNS struct{}
