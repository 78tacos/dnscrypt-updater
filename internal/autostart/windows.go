package autostart

import "strings"

type windowsToggle struct {
	label string
	help  string
	name  string
	exec  string
	reg   RegistryStore
}

func (w windowsToggle) Label() string { return w.label }
func (w windowsToggle) Help() string  { return w.help }

func (w windowsToggle) Enabled() (bool, error) {
	if w.reg == nil {
		return false, errNoRegistry
	}
	v, ok, err := w.reg.Get(w.name)
	if err != nil || !ok {
		return false, err
	}
	return strings.TrimSpace(v) != "", nil
}

func (w windowsToggle) Set(on bool) error {
	if w.reg == nil {
		return errNoRegistry
	}
	if !on {
		return w.reg.Delete(w.name)
	}
	if strings.TrimSpace(w.exec) == "" {
		return errNoExecutable
	}
	return w.reg.Set(w.name, FormatRunCommand(w.exec))
}
