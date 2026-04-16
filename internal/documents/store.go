package documents

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/stage"
	"github.com/kriswong/corticalstack/internal/vault"
)

const documentsDir = "documents"

// Store is a read/write scanner over the vault/documents/ folder.
// Mirrors the meetings.Store pattern: the vault is the source of
// truth, this type is a thin reader plus a single SetStage write
// path so the dashboard's per-card UI can advance documents through
// the Need → In-Progress → Final flow without leaving the page.
type Store struct {
	vault *vault.Vault
}

// New returns a documents store bound to a vault.
func New(v *vault.Vault) *Store {
	return &Store{vault: v}
}

// EnsureFolder creates vault/documents/ if missing. Failure is
// logged at the call site, not fatal — a fresh vault on a read-only
// mount should still let the dashboard render an empty Documents
// card.
func (s *Store) EnsureFolder() error {
	dir := filepath.Join(s.vault.Path(), documentsDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating documents dir: %w", err)
	}
	return nil
}

// List returns every markdown note under vault/documents/, newest
// first. A missing folder returns an empty slice with no error so
// the dashboard renders gracefully on a fresh vault.
func (s *Store) List() ([]*Document, error) {
	root := filepath.Join(s.vault.Path(), documentsDir)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var out []*Document
	err := filepath.Walk(root, func(fullPath string, info os.FileInfo, err error) error {
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
		rel, relErr := filepath.Rel(s.vault.Path(), fullPath)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		doc, readErr := s.readDocument(relSlash)
		if readErr != nil {
			slog.Warn("documents: skipping note", "path", relSlash, "error", readErr)
			return nil
		}
		out = append(out, doc)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking documents dir: %w", err)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Created.After(out[j].Created)
	})
	return out, nil
}

// Get returns one document by ID. Returns nil + no error when the
// ID is unknown so handlers can reply 404 with a clean message.
func (s *Store) Get(id string) (*Document, error) {
	list, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, d := range list {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, nil
}

// SetStage rewrites the `stage` frontmatter key on the document's
// markdown file. The body is preserved as-is. Returns an error if
// the document is unknown, the stage value is not a valid document
// stage, or the disk write fails.
//
// Mutex-free by design: this is a single-user local app and concurrent
// edits to the same document file are not a concern. If that ever
// changes, lift this to a per-store sync.RWMutex.
func (s *Store) SetStage(id string, target stage.Stage) error {
	if !stage.Validate(stage.EntityDocument, string(target)) {
		return fmt.Errorf("invalid document stage: %q", target)
	}
	doc, err := s.Get(id)
	if err != nil {
		return err
	}
	if doc == nil {
		return fmt.Errorf("document not found: %s", id)
	}
	note, err := s.vault.ReadNote(doc.Path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", doc.Path, err)
	}
	if note.Frontmatter == nil {
		note.Frontmatter = map[string]interface{}{}
	}
	note.Frontmatter["stage"] = string(target)
	note.Frontmatter["updated"] = time.Now().Format(time.RFC3339)
	if err := s.vault.WriteNote(doc.Path, note); err != nil {
		return fmt.Errorf("writing %s: %w", doc.Path, err)
	}
	return nil
}

// Vault exposes the bound vault for callers that need to read raw
// markdown bodies (the document viewer modal in the frontend).
func (s *Store) Vault() *vault.Vault { return s.vault }

func (s *Store) readDocument(relPath string) (*Document, error) {
	note, err := s.vault.ReadNote(relPath)
	if err != nil {
		return nil, err
	}

	d := &Document{
		Path:  relPath,
		Stage: stage.FallbackStage(stage.EntityDocument),
	}
	fm := note.Frontmatter

	if id, ok := fm["id"].(string); ok && id != "" {
		d.ID = id
	} else {
		// Fall back to the slugified relative path so dashboard items
		// always have a stable key even on hand-dropped notes without
		// frontmatter. ToSlash + strip extension keeps the id readable.
		d.ID = strings.TrimSuffix(strings.ReplaceAll(relPath, "/", "-"), ".md")
	}
	if title, ok := fm["title"].(string); ok && title != "" {
		d.Title = title
	} else {
		d.Title = strings.TrimSuffix(filepath.Base(relPath), ".md")
	}
	if raw, ok := fm["stage"].(string); ok {
		d.Stage = stage.Normalize(stage.EntityDocument, raw)
	} else if raw, ok := fm["status"].(string); ok {
		// Tolerate the legacy "status" field name on hand-authored
		// notes. Normalize handles unknowns by returning the fallback.
		d.Stage = stage.Normalize(stage.EntityDocument, raw)
	}
	if src, ok := fm["source"].(string); ok {
		d.Source = src
	}
	if src, ok := fm["source_url"].(string); ok && d.Source == "" {
		d.Source = src
	}
	if tags, ok := fm["tags"].([]interface{}); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok && s != "" {
				d.Tags = append(d.Tags, s)
			}
		}
	}
	if projects, ok := fm["projects"].([]interface{}); ok {
		for _, p := range projects {
			if s, ok := p.(string); ok && s != "" {
				d.Projects = append(d.Projects, s)
			}
		}
	}
	d.Created = parseFrontmatterTime(fm, "created", "ingested", "date")
	d.Updated = parseFrontmatterTime(fm, "updated")

	if d.Created.IsZero() {
		// Fall back to file mtime so timeline ordering still works
		// for hand-dropped notes that lack a `created` frontmatter key.
		abs := filepath.Join(s.vault.Path(), relPath)
		if info, err := os.Stat(abs); err == nil {
			d.Created = info.ModTime()
		}
	}

	return d, nil
}

// parseFrontmatterTime tries each key in priority order and returns
// the first parseable timestamp, accepting both RFC3339 and the
// `2006-01-02` shortcut. Returns the zero time when nothing parses.
func parseFrontmatterTime(fm map[string]interface{}, keys ...string) time.Time {
	for _, k := range keys {
		raw, ok := fm[k]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case time.Time:
			return v
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t
			}
			if t, err := time.Parse("2006-01-02", v); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}
