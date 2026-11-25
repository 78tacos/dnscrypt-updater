package autostart

import (
	"os"
	"strings"
)

type linuxToggle struct {
	label string
	help  string
	path  string
	exec  string
}

func (t linuxToggle) Label() string { return t.label }
func (t linuxToggle) Help() string  { return t.help }

func (t linuxToggle) Enabled() (bool, error) {
	b, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	hidden := false
	disabled := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "hidden":
			if strings.EqualFold(strings.TrimSpace(v), "true") {
				hidden = true
			}
		case "x-gnome-autostart-enabled":
			if strings.EqualFold(strings.TrimSpace(v), "false") {
				disabled = true
			}
		}
	}
	return !hidden && !disabled, nil
}

func (t linuxToggle) Set(on bool) error {
	if !on {
		return removeFile(t.path)
	}
	if strings.TrimSpace(t.exec) == "" {
		return errNoExecutable
	}
	return writeFile(t.path, linuxDesktop(t.exec))
}

func linuxDesktop(exec string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Version=1.0\n" +
		"Name=dnscrypt-proxy-updater\n" +
		"Comment=Watch for official dnscrypt-proxy updates\n" +
		"Exec=" + desktopExec(exec) + "\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
}

func desktopExec(path string) string {
	if strings.ContainsAny(path, " \t\"\\$`") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}
