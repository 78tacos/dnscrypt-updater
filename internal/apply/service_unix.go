//go:build !windows

package apply

import (
	"context"
	"strings"
)

func (s nativeService) Query(ctx context.Context) (ServiceStatus, error) {
	out, err := s.run(ctx, "", "systemctl", "is-active", "dnscrypt-proxy")
	text := strings.TrimSpace(string(out))
	if err != nil && text != "inactive" && text != "failed" && text != "unknown" && text != "inactive\n" {
		if text == "" {
			return ServiceStatus{}, nil
		}
	}
	st := ServiceStatus{}
	switch text {
	case "active":
		st.Installed = true
		st.Running = true
	case "inactive", "failed":
		st.Installed = true
	default:
		// try -service query via binary later; treat as not installed
	}
	return st, nil
}

func (s nativeService) Install(ctx context.Context, binPath string) error {
	return s.proxyService(ctx, binPath, "install")
}

func (s nativeService) Stop(ctx context.Context, binPath string) error {
	if err := s.proxyService(ctx, binPath, "stop"); err == nil {
		return nil
	}
	_, err := s.run(ctx, "", "systemctl", "stop", "dnscrypt-proxy")
	return err
}

func (s nativeService) Start(ctx context.Context, binPath string) error {
	if err := s.proxyService(ctx, binPath, "start"); err == nil {
		return nil
	}
	_, err := s.run(ctx, "", "systemctl", "start", "dnscrypt-proxy")
	return err
}

func defaultService(goos string) ServiceManager {
	return nativeService{GOOS: goos}
}
