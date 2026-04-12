package persona

import (
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kriswong/corticalstack/internal/vault"
)

//go:embed templates/*.md
var embeddedTemplates embed.FS

// Loader reads and caches the three persona files, bootstrapping them from
// embedded templates on first install. It's safe for concurrent use.
type Loader struct {
	vault *vault.Vault

	mu    sync.RWMutex
	cache map[Name]cachedEntry
}

type cachedEntry struct {
	content string
	mtime   time.Time
}

// New creates a loader bound to a vault.
func New(v *vault.Vault) *Loader {
	return &Loader{
		vault: v,
		cache: make(map[Name]cachedEntry),
	}
}

// InitIfMissing writes any missing persona files from the embedded
// templates. Existing files are never touched. Returns the list of files
// that were freshly created.
func (l *Loader) InitIfMissing() (*InitResult, error) {
	result := &InitResult{}
	for _, name := range AllNames() {
		relPath := name.File()
		if l.vault.Exists(relPath) {
			continue
		}
		tmpl, err := embeddedTemplates.ReadFile("templates/" + string(name) + ".md")
		if err != nil {
			return nil, fmt.Errorf("reading embedded %s template: %w", name, err)
		}
		if err := l.vault.WriteFile(relPath, string(tmpl)); err != nil {
			return nil, fmt.Errorf("writing %s: %w", relPath, err)
		}
		result.Created = append(result.Created, name)
	}
	return result, nil
}

// Get returns the current content of a persona file, using an mtime-based
// cache so repeated calls are cheap but direct Obsidian edits are picked
// up automatically on the next call.
func (l *Loader) Get(name Name) (string, error) {
	path := filepath.Join(l.vault.Path(), name.File())
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	l.mu.RLock()
	if entry, ok := l.cache[name]; ok && entry.mtime.Equal(stat.ModTime()) {
		l.mu.RUnlock()
		return entry.content, nil
	}
	l.mu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	l.mu.Lock()
	l.cache[name] = cachedEntry{content: string(data), mtime: stat.ModTime()}
	l.mu.Unlock()

	return string(data), nil
}

// Set writes a persona file and bumps the cache. Used by the dashboard
// editor.
func (l *Loader) Set(name Name, content string) error {
	if !IsValid(string(name)) {
		return fmt.Errorf("invalid persona name: %s", name)
	}
	if err := l.vault.WriteFile(name.File(), content); err != nil {
		return err
	}
	// Invalidate cache so the next Get() reloads with the fresh mtime.
	l.mu.Lock()
	delete(l.cache, name)
	l.mu.Unlock()
	return nil
}

// getTrimmed returns the persona's content capped at its budget, preferring
// to preserve frontmatter and the start of the body.
func (l *Loader) getTrimmed(name Name) string {
	raw, err := l.Get(name)
	if err != nil || raw == "" {
		return ""
	}
	budget := name.Budget()
	if budget <= 0 || len(raw) <= budget {
		return raw
	}
	// Log the truncation so the user knows to trim their file.
	slog.Warn("persona truncated", "name", name, "original_len", len(raw), "budget", budget)
	return raw[:budget]
}

// hasBody reports whether a persona file has non-empty content beneath its
// frontmatter. Used to skip empty files during prompt building.
func hasBody(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	// Strip an optional frontmatter block
	if strings.HasPrefix(trimmed, "---") {
		if idx := strings.Index(trimmed[3:], "---"); idx > 0 {
			trimmed = strings.TrimSpace(trimmed[3+idx+3:])
		}
	}
	return trimmed != ""
}
