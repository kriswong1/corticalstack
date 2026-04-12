package actions

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReconcileResult summarizes what changed during a reconcile pass.
type ReconcileResult struct {
	Scanned      int      `json:"scanned"`       // files scanned
	LinesMatched int      `json:"lines_matched"` // lines carrying a known ID
	Updated      int      `json:"updated"`       // actions whose status flipped
	Missing      []string `json:"missing,omitempty"` // IDs present in index but not in any file
	Unknown      []string `json:"unknown,omitempty"` // IDs in files that are not in the index
}

// Reconcile scans every known location for action lines and compares
// their status to the index. If a markdown line disagrees with the index
// we take the markdown as authoritative (user likely edited in Obsidian)
// and re-sync every location afterwards.
func (s *Store) Reconcile() (*ReconcileResult, error) {
	res := &ReconcileResult{}

	// Collect every file that might contain action lines.
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

	// Apply changes to the index where markdown disagreed.
	changedIDs := make([]string, 0)
	s.mu.Lock()
	for id, o := range observed {
		a, ok := s.byID[id]
		if !ok {
			res.Unknown = append(res.Unknown, id)
			continue
		}
		if o.status != a.Status {
			a.Status = o.status
			a.Updated = time.Now()
			res.Updated++
			changedIDs = append(changedIDs, id)
		}
	}
	for id := range s.byID {
		if !knownIDs[id] {
			res.Missing = append(res.Missing, id)
		}
	}
	if err := s.flushLocked(); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("writing reconciled index: %w", err)
	}
	s.mu.Unlock()

	// Re-sync the updated actions so every location agrees.
	for _, id := range changedIDs {
		s.mu.RLock()
		a := s.byID[id]
		s.mu.RUnlock()
		if a != nil {
			if err := s.Sync(a); err != nil {
				slog.Warn("reconcile: sync failed", "action_id", id, "error", err)
			}
		}
	}

	return res, nil
}
