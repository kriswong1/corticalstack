package actions

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReconcileResult summarizes what changed during a reconcile pass.
type ReconcileResult struct {
	Scanned       int      `json:"scanned"`        // files scanned
	LinesMatched  int      `json:"lines_matched"`  // lines carrying a known ID (across all locations)
	UniqueActions int      `json:"unique_actions"`  // distinct action IDs found
	Updated       int      `json:"updated"`        // actions whose status flipped
	Missing       []string `json:"missing,omitempty"` // IDs present in index but not in any file
	Unknown       []string `json:"unknown,omitempty"` // IDs in files that are not in the index
}

// Reconcile scans every known location for action lines and compares
// their status to the index. If a markdown line disagrees with the index
// we take the markdown as authoritative (user likely edited in Obsidian)
// and re-sync every location afterwards.
func (s *Store) Reconcile() (*ReconcileResult, error) {
	res := &ReconcileResult{}

	// Collect every file that might contain action lines. Start with the
	// known set: central tracker + each action's recorded SourceNote +
	// each associated project's ACTION-ITEMS.md.
	files := map[string]bool{}
	files[s.CentralFilePath()] = true

	s.mu.RLock()
	for _, a := range s.byID {
		if a.SourceNote != "" {
			files[a.SourceNote] = true
		}
		for _, pid := range a.ProjectIDs {
			files[s.ProjectFilePath(pid)] = true
		}
	}
	s.mu.RUnlock()

	// LO-07: union in any additional .md file in the vault that carries
	// an action ID marker. This catches source notes the user moved in
	// Obsidian — the stored SourceNote is now stale but the `<!-- id:
	// <uuid> -->` line moved with the file, so we discover the new
	// location and reconcile the user's checkbox edits there.
	//
	// A fast byte-level substring gate (`<!-- id:`) keeps the file set
	// bounded: we only add files that could plausibly contain an action
	// line, not every note in the vault. Reconcile is user-triggered
	// (POST /api/actions/reconcile) so one full-vault walk per click is
	// well within budget for a local app.
	//
	// The SourceNote stored on the Action is NOT auto-rebased to the
	// new location — that's a separate, ambiguous concern (same ID could
	// show up in multiple files after a copy) and left for a future
	// "Rebase source note" action if the need becomes concrete. The
	// next Sync will still write to the stale SourceNote path, which
	// fails silently at that path but writes correctly to the central
	// and project files.
	vaultRoot := s.vault.Path()
	_ = filepath.Walk(vaultRoot, func(fullPath string, info os.FileInfo, err error) error {
		if err != nil {
			// A single unreadable entry shouldn't abort the whole walk.
			// We already had files set from the known paths — just skip.
			return nil
		}
		if info.IsDir() {
			// Skip dot-directories: .obsidian, .cortical, .git, .trash, etc.
			// They may contain files we don't own (plugin state,
			// cortical's own index, git objects) and scanning them for
			// action markers is both pointless and risky.
			if strings.HasPrefix(info.Name(), ".") && fullPath != vaultRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(vaultRoot, fullPath)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if files[rel] {
			return nil // already in the known set
		}
		data, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			return nil
		}
		if bytes.Contains(data, []byte("<!-- id:")) {
			files[rel] = true
		}
		return nil
	})

	// Observed state per action ID. If the same ID is seen in multiple
	// files, the action's canonical SourceNote entry wins over others.
	type observedState struct {
		status   Status
		fromPath string
	}
	observed := make(map[string]observedState)
	knownIDs := make(map[string]bool)

	for path := range files {
		full := filepath.Join(s.vault.Path(), path)
		data, err := os.ReadFile(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			slog.Warn("reconcile: reading action file", "path", full, "error", err)
			continue
		}
		res.Scanned++

		for _, line := range strings.Split(string(data), "\n") {
			if !LineCarriesID(line) {
				continue
			}
			p := ParseLine(line)
			if p == nil {
				continue
			}
			res.LinesMatched++
			knownIDs[p.ID] = true

			// Checkbox overrides tag (ticking [x] means "done" even if tag lags).
			st := p.Status
			if p.Checked && st != StatusDone {
				st = StatusDone
			}

			existing, seen := observed[p.ID]
			if !seen {
				observed[p.ID] = observedState{status: st, fromPath: path}
				continue
			}
			// Prefer the line coming from the action's canonical SourceNote.
			s.mu.RLock()
			a, stored := s.byID[p.ID]
			s.mu.RUnlock()
			if stored && a.SourceNote == path {
				observed[p.ID] = observedState{status: st, fromPath: path}
			} else if existing.fromPath == s.CentralFilePath() {
				observed[p.ID] = observedState{status: st, fromPath: path}
			}
		}
	}

	res.UniqueActions = len(observed)

	// Apply changes to the index where markdown disagreed.
	//
	// DFW-01: snapshot each mutated action's prior state before the
	// mutation so we can roll back if flushLocked fails — same pattern
	// Upsert uses. Without this, a disk-full / permission error leaves
	// the in-memory map holding reconciled statuses while the on-disk
	// index still shows the old ones, so a restart silently reverts
	// every change.
	//
	// MD-09 / DFW-02: snapshot each changed action by VALUE (not just
	// the pointer) so the subsequent Sync loop writes the state that
	// was authoritative at reconcile time. Without this snapshot the
	// Sync loop reads `s.byID[id]` under RLock and picks up whatever
	// another handler just wrote — producing a last-write-wins race.
	// With the snapshot, a concurrent handler update takes effect on
	// the in-memory view but the markdown file reflects the reconciled
	// status (the user's Obsidian edit) until the concurrent handler's
	// own Sync runs, at which point IT wins. Both outcomes are
	// deterministic and visible to the reconcile caller.
	type rollbackEntry struct {
		action      *Action
		prevStatus  Status
		prevUpdated time.Time
	}
	rollbacks := make([]rollbackEntry, 0)
	// Snapshot the action-VALUE (not just pointer) at reconcile time so
	// Sync writes the reconciled state even if another handler mutates
	// the pointer while we're unlocked.
	changedSnapshots := make([]Action, 0)

	s.mu.Lock()
	for id, o := range observed {
		a, ok := s.byID[id]
		if !ok {
			res.Unknown = append(res.Unknown, id)
			continue
		}
		if o.status != a.Status {
			rollbacks = append(rollbacks, rollbackEntry{
				action:      a,
				prevStatus:  a.Status,
				prevUpdated: a.Updated,
			})
			a.Status = o.status
			a.Updated = time.Now()
			res.Updated++
			// Snapshot by VALUE so the Sync loop is not racy against
			// concurrent Upsert/Update on the same pointer.
			changedSnapshots = append(changedSnapshots, *a)
		}
	}
	for id := range s.byID {
		if !knownIDs[id] {
			res.Missing = append(res.Missing, id)
		}
	}
	if err := s.flushLocked(); err != nil {
		// DFW-01: roll back every in-memory mutation so the store's
		// in-RAM view matches on-disk. Restart would otherwise silently
		// revert the user's Obsidian edits.
		for _, rb := range rollbacks {
			rb.action.Status = rb.prevStatus
			rb.action.Updated = rb.prevUpdated
		}
		s.mu.Unlock()
		return nil, fmt.Errorf("writing reconciled index: %w", err)
	}
	s.mu.Unlock()

	// Re-sync the updated actions so every location agrees.
	//
	// We use the VALUE snapshots captured above, not live pointer reads,
	// so this loop is not racy against concurrent Upsert/Update. See
	// the MD-09 / DFW-02 note in the mutation block above.
	for i := range changedSnapshots {
		snapshot := changedSnapshots[i]
		if err := s.Sync(&snapshot); err != nil {
			slog.Warn("reconcile: sync failed", "action_id", snapshot.ID, "error", err)
		}
	}

	return res, nil
}
