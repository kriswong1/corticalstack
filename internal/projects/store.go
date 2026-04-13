package projects

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kriswong/corticalstack/internal/vault"
)

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
// Returns ErrExists if the project id already exists.
func (s *Store) Create(req CreateRequest) (*Project, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("project name required")
	}
	id := vault.Slugify(name)
	if id == "" {
		return nil, fmt.Errorf("project name produced empty slug")
	}

	s.mu.RLock()
	_, exists := s.cache[id]
	s.mu.RUnlock()
	if exists {
		return nil, fmt.Errorf("project %q already exists", id)
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

	s.mu.Lock()
	s.cache[id] = project
	s.mu.Unlock()

	return project, nil
}

// EnsureExists creates a project with the given id if it doesn't already exist.
// Used during ingest to auto-create projects referenced in the preview panel.
func (s *Store) EnsureExists(id string) {
	s.mu.RLock()
	_, exists := s.cache[id]
	s.mu.RUnlock()
	if exists {
		return
	}
	// Auto-create with the id as the name; user can rename later.
	s.Create(CreateRequest{Name: id})
}

// SyncFromVault scans all markdown notes in the vault for frontmatter
// `projects:` entries and ensures each referenced project exists in the store.
// Returns the list of newly created project IDs.
func (s *Store) SyncFromVault() ([]string, error) {
	var created []string
	seen := map[string]bool{}

	err := s.vault.Walk(func(relPath string, note *vault.Note) {
		projects, ok := note.Frontmatter["projects"].([]interface{})
		if !ok {
			return
		}
		for _, raw := range projects {
			pid, ok := raw.(string)
			if !ok || pid == "" {
				continue
			}
			if seen[pid] {
				continue
			}
			seen[pid] = true

			s.mu.RLock()
			_, exists := s.cache[pid]
			s.mu.RUnlock()
			if exists {
				continue
			}
			if _, err := s.Create(CreateRequest{Name: pid}); err == nil {
				created = append(created, pid)
			}
		}
	})
	return created, err
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
	if tags, ok := note.Frontmatter["tags"].([]interface{}); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				p.Tags = append(p.Tags, s)
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
