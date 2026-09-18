//go:build windows

package apply

import (
	"context"
	"os/exec"
	"strings"
)

func (s nativeService) Query(ctx context.Context) (ServiceStatus, error) {
	out, err := s.run(ctx, "", "sc", "query", "dnscrypt-proxy")
	text := strings.ToUpper(string(out))
	if err != nil && !strings.Contains(text, "1060") && !strings.Contains(text, "DOES NOT EXIST") {
		// sc query returns a non-zero exit when the service is missing (1060)
		if strings.Contains(text, "FAILED") && strings.Contains(text, "1060") {
			return ServiceStatus{}, nil
		}
	}
	if strings.Contains(text, "1060") || strings.Contains(text, "DOES NOT EXIST") {
		return ServiceStatus{}, nil
	}
	st := ServiceStatus{Installed: strings.Contains(text, "SERVICE_NAME")}
	if strings.Contains(text, "RUNNING") {
		st.Running = true
	}
	return st, nil
}

func (s nativeService) Install(ctx context.Context, binPath string) error {
	return s.proxyService(ctx, binPath, "install")
}

func (s nativeService) Stop(ctx context.Context, binPath string) error {
	return s.proxyService(ctx, binPath, "stop")
}

func (s nativeService) Start(ctx context.Context, binPath string) error {
	return s.proxyService(ctx, binPath, "start")
}

func defaultService(goos string) ServiceManager {
	return nativeService{GOOS: goos, Command: func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		if dir != "" {
			cmd.Dir = dir
		}
		return cmd.CombinedOutput()
	}}
}
