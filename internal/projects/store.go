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

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/vault"
)

// ErrProjectExists is returned by Create when a project with the derived
// slug already exists. Callers that want idempotent creation should
// use CreateIfMissing instead of catching this error by string.
var ErrProjectExists = errors.New("project already exists")

// ErrProjectNotFound is returned by Get/Update/Delete when no project
// matches the provided id (UUID or slug).
var ErrProjectNotFound = errors.New("project not found")

const (
	projectsFolder  = "projects"
	manifestName    = "project.md"
	actionItemsName = "ACTION-ITEMS.md"

	// trashFolder holds soft-deleted projects. Sibling of projectsFolder
	// so a `.trash` directory under projects/ doesn't get mistaken for a
	// project on Refresh (we explicitly skip dotfiles in the walker).
	trashFolder = ".trash/projects"
)

// Store reads and writes projects inside the vault.
//
// The store keeps two indexes over the same set of projects: byUUID is
// the canonical lookup, bySlug is a convenience for filesystem and
// hand-written-manifest lookups. Both maps point to the same *Project
// values — never duplicate.
type Store struct {
	vault *vault.Vault

	mu     sync.RWMutex
	byUUID map[string]*Project
	bySlug map[string]*Project
}

// New creates a store bound to a vault. Call Refresh() to populate the cache.
func New(v *vault.Vault) *Store {
	return &Store{
		vault:  v,
		byUUID: make(map[string]*Project),
		bySlug: make(map[string]*Project),
	}
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

	nextByUUID := make(map[string]*Project)
	nextBySlug := make(map[string]*Project)
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
		nextByUUID[p.UUID] = p
		nextBySlug[p.Slug] = p
	}

	s.mu.Lock()
	s.byUUID = nextByUUID
	s.bySlug = nextBySlug
	s.mu.Unlock()
	return nil
}

// List returns all known projects sorted by name.
func (s *Store) List() []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Project, 0, len(s.byUUID))
	for _, p := range s.byUUID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Get returns a project by UUID or slug, or nil if unknown. Accepts either
// form so HTTP routes (chi.URLParam "id") can match without the caller
// knowing which shape they have.
func (s *Store) Get(idOrSlug string) *Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.byUUID[idOrSlug]; ok {
		return p
	}
	return s.bySlug[idOrSlug]
}

// GetByUUID returns the project with the given UUID, or nil.
func (s *Store) GetByUUID(u string) *Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byUUID[u]
}

// GetBySlug returns the project with the given slug, or nil.
func (s *Store) GetBySlug(slug string) *Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bySlug[slug]
}

// KnownUUIDs returns the set of UUIDs currently in the cache. Used by
// CanonicalizeProjectIDs to drop dangling references on write.
func (s *Store) KnownUUIDs() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]bool, len(s.byUUID))
	for u := range s.byUUID {
		out[u] = true
	}
	return out
}

// Create writes a new project manifest and an empty action items file.
// Returns ErrProjectExists if a project with the derived slug already
// exists. The newly-minted Project carries a fresh UUID — callers should
// reference projects by UUID rather than slug going forward.
func (s *Store) Create(req CreateRequest) (*Project, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("project name required")
	}
	slug := vault.Slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("project name produced empty slug")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createLocked(slug, name, req)
}

