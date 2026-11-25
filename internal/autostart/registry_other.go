//go:build !windows

package autostart

func realRegistry() RegistryStore { return nil }
