package workspaces

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

// ErrWorkspaceExists is returned by Create when a workspace with the
// derived slug already exists.
var ErrWorkspaceExists = errors.New("workspace already exists")

// ErrWorkspaceNotFound is returned by Get/Update/Delete when no
// workspace matches the provided id (UUID or slug).
var ErrWorkspaceNotFound = errors.New("workspace not found")

const (
	workspacesFolder = "workspaces"
	manifestName     = "workspace.md"
	trashFolder      = ".trash/workspaces"
)

// Store reads and writes workspaces inside the vault. Same dual-index
// (byUUID + bySlug) pattern as projects.Store and initiatives.Store.
type Store struct {
	vault *vault.Vault

	mu     sync.RWMutex
	byUUID map[string]*Workspace
	bySlug map[string]*Workspace
}

// New creates a store bound to a vault. Call Refresh() to populate.
func New(v *vault.Vault) *Store {
	return &Store{
		vault:  v,
		byUUID: make(map[string]*Workspace),
		bySlug: make(map[string]*Workspace),
	}
}

// EnsureFolder creates vault/workspaces/ if missing.
func (s *Store) EnsureFolder() error {
	root := filepath.Join(s.vault.Path(), workspacesFolder)
	return os.MkdirAll(root, 0o700)
}

// Refresh rescans vault/workspaces/*/workspace.md.
func (s *Store) Refresh() error {
	root := filepath.Join(s.vault.Path(), workspacesFolder)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("ensuring workspaces dir: %w", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("reading workspaces dir: %w", err)
	}

	nextByUUID := make(map[string]*Workspace)
	nextBySlug := make(map[string]*Workspace)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		rel := filepath.Join(workspacesFolder, e.Name(), manifestName)
		if !s.vault.Exists(rel) {
			continue
		}
		w, err := s.loadManifest(rel)
		if err != nil {
			slog.Warn("workspaces: skipping malformed manifest", "path", rel, "error", err)
			continue
		}
		nextByUUID[w.UUID] = w
		nextBySlug[w.Slug] = w
	}

	s.mu.Lock()
	s.byUUID = nextByUUID
	s.bySlug = nextBySlug
	s.mu.Unlock()
	return nil
}

// List returns all known workspaces sorted by name.
func (s *Store) List() []*Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Workspace, 0, len(s.byUUID))
	for _, w := range s.byUUID {
		out = append(out, w)
	}
	sort.Slice(out, func(a, b int) bool {
		return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name)
	})
	return out
}

// Get returns a workspace by UUID or slug, or nil.
func (s *Store) Get(idOrSlug string) *Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if w, ok := s.byUUID[idOrSlug]; ok {
		return w
	}
	return s.bySlug[idOrSlug]
}

// GetByUUID returns the workspace with the given UUID, or nil.
func (s *Store) GetByUUID(u string) *Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byUUID[u]
}

// GetBySlug returns the workspace with the given slug, or nil.
func (s *Store) GetBySlug(slug string) *Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bySlug[slug]
}

// Count returns the number of workspaces currently loaded. Used by the
// frontend's lazy-disclosure check.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byUUID)
}

// Create writes a new workspace manifest.
func (s *Store) Create(req CreateRequest) (*Workspace, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("workspace name required")
	}
	slug := vault.Slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("workspace name produced empty slug")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.bySlug[slug]; exists {
		return nil, fmt.Errorf("%w: %q", ErrWorkspaceExists, slug)
	}

	w := &Workspace{
		UUID:              uuid.NewString(),
		Slug:              slug,
		Name:              name,
		Description:       strings.TrimSpace(req.Description),
		LinearWorkspaceID: strings.TrimSpace(req.LinearWorkspaceID),
		LinearTeamKey:     strings.TrimSpace(req.LinearTeamKey),
		LinearAPIKeyEnv:   strings.TrimSpace(req.LinearAPIKeyEnv),
		Created:           time.Now(),
	}

	if err := s.writeManifest(w); err != nil {
		return nil, err
	}
	s.byUUID[w.UUID] = w
	s.bySlug[w.Slug] = w
	return w, nil
}

