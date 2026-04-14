package projects

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kriswong/corticalstack/internal/vault"
)

// ErrProjectExists is returned by Create when a project with the derived
// slug id already exists. Callers that want idempotent creation should
// use CreateIfMissing instead of catching this error by string.
var ErrProjectExists = errors.New("project already exists")

const (
	projectsFolder  = "projects"
	manifestName    = "project.md"
	actionItemsName = "ACTION-ITEMS.md"
)

// Store reads and writes projects inside the vault.
type Store struct {
	vault *vault.Vault

	mu    sync.RWMutex
	cache map[string]*Project
}

// New creates a store bound to a vault. Call Refresh() to populate the cache.
func New(v *vault.Vault) *Store {
	return &Store{vault: v, cache: make(map[string]*Project)}
}

// Refresh rescans vault/projects/*/project.md and rebuilds the in-memory cache.
func (s *Store) Refresh() error {
	root := filepath.Join(s.vault.Path(), projectsFolder)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("ensuring projects dir: %w", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("reading projects dir: %w", err)
	}

	next := make(map[string]*Project)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		rel := filepath.Join(projectsFolder, e.Name(), manifestName)
		if !s.vault.Exists(rel) {
			continue
		}
		p, err := s.loadManifest(rel)
		if err != nil {
			// Skip malformed manifests but don't fail the whole refresh.
			continue
		}
		next[p.ID] = p
	}

	s.mu.Lock()
	s.cache = next
	s.mu.Unlock()
	return nil
}

// List returns all known projects sorted by name.
func (s *Store) List() []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Project, 0, len(s.cache))
	for _, p := range s.cache {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Get returns a project by ID or nil if unknown.
func (s *Store) Get(id string) *Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[id]
}

// Create writes a new project manifest and an empty action items file.
// Returns ErrProjectExists if the project id already exists.
//
// Create holds the write lock for the full check-and-write so a race
// between two Create calls produces exactly one success and one
// ErrProjectExists. Callers that want idempotent "create if not
// present" semantics should use CreateIfMissing.
func (s *Store) Create(req CreateRequest) (*Project, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("project name required")
	}
	id := vault.Slugify(name)
	if id == "" {
		return nil, fmt.Errorf("project name produced empty slug")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createLocked(id, name, req)
}

// createLocked is the shared write-under-lock implementation used by both
// Create and CreateIfMissing. Caller must hold s.mu for writing.
func (s *Store) createLocked(id, name string, req CreateRequest) (*Project, error) {
	if _, exists := s.cache[id]; exists {
		return nil, fmt.Errorf("%w: %q", ErrProjectExists, id)
	}

	project := &Project{
		ID:          id,
		Name:        name,
		Status:      StatusActive,
		Description: strings.TrimSpace(req.Description),
		Tags:        req.Tags,
		Created:     time.Now(),
	}

	if err := s.writeManifest(project); err != nil {
		return nil, err
	}
	if err := s.writeEmptyActionItems(project); err != nil {
		return nil, err
	}

	s.cache[id] = project
	return project, nil
}

// CreateIfMissing is the idempotent, race-safe variant of Create used by
// fan-out paths like SyncFromVault and EnsureExists. Returns:
//   - (project, true, nil)  — a new project was created
//   - (project, false, nil) — a project with that id already existed
//   - (nil,     false, err) — slug was invalid or disk write failed
//
// The check-and-create runs under a single write lock so concurrent
// callers for the same project id produce exactly one "created" and one
// "already existed" — never a "both created" or "silent disk error".
// MD-06 / MD-07: replaces the old read-unlock-then-create TOCTOU pattern.
func (s *Store) CreateIfMissing(req CreateRequest) (*Project, bool, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, false, fmt.Errorf("project name required")
	}
	id := vault.Slugify(name)
	if id == "" {
		return nil, false, fmt.Errorf("project name produced empty slug")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.cache[id]; ok {
		return existing, false, nil
	}
	project, err := s.createLocked(id, name, req)
	if err != nil {
		return nil, false, err
	}
	return project, true, nil
}

// EnsureExists creates a project with the given id if it doesn't already exist.
// Used during ingest to auto-create projects referenced in the preview panel.
//
// MD-07: now routes through CreateIfMissing so the existence check and
// the create run under a single lock, and disk-write failures are logged
// at Warn level instead of silently dropped. Still returns no value
// because ingest callers are fire-and-forget — they just want the
// project to exist by the time they look it up.
func (s *Store) EnsureExists(id string) {
	if id == "" {
		return
	}
	// Auto-create with the id as the name; user can rename later.
	if _, _, err := s.CreateIfMissing(CreateRequest{Name: id}); err != nil {
		slog.Warn("projects: auto-create failed",
			"project_id", id, "error", err)
	}
}

