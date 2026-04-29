package initiatives

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

// ErrInitiativeExists is returned by Create when an initiative with
// the derived slug already exists.
var ErrInitiativeExists = errors.New("initiative already exists")

// ErrInitiativeNotFound is returned by Get/Update/Delete when no
// initiative matches the provided id (UUID or slug).
var ErrInitiativeNotFound = errors.New("initiative not found")

const (
	initiativesFolder = "initiatives"
	manifestName      = "initiative.md"

	// trashFolder mirrors projects: dotfile parent so Refresh's walker
	// (which skips dotfiles) doesn't mistake it for an initiative.
	trashFolder = ".trash/initiatives"
)

// Store reads and writes initiatives inside the vault. Same dual-index
// (byUUID + bySlug) as projects.Store; see internal/projects/store.go
// for the design rationale.
type Store struct {
	vault *vault.Vault

	mu     sync.RWMutex
	byUUID map[string]*Initiative
	bySlug map[string]*Initiative
}

// New creates a store bound to a vault. Call Refresh() to populate.
func New(v *vault.Vault) *Store {
	return &Store{
		vault:  v,
		byUUID: make(map[string]*Initiative),
		bySlug: make(map[string]*Initiative),
	}
}

// EnsureFolder creates vault/initiatives/ if missing. Cheap to call.
func (s *Store) EnsureFolder() error {
	root := filepath.Join(s.vault.Path(), initiativesFolder)
	return os.MkdirAll(root, 0o700)
}

// Refresh rescans vault/initiatives/*/initiative.md and rebuilds the cache.
func (s *Store) Refresh() error {
	root := filepath.Join(s.vault.Path(), initiativesFolder)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("ensuring initiatives dir: %w", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("reading initiatives dir: %w", err)
	}

	nextByUUID := make(map[string]*Initiative)
	nextBySlug := make(map[string]*Initiative)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		rel := filepath.Join(initiativesFolder, e.Name(), manifestName)
		if !s.vault.Exists(rel) {
			continue
		}
		i, err := s.loadManifest(rel)
		if err != nil {
			slog.Warn("initiatives: skipping malformed manifest", "path", rel, "error", err)
			continue
		}
		nextByUUID[i.UUID] = i
		nextBySlug[i.Slug] = i
	}

	s.mu.Lock()
	s.byUUID = nextByUUID
	s.bySlug = nextBySlug
	s.mu.Unlock()
	return nil
}

// List returns all known initiatives sorted by name.
func (s *Store) List() []*Initiative {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Initiative, 0, len(s.byUUID))
	for _, i := range s.byUUID {
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool {
		return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name)
	})
	return out
}

// Get returns an initiative by UUID or slug, or nil if unknown.
func (s *Store) Get(idOrSlug string) *Initiative {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i, ok := s.byUUID[idOrSlug]; ok {
		return i
	}
	return s.bySlug[idOrSlug]
}

// GetByUUID returns the initiative with the given UUID, or nil.
func (s *Store) GetByUUID(u string) *Initiative {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byUUID[u]
}

// GetBySlug returns the initiative with the given slug, or nil.
func (s *Store) GetBySlug(slug string) *Initiative {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bySlug[slug]
}

// Count returns the number of initiatives currently loaded. Used by the
// frontend's lazy-disclosure check: zero = hide the sidebar group.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byUUID)
}

// Create writes a new initiative manifest.
func (s *Store) Create(req CreateRequest) (*Initiative, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("initiative name required")
	}
	slug := vault.Slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("initiative name produced empty slug")
	}

	target, err := parseDateOptional(req.TargetDate)
	if err != nil {
		return nil, fmt.Errorf("target_date: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.bySlug[slug]; exists {
		return nil, fmt.Errorf("%w: %q", ErrInitiativeExists, slug)
	}

	i := &Initiative{
		UUID:               uuid.NewString(),
		Slug:               slug,
		Name:               name,
		Status:             StatusActive,
		Description:        strings.TrimSpace(req.Description),
		Owner:              strings.TrimSpace(req.Owner),
		TargetDate:         target,
		ParentInitiativeID: req.ParentInitiativeID,
		TeamID:             req.TeamID,
		Created:            time.Now(),
	}

	if err := s.writeManifest(i); err != nil {
		return nil, err
	}
	s.byUUID[i.UUID] = i
	s.bySlug[i.Slug] = i
	return i, nil
}

