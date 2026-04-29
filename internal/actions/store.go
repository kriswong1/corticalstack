package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/vault"
)

// ErrWIPLimit is returned when a transition into StatusDoing would exceed
// the configured WIP limit. Handlers should translate this to HTTP 409.
var ErrWIPLimit = errors.New("WIP limit reached")

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

	mu   sync.RWMutex
	byID map[string]*Action

	// syncMu serializes all markdown read-modify-write cycles performed
	// by Sync(). It is intentionally separate from `mu` so that concurrent
	// readers of the in-memory index (List/Get/CountByStatus) don't have
	// to wait for a disk-bound markdown write to complete. Acquired for
	// the full duration of a Sync() call.
	//
	// This is the HI-03 fix: before this mutex existed, two concurrent
	// handlers calling Sync() on different actions that shared a markdown
	// file (e.g. the central ACTION-ITEMS.md or a project tracker) could
	// both read the old file body, both append, and the second writer
	// would clobber the first.
	//
	// We use a single global mutex rather than a per-file map because
	// (a) this is a local-only app with modest write volume, (b) the
	// central ACTION-ITEMS.md is touched by nearly every sync anyway,
	// and (c) the simpler approach has zero risk of map-growth races
	// or forgotten-unlock bugs.
	syncMu sync.Mutex
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
//
// Semantics:
//   - On insert, the supplied pointer is stored and becomes the canonical
//     *Action returned by Get/List.
//   - On update, fields are copied into the EXISTING stored pointer so any
//     caller that previously held a Get() pointer sees the new state.
//     The supplied `a` pointer is NOT stored; callers should use the
//     returned pointer.
//   - Updated is only bumped when a meaningful field actually changed.
//     If the incoming action is byte-for-byte identical to what's already
//     stored (ignoring Created/Updated), Updated is left alone. This
//     protects dashboard stalled-item detection from re-ingest churn.
//   - flushLocked runs BEFORE the map is considered authoritative. On
//     flush failure, the in-memory state is rolled back to the pre-call
//     snapshot so a disk error doesn't leak an unpersisted action.
func (s *Store) Upsert(a *Action) (*Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.Created.IsZero() {
		a.Created = time.Now()
	}
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

	existing, existed := s.byID[a.ID]
	if !existed {
		// Insert path: stamp Updated on creation and store the new pointer.
		// Rollback on flush failure: delete the newly-added entry.
		a.Updated = time.Now()
		s.byID[a.ID] = a
		if err := s.flushLocked(); err != nil {
			delete(s.byID, a.ID)
			return nil, err
		}
		return a, nil
	}

	// Update path: mutate `existing` in place so held pointers stay valid.
	// Snapshot the previous field values so we can rewind on flush failure.
	snapshot := *existing

	// Compute a candidate next state and compare field-by-field against
	// the existing state. If nothing meaningful changed, return the live
	// pointer untouched — Updated is preserved so stalled-item detection
	// isn't clobbered by idempotent re-ingests.
	candidate := *existing
	candidate.Title = a.Title
	candidate.Description = a.Description
	candidate.Owner = a.Owner
	candidate.Deadline = a.Deadline
	candidate.Status = a.Status
	candidate.Priority = a.Priority
	candidate.Effort = a.Effort
	candidate.Context = a.Context
	candidate.SourceNote = a.SourceNote
	candidate.SourceTitle = a.SourceTitle
	candidate.ProjectIDs = a.ProjectIDs

	if actionsEqual(existing, &candidate) {
		// No-op upsert: nothing changed, no flush needed, Updated untouched.
		return existing, nil
	}

	// Apply the candidate and bump Updated.
	*existing = candidate
	existing.Updated = time.Now()

	if err := s.flushLocked(); err != nil {
		// Restore pre-call state so in-memory view doesn't leak the
		// unpersisted update.
		*existing = snapshot
		return nil, err
	}
	return existing, nil
}

// actionsEqual returns true if two actions are equal in every field that
// Upsert considers meaningful. Created and Updated are intentionally
// excluded: Created is monotonic, Updated is the output of the comparison
// itself.
//
// DFW-03: ProjectIDs is compared with length-plus-element-wise equality
// rather than reflect.DeepEqual so a caller who passes `ProjectIDs:
// []string{}` on re-upsert of an action whose stored form was
// `ProjectIDs: nil` (or vice versa) does NOT trip the "field changed"
// branch. Both shapes are semantically "no projects" and an idempotent
// re-ingest should not bump Updated over this distinction.
func actionsEqual(a, b *Action) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.ID != b.ID ||
		a.Title != b.Title ||
		a.Description != b.Description ||
		a.Owner != b.Owner ||
		a.Deadline != b.Deadline ||
		a.Status != b.Status ||
		a.Priority != b.Priority ||
		a.Effort != b.Effort ||
		a.Context != b.Context ||
		a.SourceNote != b.SourceNote ||
		a.SourceTitle != b.SourceTitle {
		return false
	}
	return stringSlicesEqual(a.ProjectIDs, b.ProjectIDs)
}

