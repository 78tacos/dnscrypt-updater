package proxyconf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileChange is a companion list file write.
type FileChange struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func allowedCompanion(name string) bool {
	base := filepath.Base(name)
	for _, c := range companionFiles {
		if c.Live == base {
			return true
		}
	}
	return false
}

func resolveListPath(installDir, name string) (string, error) {
	base := filepath.Base(strings.TrimSpace(name))
	if !allowedCompanion(base) {
		return "", fmt.Errorf("refusing to edit %q", name)
	}
	return filepath.Join(installDir, base), nil
}

// LoadListFiles returns live companion file contents (empty string if missing).
func LoadListFiles(installDir string) ([]FileChange, error) {
	out := make([]FileChange, 0, len(companionFiles))
	for _, c := range companionFiles {
		p := filepath.Join(installDir, c.Live)
		b, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				ex, e2 := ExampleListFile(c.Example)
				content := ""
				if e2 == nil {
					content = string(ex)
				}
				out = append(out, FileChange{Name: c.Live, Content: content})
				continue
			}
			return nil, err
		}
		out = append(out, FileChange{Name: c.Live, Content: string(b)})
	}
	return out, nil
}

func writeFileAtomic(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeFileAtomic(dst, string(b))
}

func backupPath(path string) string {
	return path + ".bak"
}

// timestampedBackupPath is path.YYYYMMDDTHHMMSSZ.bak, with a numeric suffix
// if that name is already taken.
func timestampedBackupPath(path string, now time.Time) string {
	stamp := now.UTC().Format("20060102T150405Z")
	candidate := path + "." + stamp + ".bak"
	if _, err := os.Stat(candidate); err != nil {
		return candidate
	}
	for n := 2; n < 1000; n++ {
		candidate = fmt.Sprintf("%s.%s-%d.bak", path, stamp, n)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
	return candidate
}

// InProgramFiles reports a Windows Program Files install location.
func InProgramFiles(dir string) bool {
	n := strings.ToLower(filepath.ToSlash(dir))
	return strings.Contains(n, "/program files/") || strings.HasSuffix(n, "/program files") ||
		strings.Contains(n, "/program files (x86)/") || strings.HasSuffix(n, "/program files (x86)")
}

func CanWrite(path string) bool {
	dir := filepath.Dir(path)
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return false
		}
		_ = f.Close()
		return true
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}