// createLocked is the shared write-under-lock implementation used by both
// Create and CreateIfMissing. Caller must hold s.mu for writing.
func (s *Store) createLocked(slug, name string, req CreateRequest) (*Project, error) {
	if _, exists := s.bySlug[slug]; exists {
		return nil, fmt.Errorf("%w: %q", ErrProjectExists, slug)
	}

	project := &Project{
		UUID:        uuid.NewString(),
		Slug:        slug,
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

	s.byUUID[project.UUID] = project
	s.bySlug[project.Slug] = project
	return project, nil
}

// CreateIfMissing is the idempotent, race-safe variant of Create used by
// fan-out paths like SyncFromVault. Returns:
//   - (project, true, nil)  — a new project was created
//   - (project, false, nil) — a project with that slug already existed
//   - (nil,     false, err) — slug was invalid or disk write failed
//
// The check-and-create runs under a single write lock so concurrent
// callers for the same project slug produce exactly one "created" and one
// "already existed" — never a "both created" or "silent disk error".
func (s *Store) CreateIfMissing(req CreateRequest) (*Project, bool, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, false, fmt.Errorf("project name required")
	}
	slug := vault.Slugify(name)
	if slug == "" {
		return nil, false, fmt.Errorf("project name produced empty slug")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.bySlug[slug]; ok {
		return existing, false, nil
	}
	project, err := s.createLocked(slug, name, req)
	if err != nil {
		return nil, false, err
	}
	return project, true, nil
}

// EnsureExists creates a project with the given id (slug-form name) if it
// doesn't already exist. Used by SyncFromVault to backfill projects
// referenced in note frontmatter.
//
// DEPRECATED: ingest no longer auto-creates projects via this path — Phase 4
// removed the silent-create behavior in favor of an explicit "Create new
// project «foo»?" preview affordance. SyncFromVault still uses this for
// hand-edited frontmatter, but the classifier and runConfirm path do not.
func (s *Store) EnsureExists(id string) {
	if id == "" {
		return
	}
	if _, _, err := s.CreateIfMissing(CreateRequest{Name: id}); err != nil {
		slog.Warn("projects: auto-create failed",
			"project_id", id, "error", err)
	}
}

// Update applies a partial patch to an existing project. If req.Name is
// supplied and changes, the directory is renamed (slug regenerates) but
// the UUID stays stable so cross-references survive. Returns the updated
// project or ErrProjectNotFound.
func (s *Store) Update(idOrSlug string, req UpdateRequest) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := s.byUUID[idOrSlug]
	if p == nil {
		p = s.bySlug[idOrSlug]
	}
	if p == nil {
		return nil, ErrProjectNotFound
	}

	oldSlug := p.Slug
	updated := *p // copy so a mid-write failure leaves the cache untouched

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		newSlug := vault.Slugify(name)
		if newSlug == "" {
			return nil, fmt.Errorf("name produced empty slug")
		}
		if newSlug != oldSlug {
			if _, taken := s.bySlug[newSlug]; taken {
				return nil, fmt.Errorf("%w: %q", ErrProjectExists, newSlug)
			}
		}
		updated.Name = name
		updated.Slug = newSlug
	}
	if req.Description != nil {
		updated.Description = strings.TrimSpace(*req.Description)
	}
	if req.Status != nil {
		updated.Status = *req.Status
	}
	if req.Tags != nil {
		updated.Tags = *req.Tags
	}

	// Rename directory before writing the new manifest so we don't end up
	// with a half-renamed state if the write fails.
	if updated.Slug != oldSlug {
		oldDir := filepath.Join(s.vault.Path(), projectsFolder, oldSlug)
		newDir := filepath.Join(s.vault.Path(), projectsFolder, updated.Slug)
		if err := os.Rename(oldDir, newDir); err != nil {
			return nil, fmt.Errorf("rename project dir: %w", err)
		}
	}

	if err := s.writeManifest(&updated); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	// Rewrite action-items header (carries the project name).
	if err := s.writeEmptyActionItemsIfMissing(&updated); err != nil {
		slog.Warn("projects: action-items header refresh failed",
			"uuid", updated.UUID, "error", err)
	}

	if oldSlug != updated.Slug {
		delete(s.bySlug, oldSlug)
	}
	s.byUUID[updated.UUID] = &updated
	s.bySlug[updated.Slug] = &updated
	return &updated, nil
}

// Delete soft-deletes a project by moving its directory to vault/.trash/projects/
// with a timestamp suffix. Returns ErrProjectNotFound if no match.
//
// Referencing notes are left alone — their `projects:` arrays keep the
// dangling UUID. CanonicalizeProjectIDs drops unknown UUIDs at next write,
// and restoring the project (move folder back, Refresh) reanimates them.
func (s *Store) Delete(idOrSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := s.byUUID[idOrSlug]
	if p == nil {
		p = s.bySlug[idOrSlug]
	}
	if p == nil {
		return ErrProjectNotFound
	}

	src := filepath.Join(s.vault.Path(), projectsFolder, p.Slug)
	trashRoot := filepath.Join(s.vault.Path(), trashFolder)
	if err := os.MkdirAll(trashRoot, 0o700); err != nil {
		return fmt.Errorf("ensuring trash dir: %w", err)
	}
	dst := filepath.Join(trashRoot, fmt.Sprintf("%s-%d", p.Slug, time.Now().Unix()))
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("move to trash: %w", err)
	}

	delete(s.byUUID, p.UUID)
	delete(s.bySlug, p.Slug)
	return nil
}

