package apply

import "strings"

// powershellArgumentList builds a PowerShell -ArgumentList array expression.
//
// Start-Process joins array elements with spaces and does not quote them for
// CreateProcess. Paths like C:\Program Files\... must therefore include their
// own Windows command-line quotes inside each element, otherwise the elevated
// child sees a split -install-dir / -staging / -config value and fails (or
// worse, writes to the wrong folder).
func powershellArgumentList(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = powershellSingleQuote(windowsCmdlineArg(a))
	}
	return "@(" + strings.Join(parts, ",") + ")"
}

func powershellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// windowsCmdlineArg returns s, wrapped in double quotes when needed so that
// Windows CreateProcess keeps it as one argv element.
func windowsCmdlineArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
