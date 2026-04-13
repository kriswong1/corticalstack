package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/vault"
)

const (
	indexDir      = ".cortical"
	indexFile     = "actions.json"
	centralFile   = "ACTION-ITEMS.md"
	projectsDir   = "projects"
	actionsHeader = `---
type: tracker
purpose: Central action item tracker (multi-location sync)
---

# Action Items

> All action items extracted by CorticalStack. Status changes from any location (dashboard, Obsidian) propagate via ` + "`POST /api/actions/reconcile`" + `.

## Open Items

`
)

// Store is the canonical action index backed by a JSON file in the vault.
type Store struct {
	vault *vault.Vault

	mu    sync.RWMutex
	byID  map[string]*Action
}

// New creates an action store bound to a vault.
func New(v *vault.Vault) *Store {
	return &Store{vault: v, byID: make(map[string]*Action)}
}

// Load reads the index from disk, creating an empty one if missing.
func (s *Store) Load() error {
	path := s.indexPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating .cortical dir: %w", err)
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s.flushLocked()
	}
	if err != nil {
		return fmt.Errorf("reading actions index: %w", err)
	}

	var list []*Action
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parsing actions index: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID = make(map[string]*Action, len(list))
	for _, a := range list {
		s.byID[a.ID] = a
	}
	return nil
}

// Save writes the index to disk.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushLocked()
}

func (s *Store) flushLocked() error {
	list := make([]*Action, 0, len(s.byID))
	for _, a := range s.byID {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created.Before(list[j].Created) })

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling actions index: %w", err)
	}
	path := s.indexPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// List returns all known actions, newest first.
func (s *Store) List() []*Action {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Action, 0, len(s.byID))
	for _, a := range s.byID {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// ListByStatus returns actions filtered to a single status.
func (s *Store) ListByStatus(status Status) []*Action {
	all := s.List()
	out := make([]*Action, 0, len(all))
	for _, a := range all {
		if a.Status == status {
			out = append(out, a)
		}
	}
	return out
}

// CountByStatus returns a map of status → count.
func (s *Store) CountByStatus() map[Status]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[Status]int)
	for _, a := range s.byID {
		out[a.Status]++
	}
	return out
}

// Get returns an action by ID, or nil.
func (s *Store) Get(id string) *Action {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byID[id]
}

// Upsert inserts or updates an action. If a.ID is empty a new UUID is assigned.
func (s *Store) Upsert(a *Action) (*Action, error) {
	s.mu.Lock()
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.Created.IsZero() {
		a.Created = time.Now()
	}
	a.Updated = time.Now()
	if a.Status == "" {
		a.Status = StatusInbox
	}
	a.Status = MigrateStatus(a.Status)
	if a.Priority == "" {
		a.Priority = PriorityMedium
	}
	if a.Effort == "" {
		a.Effort = EffortM
	}
	s.byID[a.ID] = a
	err := s.flushLocked()
	s.mu.Unlock()
	return a, err
}

// SetStatus updates the status of an existing action.
func (s *Store) SetStatus(id string, status Status) (*Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("action not found: %s", id)
	}
	if !IsValid(string(status)) {
		return nil, fmt.Errorf("invalid status: %s", status)
	}
	a.Status = status
	a.Updated = time.Now()
	return a, s.flushLocked()
}

// Update applies partial changes to an existing action. Only non-zero fields
// in the patch are applied. Returns the updated action.
func (s *Store) Update(id string, patch ActionPatch) (*Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("action not found: %s", id)
	}
	if patch.Title != nil {
		a.Title = *patch.Title
	}
	if patch.Description != "" {
		a.Description = patch.Description
	}
	if patch.Owner != "" {
		a.Owner = patch.Owner
	}
	if patch.Deadline != nil {
		a.Deadline = *patch.Deadline
	}
	if patch.Status != "" && IsValid(string(patch.Status)) {
		a.Status = MigrateStatus(patch.Status)
	}
	if patch.Priority != "" {
		a.Priority = patch.Priority
	}
	if patch.Effort != "" {
		a.Effort = patch.Effort
	}
	if patch.Context != nil {
		a.Context = *patch.Context
	}
	a.Updated = time.Now()
	return a, s.flushLocked()
}

// ActionPatch holds optional fields for a partial action update.
type ActionPatch struct {
	Title       *string  `json:"title"`
	Description string   `json:"description,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Deadline    *string  `json:"deadline"`
	Status      Status   `json:"status,omitempty"`
	Priority    Priority `json:"priority,omitempty"`
	Effort      Effort   `json:"effort,omitempty"`
	Context     *string  `json:"context"`
}

// CentralFilePath returns the vault-relative path of the central tracker.
func (s *Store) CentralFilePath() string {
	return centralFile
}

// ProjectFilePath returns the vault-relative path for a project's action file.
func (s *Store) ProjectFilePath(projectID string) string {
	return filepath.ToSlash(filepath.Join(projectsDir, projectID, centralFile))
}

// VaultPath exposes the bound vault for sync/reconcile helpers.
func (s *Store) VaultPath() string {
	return s.vault.Path()
}

// Vault returns the bound vault.
func (s *Store) Vault() *vault.Vault {
	return s.vault
}

func (s *Store) indexPath() string {
	return filepath.Join(s.vault.Path(), indexDir, indexFile)
}

// EnsureCentralFile creates the central ACTION-ITEMS.md header if missing.
func (s *Store) EnsureCentralFile() error {
	if s.vault.Exists(centralFile) {
		return nil
	}
	return s.vault.WriteFile(centralFile, actionsHeader)
}
