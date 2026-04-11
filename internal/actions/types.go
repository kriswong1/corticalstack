// Package actions is the smart action-item store backing CorticalStack's
// multi-location action tracking. Actions are written to the source note,
// every associated project's ACTION-ITEMS.md, and the central tracker,
// each carrying a stable ID. A JSON index at $VAULT_PATH/.cortical/actions.json
// is the canonical state; reconcile operations sync the index with
// whatever markdown edits the user made by hand in Obsidian.
package actions

import "time"

// Status is the state of an action. Multi-state so the user can
// distinguish "seen but not started" from "actively being worked on".
type Status string

const (
	StatusPending   Status = "pending"    // extracted, not yet acknowledged
	StatusAck       Status = "ack"        // user has seen it
	StatusDoing     Status = "doing"      // actively in progress
	StatusDone      Status = "done"       // completed
	StatusDeferred  Status = "deferred"   // pushed back
	StatusCancelled Status = "cancelled"  // no longer relevant
)

// AllStatuses returns the full list in a stable order.
func AllStatuses() []Status {
	return []Status{StatusPending, StatusAck, StatusDoing, StatusDone, StatusDeferred, StatusCancelled}
}

// IsValid reports whether s is a supported status.
func IsValid(s string) bool {
	for _, v := range AllStatuses() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// Action is one tracked item across the three locations.
type Action struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Owner       string    `json:"owner"`
	Deadline    string    `json:"deadline,omitempty"`
	Status      Status    `json:"status"`
	SourceNote  string    `json:"source_note"` // e.g., "notes/2026-04-11_slug.md"
	SourceTitle string    `json:"source_title,omitempty"`
	ProjectIDs  []string  `json:"project_ids,omitempty"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
}

// Locations returns the relative vault paths this action should appear in.
// centralFile is the global tracker path; projectFileFn maps a project ID
// to that project's tracker path.
func (a *Action) Locations(centralFile string, projectFileFn func(string) string) []string {
	paths := []string{centralFile}
	if a.SourceNote != "" {
		paths = append(paths, a.SourceNote)
	}
	for _, id := range a.ProjectIDs {
		paths = append(paths, projectFileFn(id))
	}
	return paths
}
