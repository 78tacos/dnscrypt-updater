package detect

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/78tacos/dnscrypt-updater/internal/version"
)

// versionToken matches a SemVer-ish token, including an optional v prefix
// and pre-release suffix. Used to scrape dnscrypt-proxy CLI output.
var versionToken = regexp.MustCompile(`(?i)\bv?\d+\.\d+(?:\.\d+)*(?:-[0-9A-Za-z.-]+)?`)

// ParseCLIVersion extracts a version from `dnscrypt-proxy -version` / `--version` stdout.
func ParseCLIVersion(output string) (version.Version, error) {
	s := strings.TrimSpace(output)
	if s == "" {
		return version.Version{}, fmt.Errorf("empty dnscrypt-proxy version output")
	}
	// Prefer the first line; some builds print extra help.
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		first := strings.TrimSpace(s[:i])
		if first != "" {
			s = first
		}
	}
	m := versionToken.FindString(s)
	if m == "" {
		return version.Version{}, fmt.Errorf("no version token in output %q", trimForErr(output))
	}
	v, err := version.Parse(m)
	if err != nil {
		return version.Version{}, fmt.Errorf("parse CLI version %q: %w", m, err)
	}
	return v, nil
}

func trimForErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// ParseSCBinaryPath extracts ImagePath/BINARY_PATH_NAME from `sc qc` output.
func ParseSCBinaryPath(output string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		key := "BINARY_PATH_NAME"
		idx := strings.Index(strings.ToUpper(line), key)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx+len(key):])
		rest = strings.TrimLeft(rest, ": \t")
		path := firstPathToken(rest)
		if path != "" {
			return path, true
		}
	}
	return "", false
}

// ParseSystemctlExecStart extracts the executable from `systemctl show -p ExecStart`.
func ParseSystemctlExecStart(output string) (string, bool) {
	s := strings.TrimSpace(output)
	const marker = "path="
	if i := strings.Index(s, marker); i >= 0 {
		rest := s[i+len(marker):]
		end := len(rest)
		for j, r := range rest {
			if r == ' ' || r == ';' || r == '\n' || r == '\t' {
				end = j
				break
			}
		}
		p := strings.TrimSpace(rest[:end])
		if p != "" {
			return p, true
		}
	}
	return "", false
}

func firstPathToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if s[0] == '"' {
		if i := strings.IndexByte(s[1:], '"'); i >= 0 {
			return s[1 : i+1]
		}
		return strings.Trim(s, `"`)
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
