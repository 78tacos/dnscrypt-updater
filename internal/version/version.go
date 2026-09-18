// Package version compares dnscrypt-proxy release tags and CLI version strings.
//
// Tags and CLI output typically look like "2.1.18" or "v2.1.18". Comparison
// follows a practical SemVer subset: optional v prefix, numeric core
// identifiers, optional pre-release, ignored build metadata.
package version

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Version is a parsed release or CLI version.
type Version struct {
	// Core is the numeric identifiers (major, minor, patch, …). Missing
	// trailing identifiers compare as 0 (so 2.1 == 2.1.0).
	Core []int
	// Pre is the pre-release identifier list after the first hyphen
	// (e.g. "beta", "1" for 2.1.18-beta.1). Empty means a release build.
	Pre []string
	// Original is the input after trimming space.
	Original string
}

func (v Version) IsZero() bool {
	return len(v.Core) == 0 && v.Original == ""
}

func (v Version) String() string {
	if v.IsZero() {
		return ""
	}
	parts := make([]string, len(v.Core))
	for i, n := range v.Core {
		parts[i] = strconv.Itoa(n)
	}
	s := strings.Join(parts, ".")
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	return s
}

// Normalize strips a leading v/V and surrounding space. Invalid input is
// returned trimmed but otherwise unchanged.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	v, err := Parse(s)
	if err != nil {
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V"))
	}
	return v.String()
}

// Parse parses a version or GitHub tag. The optional v/V prefix is ignored.
func Parse(s string) (Version, error) {
	orig := strings.TrimSpace(s)
	if orig == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	in := orig
	if len(in) > 0 && (in[0] == 'v' || in[0] == 'V') {
		in = in[1:]
	}
	in = strings.TrimSpace(in)
	if in == "" {
		return Version{}, fmt.Errorf("empty version after prefix: %q", orig)
	}

	// Build metadata does not participate in comparison.
	if i := strings.IndexByte(in, '+'); i >= 0 {
		in = in[:i]
	}
	if in == "" {
		return Version{}, fmt.Errorf("invalid version %q", orig)
	}

	coreStr, preStr := in, ""
	if i := strings.IndexByte(in, '-'); i >= 0 {
		if i == 0 || i == len(in)-1 {
			return Version{}, fmt.Errorf("invalid version %q", orig)
		}
		coreStr, preStr = in[:i], in[i+1:]
	}
	if coreStr == "" {
		return Version{}, fmt.Errorf("invalid version %q", orig)
	}

	coreParts := strings.Split(coreStr, ".")
	core := make([]int, 0, len(coreParts))
	for _, p := range coreParts {
		if p == "" {
			return Version{}, fmt.Errorf("invalid version %q: empty numeric identifier", orig)
		}
		for _, r := range p {
			if !unicode.IsDigit(r) {
				return Version{}, fmt.Errorf("invalid version %q: non-numeric identifier %q", orig, p)
			}
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version %q: %w", orig, err)
		}
		core = append(core, n)
	}

	var pre []string
	if preStr != "" {
		pre = strings.Split(preStr, ".")
		for _, p := range pre {
			if p == "" {
				return Version{}, fmt.Errorf("invalid version %q: empty pre-release identifier", orig)
			}
		}
	}

	return Version{Core: core, Pre: pre, Original: orig}, nil
}

// CompareStrings parses a and b and returns -1, 0, or 1.
func CompareStrings(a, b string) (int, error) {
	va, err := Parse(a)
	if err != nil {
		return 0, fmt.Errorf("local/left version: %w", err)
	}
	vb, err := Parse(b)
	if err != nil {
		return 0, fmt.Errorf("remote/right version: %w", err)
	}
	return Compare(va, vb), nil
}

// Compare reports whether a is less than, equal to, or greater than b.
// A pre-release is less than the corresponding release (2.1.18-beta < 2.1.18).
func Compare(a, b Version) int {
	n := len(a.Core)
	if len(b.Core) > n {
		n = len(b.Core)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(a.Core) {
			ai = a.Core[i]
		}
		if i < len(b.Core) {
			bi = b.Core[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}

	aPre, bPre := len(a.Pre) > 0, len(b.Pre) > 0
	switch {
	case !aPre && !bPre:
		return 0
	case aPre && !bPre:
		return -1
	case !aPre && bPre:
		return 1
	}

	m := len(a.Pre)
	if len(b.Pre) > m {
		m = len(b.Pre)
	}
	for i := 0; i < m; i++ {
		if i >= len(a.Pre) {
			return -1
		}
		if i >= len(b.Pre) {
			return 1
		}
		if c := comparePreIdent(a.Pre[i], b.Pre[i]); c != 0 {
			return c
		}
	}
	return 0
}

func comparePreIdent(a, b string) int {
	aNum, aIsNum := parsePreNum(a)
	bNum, bIsNum := parsePreNum(b)
	switch {
	case aIsNum && bIsNum:
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		return 0
	case aIsNum && !bIsNum:
		return -1
	case !aIsNum && bIsNum:
		return 1
	default:
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
}

func parsePreNum(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