// SyncFromVault scans all markdown notes in the vault for frontmatter
// `projects:` entries and ensures each referenced project exists in the store.
// Returns the list of newly created project IDs.
//
// MD-06: now uses CreateIfMissing (single-lock check-and-create) and
// logs disk-write failures at Warn level instead of dropping them. Also
// accepts both `[]interface{}` (yaml-round-trip) and `[]string`
// (in-memory writer) shapes for the `projects:` frontmatter field — see
// MD-04 in aggregator.go for the canonical form of this parse.
func (s *Store) SyncFromVault() ([]string, error) {
	var created []string
	seen := map[string]bool{}

	err := s.vault.Walk(func(relPath string, note *vault.Note) {
		for _, pid := range parseProjectsField(note.Frontmatter) {
			if pid == "" || seen[pid] {
				continue
			}
			seen[pid] = true

			_, wasCreated, err := s.CreateIfMissing(CreateRequest{Name: pid})
			if err != nil {
				slog.Warn("projects: sync-from-vault create failed",
					"project_id", pid, "note", relPath, "error", err)
				continue
			}
			if wasCreated {
				created = append(created, pid)
			}
		}
	})
	return created, err
}

// parseProjectsField returns the `projects:` frontmatter list, accepting
// every shape produced by callers that write the field:
//
//   - []interface{} — yaml.v3's round-trip form (every Walk-based reader)
//   - []string      — in-memory writer shape (route.go etc.)
//   - string        — scalar form for user-hand-written frontmatter
//
// Empty strings and non-string elements are filtered out. Anything else
// (int, map, nil) yields nil.
func parseProjectsField(fm map[string]interface{}) []string {
	raw, ok := fm["projects"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}

// ActionItemsPath returns the relative vault path of a project's action items file.
func (s *Store) ActionItemsPath(id string) string {
	return filepath.ToSlash(filepath.Join(projectsFolder, id, actionItemsName))
}

// ProjectDir returns the relative vault path of a project's directory.
func (s *Store) ProjectDir(id string) string {
	return filepath.ToSlash(filepath.Join(projectsFolder, id))
}

// loadManifest reads and parses vault/projects/<id>/project.md.
func (s *Store) loadManifest(relPath string) (*Project, error) {
	note, err := s.vault.ReadNote(relPath)
	if err != nil {
		return nil, err
	}

	p := &Project{Status: StatusActive}
	if id, ok := note.Frontmatter["id"].(string); ok {
		p.ID = id
	}
	if name, ok := note.Frontmatter["name"].(string); ok {
		p.Name = name
	}
	if status, ok := note.Frontmatter["status"].(string); ok {
		p.Status = Status(status)
	}
	if desc, ok := note.Frontmatter["description"].(string); ok {
		p.Description = desc
	}
	// MD-04: accept both yaml-round-trip ([]interface{}) and in-memory
	// ([]string) shapes plus the scalar one-element form. This matches the
	// dashboard's parseFrontmatterStrings so every reader agrees.
	if tagsRaw, ok := note.Frontmatter["tags"]; ok {
		switch v := tagsRaw.(type) {
		case []interface{}:
			for _, t := range v {
				if s, ok := t.(string); ok && s != "" {
					p.Tags = append(p.Tags, s)
				}
			}
		case []string:
			for _, s := range v {
				if s != "" {
					p.Tags = append(p.Tags, s)
				}
			}
		case string:
			if v != "" {
				p.Tags = append(p.Tags, v)
			}
		}
	}
	if created, ok := note.Frontmatter["created"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, created); err == nil {
			p.Created = parsed
		} else if parsed, err := time.Parse("2006-01-02", created); err == nil {
			p.Created = parsed
		}
	}

	if p.ID == "" {
		// Derive from folder name as a fallback
		dir := filepath.Base(filepath.Dir(relPath))
		p.ID = dir
	}
	if p.Name == "" {
		p.Name = p.ID
	}

	return p, nil
}

func (s *Store) writeManifest(p *Project) error {
	fm := map[string]interface{}{
		"id":     p.ID,
		"name":   p.Name,
		"status": string(p.Status),
	}
	if p.Description != "" {
		fm["description"] = p.Description
	}
	if len(p.Tags) > 0 {
		fm["tags"] = p.Tags
	}
	fm["created"] = p.Created.Format(time.RFC3339)

	var body strings.Builder
	body.WriteString(fmt.Sprintf("# %s\n\n", p.Name))
	if p.Description != "" {
		body.WriteString(p.Description)
		body.WriteString("\n\n")
	}
	body.WriteString("## Notes\n\n> CorticalStack notes tagged with this project will backlink here via frontmatter `projects:`.\n\n")
	body.WriteString("## Action items\n\n> See [[")
	body.WriteString(filepath.ToSlash(filepath.Join(projectsFolder, p.ID, "ACTION-ITEMS")))
	body.WriteString("]].\n")

	note := &vault.Note{Frontmatter: fm, Body: body.String()}
	rel := filepath.ToSlash(filepath.Join(projectsFolder, p.ID, manifestName))
	return s.vault.WriteNote(rel, note)
}

func (s *Store) writeEmptyActionItems(p *Project) error {
	header := fmt.Sprintf(`---
type: tracker
project: %s
purpose: Action items for project %s
---

# %s — Action Items

> Items here are mirrored in the note that created them and in the central vault/ACTION-ITEMS.md.
> Status changes from any location propagate via `+"`POST /api/actions/reconcile`"+`.

## Open Items

`, p.ID, p.Name, p.Name)

	rel := filepath.ToSlash(filepath.Join(projectsFolder, p.ID, actionItemsName))
	return s.vault.WriteFile(rel, header)
}