// stringSlicesEqual compares two string slices treating nil and
// zero-length slices as equivalent. Element order matters.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SetStatus updates the status of an existing action.
//
// Note: this does NOT enforce the WIP limit. Handlers that need to
// enforce the limit atomically should call SetStatusWithLimit instead.
func (s *Store) SetStatus(id string, status Status) (*Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setStatusLocked(id, status)
}

// SetStatusWithLimit atomically enforces the WIP-limit-for-doing and
// updates the action's status in a single critical section. If the
// target status is StatusDoing and the store already contains wipLimit
// actions in StatusDoing, ErrWIPLimit is returned without mutating
// anything. A zero wipLimit disables enforcement.
//
// This closes the TOCTOU window that existed when handlers called
// CountByStatus() and SetStatus() as separate operations.
func (s *Store) SetStatusWithLimit(id string, status Status, wipLimit int) (*Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	target := MigrateStatus(status)
	if target == StatusDoing && wipLimit > 0 {
		// Only enforce the cap if THIS transition would add another
		// action to the "doing" bucket. If the action is already in
		// "doing", the status change is a no-op for the count.
		existing, ok := s.byID[id]
		if !ok {
			return nil, fmt.Errorf("action not found: %s", id)
		}
		if existing.Status != StatusDoing {
			n := 0
			for _, a := range s.byID {
				if a.Status == StatusDoing {
					n++
				}
			}
			if n >= wipLimit {
				return nil, ErrWIPLimit
			}
		}
	}
	return s.setStatusLocked(id, status)
}

// setStatusLocked is the shared implementation. Caller must hold s.mu.
// On flushLocked failure, rolls back the in-memory status change.
func (s *Store) setStatusLocked(id string, status Status) (*Action, error) {
	a, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("action not found: %s", id)
	}
	if !IsValid(string(status)) {
		return nil, fmt.Errorf("invalid status: %s", status)
	}
	prevStatus := a.Status
	prevUpdated := a.Updated
	a.Status = MigrateStatus(status)
	a.Updated = time.Now()
	if err := s.flushLocked(); err != nil {
		a.Status = prevStatus
		a.Updated = prevUpdated
		return nil, err
	}
	return a, nil
}

// Update applies partial changes to an existing action. Only non-zero fields
// in the patch are applied. Returns the updated action.
//
// Note: this does NOT enforce the WIP limit. Handlers that need to
// enforce the limit atomically should call UpdateWithLimit instead.
func (s *Store) Update(id string, patch ActionPatch) (*Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateLocked(id, patch)
}

// UpdateWithLimit atomically enforces the WIP-limit-for-doing when the
// patch contains a status transition into StatusDoing. Otherwise it
// behaves exactly like Update. A zero wipLimit disables enforcement.
func (s *Store) UpdateWithLimit(id string, patch ActionPatch, wipLimit int) (*Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if patch.Status != "" && wipLimit > 0 {
		target := MigrateStatus(patch.Status)
		if target == StatusDoing {
			existing, ok := s.byID[id]
			if !ok {
				return nil, fmt.Errorf("action not found: %s", id)
			}
			if existing.Status != StatusDoing {
				n := 0
				for _, a := range s.byID {
					if a.Status == StatusDoing {
						n++
					}
				}
				if n >= wipLimit {
					return nil, ErrWIPLimit
				}
			}
		}
	}
	return s.updateLocked(id, patch)
}

// updateLocked is the shared implementation. Caller must hold s.mu.
// On flushLocked failure, rolls back every field the patch touched so the
// in-memory view doesn't leak an unpersisted change.
func (s *Store) updateLocked(id string, patch ActionPatch) (*Action, error) {
	a, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("action not found: %s", id)
	}
	snapshot := *a
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
	if patch.LinearIssueID != nil {
		a.LinearIssueID = *patch.LinearIssueID
	}
	a.Updated = time.Now()
	if err := s.flushLocked(); err != nil {
		*a = snapshot
		return nil, err
	}
	return a, nil
}

// ActionPatch holds optional fields for a partial action update.
type ActionPatch struct {
	Title         *string  `json:"title"`
	Description   string   `json:"description,omitempty"`
	Owner         string   `json:"owner,omitempty"`
	Deadline      *string  `json:"deadline"`
	Status        Status   `json:"status,omitempty"`
	Priority      Priority `json:"priority,omitempty"`
	Effort        Effort   `json:"effort,omitempty"`
	Context       *string  `json:"context"`
	LinearIssueID *string  `json:"linear_issue_id,omitempty"` // L4
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