// SyncFromVault scans all markdown notes in the vault for frontmatter
// `projects:` entries. After Phase 1 migration the values are UUIDs; pre-
// migration values may be slugs. Either form auto-creates a project (slug
// referenced) or is left alone (UUID already known).
//
// Returns the list of newly created project slugs.
func (s *Store) SyncFromVault() ([]string, error) {
	var created []string
	seen := map[string]bool{}

	err := s.vault.Walk(func(relPath string, note *vault.Note) {
		for _, ref := range parseProjectsField(note.Frontmatter) {
			if ref == "" || seen[ref] {
				continue
			}
			seen[ref] = true

			// If the reference is a known UUID or slug, nothing to do.
			if s.Get(ref) != nil {
				continue
			}
			// Unknown reference. If it parses as a UUID, leave it alone —
			// it's likely a dangling reference to a deleted project, and
			// we don't want SyncFromVault to resurrect deleted projects.
			if _, err := uuid.Parse(ref); err == nil {
				continue
			}

			// Looks like a slug. Backfill the project.
			_, wasCreated, err := s.CreateIfMissing(CreateRequest{Name: ref})
			if err != nil {
				slog.Warn("projects: sync-from-vault create failed",
					"project_ref", ref, "note", relPath, "error", err)
				continue
			}
			if wasCreated {
				created = append(created, ref)
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
// Lookup accepts UUID or slug.
func (s *Store) ActionItemsPath(idOrSlug string) string {
	s.mu.RLock()
	p := s.byUUID[idOrSlug]
	if p == nil {
		p = s.bySlug[idOrSlug]
	}
	s.mu.RUnlock()
	slug := idOrSlug
	if p != nil {
		slug = p.Slug
	}
	return filepath.ToSlash(filepath.Join(projectsFolder, slug, actionItemsName))
}

// ProjectDir returns the relative vault path of a project's directory.
// Lookup accepts UUID or slug.
func (s *Store) ProjectDir(idOrSlug string) string {
	s.mu.RLock()
	p := s.byUUID[idOrSlug]
	if p == nil {
		p = s.bySlug[idOrSlug]
	}
	s.mu.RUnlock()
	slug := idOrSlug
	if p != nil {
		slug = p.Slug
	}
	return filepath.ToSlash(filepath.Join(projectsFolder, slug))
}

// loadManifest reads and parses vault/projects/<slug>/project.md.
// Generates a UUID if the manifest lacks one (legacy pre-Phase-1 manifests);
// the migrator persists the freshly-generated value on its next pass.
func (s *Store) loadManifest(relPath string) (*Project, error) {
	note, err := s.vault.ReadNote(relPath)
	if err != nil {
		return nil, err
	}

	p := &Project{Status: StatusActive}
	if u, ok := note.Frontmatter["uuid"].(string); ok {
		p.UUID = u
	}
	if id, ok := note.Frontmatter["id"].(string); ok {
		p.Slug = id
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

	// Slug fallback: derive from folder name if not in frontmatter.
	if p.Slug == "" {
		p.Slug = filepath.Base(filepath.Dir(relPath))
	}
	if p.Name == "" {
		p.Name = p.Slug
	}
	// UUID fallback: mint one for legacy manifests. The migrator persists
	// this on its next pass; until then the in-memory copy carries it.
	if p.UUID == "" {
		p.UUID = uuid.NewString()
	}

	return p, nil
}

// writeManifest writes the project's manifest in split-mode: deterministic
// header + frontmatter, user-editable `## Canvas` section preserved
// across writes, deterministic footer regenerated each time. The canvas
// is read off the existing manifest body (if any) and round-tripped.
func (s *Store) writeManifest(p *Project) error {
	fm := map[string]interface{}{
		"uuid":   p.UUID,
		"id":     p.Slug,
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

	rel := filepath.ToSlash(filepath.Join(projectsFolder, p.Slug, manifestName))

	// Preserve any existing canvas content. ReadNote returns an error
	// for missing manifests (initial create); we treat that as empty
	// canvas and let composeBody emit just the heading.
	var canvas string
	if existing, err := s.vault.ReadNote(rel); err == nil {
		canvas = extractCanvas(existing.Body)
	}

	note := &vault.Note{Frontmatter: fm, Body: composeBody(p, canvas)}
	return s.vault.WriteNote(rel, note)
}

// Canvas returns the user-editable canvas text for a project, or "" if
// the manifest has no canvas section yet. Lookup accepts UUID or slug.
func (s *Store) Canvas(idOrSlug string) (string, error) {
	p := s.Get(idOrSlug)
	if p == nil {
		return "", ErrProjectNotFound
	}
	rel := filepath.ToSlash(filepath.Join(projectsFolder, p.Slug, manifestName))
	note, err := s.vault.ReadNote(rel)
	if err != nil {
		return "", err
	}
	return extractCanvas(note.Body), nil
}

// SetCanvas writes new canvas content into the project's manifest. The
// rest of the body (header, footer) is recomposed from the project's
// current state. Returns ErrProjectNotFound if no match.
func (s *Store) SetCanvas(idOrSlug, canvas string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.byUUID[idOrSlug]
	if p == nil {
		p = s.bySlug[idOrSlug]
	}
	if p == nil {
		return ErrProjectNotFound
	}

	fm := map[string]interface{}{
		"uuid":   p.UUID,
		"id":     p.Slug,
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

	rel := filepath.ToSlash(filepath.Join(projectsFolder, p.Slug, manifestName))
	note := &vault.Note{Frontmatter: fm, Body: composeBody(p, canvas)}
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

`, p.Slug, p.Name, p.Name)

	rel := filepath.ToSlash(filepath.Join(projectsFolder, p.Slug, actionItemsName))
	return s.vault.WriteFile(rel, header)
}

// writeEmptyActionItemsIfMissing only writes the header file if it doesn't
// already exist. Used after a rename so we don't clobber a populated tracker.
func (s *Store) writeEmptyActionItemsIfMissing(p *Project) error {
	rel := filepath.ToSlash(filepath.Join(projectsFolder, p.Slug, actionItemsName))
	if s.vault.Exists(rel) {
		return nil
	}
	return s.writeEmptyActionItems(p)
}
