// Package vault provides read/write access to an Obsidian-compatible
// markdown vault on disk.
package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Vault represents an Obsidian-compatible markdown vault on disk.
type Vault struct {
	path string
}

// ErrUnsafePath is returned by SafeRelPath (and every ReadFile / WriteFile
// caller that routes through it) when the supplied relative path would
// escape the vault root, is absolute, or contains a null byte. Callers that
// surface 400-level HTTP errors can errors.Is against this sentinel.
var ErrUnsafePath = errors.New("vault: unsafe path")

// New creates a Vault pointing at the given directory.
func New(path string) *Vault {
	return &Vault{path: path}
}

// Path returns the vault root directory.
func (v *Vault) Path() string {
	return v.path
}

// SafeRelPath validates a caller-supplied relative path and returns a
// cleaned form guaranteed to resolve inside the vault root. It rejects:
//   - empty strings
//   - null bytes (defeats Go's underlying syscall semantics on Unix)
//   - absolute paths (Unix `/`, Windows drive letters `C:\`, UNC `\\host\share`)
//   - any path that, after filepath.Clean, still starts with `..` — i.e.
//     climbs above the vault root
//   - any path whose filepath.Rel against the vault root resolves above the
//     root (defense in depth against Windows-specific quirks of Join/Clean)
//
// Every v3 synthesis endpoint that accepts user-supplied vault-relative
// paths (source_path / pitch_path / source_paths / extra_context_paths)
// must call SafeRelPath before passing the path to ReadFile / ReadNote or
// any store that reads from disk. ReadFile / WriteFile / ReadNote /
// WriteNote / Exists all call SafeRelPath internally as defense in depth,
// so this is belt-and-suspenders at the HTTP boundary — it lets handlers
// reply 400 Bad Request with a clear message instead of surfacing a
// generic store error.
func (v *Vault) SafeRelPath(relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("%w: empty path", ErrUnsafePath)
	}
	if strings.ContainsRune(relPath, 0) {
		return "", fmt.Errorf("%w: null byte in path", ErrUnsafePath)
	}
	// Reject absolute paths (Unix `/`, Windows `C:\`, UNC `\\host\share`,
	// and also a volume-relative `C:foo`).
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("%w: absolute path %q", ErrUnsafePath, relPath)
	}
	// filepath.IsAbs is OS-aware — on Windows it returns false for Unix-
	// style leading slashes ("/etc/passwd") even though those escape the
	// vault just as effectively. Reject them explicitly.
	if strings.HasPrefix(relPath, "/") || strings.HasPrefix(relPath, `\`) {
		return "", fmt.Errorf("%w: absolute path %q", ErrUnsafePath, relPath)
	}
	if len(relPath) >= 2 && relPath[1] == ':' {
		// Windows-style volume relative ("C:foo"): filepath.IsAbs returns
		// false for this on some versions, but it still escapes the vault.
		return "", fmt.Errorf("%w: volume-relative path %q", ErrUnsafePath, relPath)
	}

	cleaned := filepath.Clean(relPath)

	// After Clean, any `..` segment at the front means the path climbed
	// above the vault root. Check both OS-native and forward-slash forms
	// so tests on Windows catch the Unix encoding too.
	if cleaned == ".." ||
		strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) ||
		strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: traversal %q", ErrUnsafePath, relPath)
	}

	// Defense in depth: recompute against the vault root and make sure
	// filepath.Rel agrees. This catches corner cases where Join+Clean
	// might still resolve above the root on exotic inputs.
	full := filepath.Join(v.path, cleaned)
	rel, err := filepath.Rel(v.path, full)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	if rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%w: traversal after resolve %q", ErrUnsafePath, relPath)
	}

	return cleaned, nil
}

// ReadNote reads and parses a markdown file at a relative path.
func (v *Vault) ReadNote(relPath string) (*Note, error) {
	safe, err := v.SafeRelPath(relPath)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(filepath.Join(v.path, safe))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", relPath, err)
	}
	note, err := ParseNote(content)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", relPath, err)
	}
	note.Path = relPath
	return note, nil
}

// WriteNote serializes a Note to disk at the given relative path.
func (v *Vault) WriteNote(relPath string, note *Note) error {
	safe, err := v.SafeRelPath(relPath)
	if err != nil {
		return err
	}
	fullPath := filepath.Join(v.path, safe)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	note.Path = relPath
	content, err := RenderNote(note)
	if err != nil {
		return err
	}
	return os.WriteFile(fullPath, content, 0o600)
}

// ReadFile reads a raw file from the vault (no frontmatter parsing).
// Routes through SafeRelPath so any caller — handler, store, or test —
// that somehow forwards an untrusted path still gets traversal protection.
func (v *Vault) ReadFile(relPath string) (string, error) {
	safe, err := v.SafeRelPath(relPath)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(v.path, safe))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// WriteFile writes raw content to a file in the vault.
func (v *Vault) WriteFile(relPath string, content string) error {
	safe, err := v.SafeRelPath(relPath)
	if err != nil {
		return err
	}
	fullPath := filepath.Join(v.path, safe)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(content), 0o600)
}

// Exists checks if a file exists in the vault. Unsafe paths return false
// (they can't exist inside the vault by definition).
func (v *Vault) Exists(relPath string) bool {
	safe, err := v.SafeRelPath(relPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(v.path, safe))
	return err == nil
}

// Walk visits every .md file in the vault, parses it as a Note, and calls
// fn with the relative path and parsed note. Files that fail to parse are
// silently skipped. Directories starting with "." are skipped.
func (v *Vault) Walk(fn func(relPath string, note *Note)) error {
	return filepath.Walk(v.path, func(fullPath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(v.path, fullPath)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		note, err := v.ReadNote(rel)
		if err != nil {
			return nil // skip unparseable
		}
		fn(rel, note)
		return nil
	})
}

// Slugify converts a string to a URL/filesystem-safe slug.
func Slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
