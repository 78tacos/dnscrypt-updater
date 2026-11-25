//go:build windows

package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

type systemRegistry struct{}

func realRegistry() RegistryStore { return systemRegistry{} }

func openRunKey(writable bool) (registry.Key, error) {
	access := uint32(registry.QUERY_VALUE)
	if writable {
		access |= registry.SET_VALUE
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, RunKey, access)
	if err == nil {
		return k, nil
	}
	if !writable || !errors.Is(err, registry.ErrNotExist) {
		return 0, err
	}
	k, _, err = registry.CreateKey(registry.CURRENT_USER, RunKey, access)
	return k, err
}

func (systemRegistry) Get(name string) (string, bool, error) {
	k, err := openRunKey(false)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (systemRegistry) Set(name, value string) error {
	k, err := openRunKey(true)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

func (systemRegistry) Delete(name string) error {
	k, err := openRunKey(true)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	defer k.Close()
	err = k.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
