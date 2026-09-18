package apply

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ServiceManager installs, starts, and stops the official dnscrypt-proxy service.
type ServiceManager interface {
	Query(ctx context.Context) (ServiceStatus, error)
	Install(ctx context.Context, binPath string) error
	Stop(ctx context.Context, binPath string) error
	Start(ctx context.Context, binPath string) error
}

// ServiceStatus is the Windows (or -service) state of dnscrypt-proxy.
type ServiceStatus struct {
	Installed bool
	Running   bool
}

type nativeService struct {
	GOOS    string
	Command func(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

func (s nativeService) run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}
	if s.Command != nil {
		return s.Command(ctx, dir, name, args...)
	}
	return runCmd(ctx, dir, name, args...)
}

func runCmd(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.Bytes(), err
}

func (s nativeService) proxyService(ctx context.Context, binPath, action string) error {
	dir := filepath.Dir(binPath)
	out, err := s.run(ctx, dir, binPath, "-service", action)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("dnscrypt-proxy -service %s: %s", action, msg)
	}
	return nil
}

func runConfigCheck(ctx context.Context, bin, configPath string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, bin, "-config", configPath, "-check")
	cmd.Dir = filepath.Dir(configPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("dnscrypt-proxy -check: %s", msg)
	}
	return nil
}
