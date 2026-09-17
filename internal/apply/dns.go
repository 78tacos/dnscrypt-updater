package apply

import "context"

// DNSSetter changes OS DNS to 127.0.0.1 so the system uses the local proxy.
type DNSSetter interface {
	SetLoopback(ctx context.Context) error
}

type nativeDNS struct{}