// Update applies a partial patch.
func (s *Store) Update(idOrSlug string, req UpdateRequest) (*Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.byUUID[idOrSlug]
	if cur == nil {
		cur = s.bySlug[idOrSlug]
	}
	if cur == nil {
		return nil, ErrWorkspaceNotFound
	}

	oldSlug := cur.Slug
	updated := *cur

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
				return nil, fmt.Errorf("%w: %q", ErrWorkspaceExists, newSlug)
			}
		}
		updated.Name = name
		updated.Slug = newSlug
	}
	if req.Description != nil {
		updated.Description = strings.TrimSpace(*req.Description)
	}
	if req.LinearWorkspaceID != nil {
		updated.LinearWorkspaceID = strings.TrimSpace(*req.LinearWorkspaceID)
	}
	if req.LinearTeamKey != nil {
		updated.LinearTeamKey = strings.TrimSpace(*req.LinearTeamKey)
	}
	if req.LinearAPIKeyEnv != nil {
		updated.LinearAPIKeyEnv = strings.TrimSpace(*req.LinearAPIKeyEnv)
	}

	if updated.Slug != oldSlug {
		oldDir := filepath.Join(s.vault.Path(), workspacesFolder, oldSlug)
		newDir := filepath.Join(s.vault.Path(), workspacesFolder, updated.Slug)
		if err := os.Rename(oldDir, newDir); err != nil {
			return nil, fmt.Errorf("rename workspace dir: %w", err)
		}
	}
	if err := s.writeManifest(&updated); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	if oldSlug != updated.Slug {
		delete(s.bySlug, oldSlug)
	}
	s.byUUID[updated.UUID] = &updated
	s.bySlug[updated.Slug] = &updated
	return &updated, nil
}

// Delete soft-deletes by moving the directory to vault/.trash/workspaces/.
func (s *Store) Delete(idOrSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.byUUID[idOrSlug]
	if cur == nil {
		cur = s.bySlug[idOrSlug]
	}
	if cur == nil {
		return ErrWorkspaceNotFound
	}

	src := filepath.Join(s.vault.Path(), workspacesFolder, cur.Slug)
	trashRoot := filepath.Join(s.vault.Path(), trashFolder)
	if err := os.MkdirAll(trashRoot, 0o700); err != nil {
		return fmt.Errorf("ensuring trash dir: %w", err)
	}
	dst := filepath.Join(trashRoot, fmt.Sprintf("%s-%d", cur.Slug, time.Now().Unix()))
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("move to trash: %w", err)
	}
	delete(s.byUUID, cur.UUID)
	delete(s.bySlug, cur.Slug)
	return nil
}

func (s *Store) loadManifest(relPath string) (*Workspace, error) {
	note, err := s.vault.ReadNote(relPath)
	if err != nil {
		return nil, err
	}
	w := &Workspace{}
	if u, ok := note.Frontmatter["uuid"].(string); ok {
		w.UUID = u
	}
	if id, ok := note.Frontmatter["id"].(string); ok {
		w.Slug = id
	}
	if name, ok := note.Frontmatter["name"].(string); ok {
		w.Name = name
	}
	if desc, ok := note.Frontmatter["description"].(string); ok {
		w.Description = desc
	}
	if v, ok := note.Frontmatter["linear_workspace_id"].(string); ok {
		w.LinearWorkspaceID = v
	}
	if v, ok := note.Frontmatter["linear_team_key"].(string); ok {
		w.LinearTeamKey = v
	}
	if v, ok := note.Frontmatter["linear_api_key_env"].(string); ok {
		w.LinearAPIKeyEnv = v
	}
	if created, ok := note.Frontmatter["created"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			w.Created = t
		} else if t, err := time.Parse("2006-01-02", created); err == nil {
			w.Created = t
		}
	}
	if w.Slug == "" {
		w.Slug = filepath.Base(filepath.Dir(relPath))
	}
	if w.Name == "" {
		w.Name = w.Slug
	}
	if w.UUID == "" {
		w.UUID = uuid.NewString()
	}
	return w, nil
}

func (s *Store) writeManifest(w *Workspace) error {
	fm := map[string]interface{}{
		"uuid": w.UUID,
		"id":   w.Slug,
		"name": w.Name,
	}
	if w.Description != "" {
		fm["description"] = w.Description
	}
	if w.LinearWorkspaceID != "" {
		fm["linear_workspace_id"] = w.LinearWorkspaceID
	}
	if w.LinearTeamKey != "" {
		fm["linear_team_key"] = w.LinearTeamKey
	}
	if w.LinearAPIKeyEnv != "" {
		fm["linear_api_key_env"] = w.LinearAPIKeyEnv
	}
	fm["created"] = w.Created.Format(time.RFC3339)

	rel := filepath.ToSlash(filepath.Join(workspacesFolder, w.Slug, manifestName))
	note := &vault.Note{Frontmatter: fm, Body: composeBody(w)}
	return s.vault.WriteNote(rel, note)
}

func composeBody(w *Workspace) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(w.Name)
	b.WriteString("\n\n")
	if w.Description != "" {
		b.WriteString(w.Description)
		b.WriteString("\n\n")
	}
	b.WriteString("## Linked Projects\n\n> Projects that set `workspace_id: ")
	b.WriteString(w.UUID)
	b.WriteString("` will route their Linear sync through this workspace's API key + team defaults.\n")
	return b.String()
}
