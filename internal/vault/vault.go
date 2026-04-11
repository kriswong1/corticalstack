// Package vault provides read/write access to an Obsidian-compatible
// markdown vault on disk.
package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Vault represents an Obsidian-compatible markdown vault on disk.
type Vault struct {
	path string
}

// New creates a Vault pointing at the given directory.
func New(path string) *Vault {
	return &Vault{path: path}
}

// Path returns the vault root directory.
func (v *Vault) Path() string {
	return v.path
}

// ReadNote reads and parses a markdown file at a relative path.
func (v *Vault) ReadNote(relPath string) (*Note, error) {
	content, err := os.ReadFile(filepath.Join(v.path, relPath))
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
	fullPath := filepath.Join(v.path, relPath)
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
func (v *Vault) ReadFile(relPath string) (string, error) {
	content, err := os.ReadFile(filepath.Join(v.path, relPath))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// WriteFile writes raw content to a file in the vault.
func (v *Vault) WriteFile(relPath string, content string) error {
	fullPath := filepath.Join(v.path, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(content), 0o600)
}

// Exists checks if a file exists in the vault.
func (v *Vault) Exists(relPath string) bool {
	_, err := os.Stat(filepath.Join(v.path, relPath))
	return err == nil
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
