package autostart

import (
	"bytes"
	"encoding/xml"
	"os"
	"strings"
)

type darwinToggle struct {
	label string
	help  string
	path  string
	exec  string
}

func (t darwinToggle) Label() string { return t.label }
func (t darwinToggle) Help() string  { return t.help }

func (t darwinToggle) Enabled() (bool, error) {
	b, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	s := string(b)
	const key = "<key>RunAtLoad</key>"
	i := strings.Index(s, key)
	if i < 0 {
		return false, nil
	}
	rest := strings.TrimSpace(s[i+len(key):])
	return strings.HasPrefix(rest, "<true"), nil
}

func (t darwinToggle) Set(on bool) error {
	if !on {
		return removeFile(t.path)
	}
	if strings.TrimSpace(t.exec) == "" {
		return errNoExecutable
	}
	return writeFile(t.path, darwinPlist(t.exec))
}

func darwinPlist(exec string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + LaunchAgentLabel + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + xmlEscape(exec) + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