// Update applies a partial patch. Name change triggers slug regen +
// directory rename; UUID stays stable so cross-references survive.
func (s *Store) Update(idOrSlug string, req UpdateRequest) (*Initiative, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.byUUID[idOrSlug]
	if cur == nil {
		cur = s.bySlug[idOrSlug]
	}
	if cur == nil {
		return nil, ErrInitiativeNotFound
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
				return nil, fmt.Errorf("%w: %q", ErrInitiativeExists, newSlug)
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
	if req.Owner != nil {
		updated.Owner = strings.TrimSpace(*req.Owner)
	}
	if req.TargetDate != nil {
		// Empty string clears the date.
		if strings.TrimSpace(*req.TargetDate) == "" {
			updated.TargetDate = nil
		} else {
			t, err := parseDateOptional(*req.TargetDate)
			if err != nil {
				return nil, fmt.Errorf("target_date: %w", err)
			}
			updated.TargetDate = t
		}
	}
	if req.ParentInitiativeID != nil {
		// Empty pointer-to-empty-string clears it.
		if strings.TrimSpace(*req.ParentInitiativeID) == "" {
			updated.ParentInitiativeID = nil
		} else {
			updated.ParentInitiativeID = req.ParentInitiativeID
		}
	}
	if req.TeamID != nil {
		if strings.TrimSpace(*req.TeamID) == "" {
			updated.TeamID = nil
		} else {
			updated.TeamID = req.TeamID
		}
	}
	if req.TeamKey != nil {
		if strings.TrimSpace(*req.TeamKey) == "" {
			updated.TeamKey = nil
		} else {
			v := *req.TeamKey
			updated.TeamKey = &v
		}
	}

	if updated.Slug != oldSlug {
		oldDir := filepath.Join(s.vault.Path(), initiativesFolder, oldSlug)
		newDir := filepath.Join(s.vault.Path(), initiativesFolder, updated.Slug)
		if err := os.Rename(oldDir, newDir); err != nil {
			return nil, fmt.Errorf("rename initiative dir: %w", err)
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

// Delete soft-deletes by moving the directory to vault/.trash/initiatives/
// with a timestamp suffix.
func (s *Store) Delete(idOrSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.byUUID[idOrSlug]
	if cur == nil {
		cur = s.bySlug[idOrSlug]
	}
	if cur == nil {
		return ErrInitiativeNotFound
	}

	src := filepath.Join(s.vault.Path(), initiativesFolder, cur.Slug)
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

// SetLinearID stamps the Linear initiative ID after a successful sync.
// Persisted to the manifest frontmatter so subsequent runs reconcile.
func (s *Store) SetLinearID(idOrSlug, linearID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.byUUID[idOrSlug]
	if cur == nil {
		cur = s.bySlug[idOrSlug]
	}
	if cur == nil {
		return ErrInitiativeNotFound
	}
	updated := *cur
	updated.LinearID = linearID
	if err := s.writeManifest(&updated); err != nil {
		return err
	}
	s.byUUID[updated.UUID] = &updated
	s.bySlug[updated.Slug] = &updated
	return nil
}

// loadManifest reads and parses vault/initiatives/<slug>/initiative.md.
func (s *Store) loadManifest(relPath string) (*Initiative, error) {
	note, err := s.vault.ReadNote(relPath)
	if err != nil {
		return nil, err
	}
	i := &Initiative{Status: StatusActive}
	if u, ok := note.Frontmatter["uuid"].(string); ok {
		i.UUID = u
	}
	if id, ok := note.Frontmatter["id"].(string); ok {
		i.Slug = id
	}
	if name, ok := note.Frontmatter["name"].(string); ok {
		i.Name = name
	}
	if status, ok := note.Frontmatter["status"].(string); ok {
		i.Status = Status(status)
	}
	if desc, ok := note.Frontmatter["description"].(string); ok {
		i.Description = desc
	}
	if owner, ok := note.Frontmatter["owner"].(string); ok {
		i.Owner = owner
	}
	if td, ok := note.Frontmatter["target_date"].(string); ok && td != "" {
		if t, err := parseDateOptional(td); err == nil {
			i.TargetDate = t
		}
	}
	if parent, ok := note.Frontmatter["parent_initiative_id"].(string); ok && parent != "" {
		i.ParentInitiativeID = &parent
	}
	if team, ok := note.Frontmatter["team_id"].(string); ok && team != "" {
		i.TeamID = &team
	}
	if tk, ok := note.Frontmatter["team_key"].(string); ok && tk != "" {
		i.TeamKey = &tk
	}
	if lid, ok := note.Frontmatter["linear_id"].(string); ok {
		i.LinearID = lid
	}
	if created, ok := note.Frontmatter["created"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			i.Created = t
		} else if t, err := time.Parse("2006-01-02", created); err == nil {
			i.Created = t
		}
	}

	if i.Slug == "" {
		i.Slug = filepath.Base(filepath.Dir(relPath))
	}
	if i.Name == "" {
		i.Name = i.Slug
	}
	if i.UUID == "" {
		i.UUID = uuid.NewString()
	}
	return i, nil
}

// writeManifest writes the manifest. Body is deterministic — initiatives
// don't have a Canvas section, just a header + description + linked
// projects backref note.
func (s *Store) writeManifest(i *Initiative) error {
	fm := map[string]interface{}{
		"uuid":   i.UUID,
		"id":     i.Slug,
		"name":   i.Name,
		"status": string(i.Status),
	}
	if i.Description != "" {
		fm["description"] = i.Description
	}
	if i.Owner != "" {
		fm["owner"] = i.Owner
	}
	if i.TargetDate != nil {
		fm["target_date"] = i.TargetDate.Format(time.RFC3339)
	}
	if i.ParentInitiativeID != nil && *i.ParentInitiativeID != "" {
		fm["parent_initiative_id"] = *i.ParentInitiativeID
	}
	if i.TeamID != nil && *i.TeamID != "" {
		fm["team_id"] = *i.TeamID
	}
	if i.TeamKey != nil && *i.TeamKey != "" {
		fm["team_key"] = *i.TeamKey
	}
	if i.LinearID != "" {
		fm["linear_id"] = i.LinearID
	}
	fm["created"] = i.Created.Format(time.RFC3339)

	rel := filepath.ToSlash(filepath.Join(initiativesFolder, i.Slug, manifestName))
	note := &vault.Note{Frontmatter: fm, Body: composeBody(i)}
	return s.vault.WriteNote(rel, note)
}

// InitiativeDir returns the relative vault path of an initiative's directory.
func (s *Store) InitiativeDir(idOrSlug string) string {
	s.mu.RLock()
	cur := s.byUUID[idOrSlug]
	if cur == nil {
		cur = s.bySlug[idOrSlug]
	}
	s.mu.RUnlock()
	slug := idOrSlug
	if cur != nil {
		slug = cur.Slug
	}
	return filepath.ToSlash(filepath.Join(initiativesFolder, slug))
}

// composeBody renders the initiative manifest body.
func composeBody(i *Initiative) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(i.Name)
	b.WriteString("\n\n")
	if i.Description != "" {
		b.WriteString(i.Description)
		b.WriteString("\n\n")
	}
	b.WriteString("## Linked Projects\n\n> Projects that set `initiative_id: ")
	b.WriteString(i.UUID)
	b.WriteString("` will backlink here.\n")
	return b.String()
}

// parseDateOptional accepts RFC3339 ("2026-04-28T00:00:00Z"), RFC3339
// without timezone, or YYYY-MM-DD. Empty input → nil pointer (clear).
func parseDateOptional(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t, nil
	}
	return nil, fmt.Errorf("unrecognized date %q (want RFC3339 or YYYY-MM-DD)", s)
}
