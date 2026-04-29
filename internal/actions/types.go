// Package actions is the smart action-item store backing CorticalStack's
// multi-location action tracking. Actions are written to the source note,
// every associated project's ACTION-ITEMS.md, and the central tracker,
// each carrying a stable ID. A JSON index at $VAULT_PATH/.cortical/actions.json
// is the canonical state; reconcile operations sync the index with
// whatever markdown edits the user made by hand in Obsidian.
package actions

import "time"

// Status is the state of an action. GTD-inspired lifecycle with
// WIP-limited "doing" and explicit "waiting" and "someday" states.
type Status string

const (
	StatusInbox     Status = "inbox"      // extracted, needs triage
	StatusNext      Status = "next"       // triaged as actionable, ready to pick up
	StatusWaiting   Status = "waiting"    // blocked on external input
	StatusDoing     Status = "doing"      // actively in progress (WIP-limited)
	StatusSomeday   Status = "someday"    // interesting but no commitment to timing
	StatusDeferred  Status = "deferred"   // deliberately postponed to a specific date
	StatusDone      Status = "done"       // completed
	StatusCancelled Status = "cancelled"  // no longer relevant

	// Legacy aliases — kept so external packages and tests compile.
	// MigrateStatus() converts these to their new equivalents at runtime.
	StatusPending Status = "pending"
	StatusAck     Status = "ack"
)

// AllStatuses returns the full list in a stable order.
func AllStatuses() []Status {
	return []Status{StatusInbox, StatusNext, StatusWaiting, StatusDoing, StatusSomeday, StatusDeferred, StatusDone, StatusCancelled}
}

// IsValid reports whether s is a supported status.
func IsValid(s string) bool {
	// Accept legacy statuses for migration compatibility.
	if s == "pending" || s == "ack" {
		return true
	}
	for _, v := range AllStatuses() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// MigrateStatus converts legacy statuses to their new equivalents.
func MigrateStatus(s Status) Status {
	switch s {
	case "pending":
		return StatusInbox
	case "ack":
		return StatusNext
	default:
		return s
	}
}

// Priority is a 3-level priority for action triage.
type Priority string

const (
	PriorityHigh   Priority = "p1" // address this week
	PriorityMedium Priority = "p2" // schedule when p1 is clear
	PriorityLow    Priority = "p3" // nice-to-have
)

// AllPriorities returns all priorities in descending importance.
func AllPriorities() []Priority {
	return []Priority{PriorityHigh, PriorityMedium, PriorityLow}
}

// Effort is a t-shirt size for rough estimation.
type Effort string

const (
	EffortXS Effort = "xs"
	EffortS  Effort = "s"
	EffortM  Effort = "m"
	EffortL  Effort = "l"
	EffortXL Effort = "xl"
)

// AllEfforts returns all effort sizes in ascending order.
func AllEfforts() []Effort {
	return []Effort{EffortXS, EffortS, EffortM, EffortL, EffortXL}
}

// Action is one tracked item across the three locations.
type Action struct {
	ID          string    `json:"id"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description"`
	Owner       string    `json:"owner"`
	Deadline    string    `json:"deadline,omitempty"`
	Status      Status    `json:"status"`
	Priority    Priority  `json:"priority,omitempty"`
	Effort      Effort    `json:"effort,omitempty"`
	Context     string    `json:"context,omitempty"`
	SourceNote  string    `json:"source_note"`
	SourceTitle string    `json:"source_title,omitempty"`
	ProjectIDs  []string  `json:"project_ids,omitempty"`
	// L4 (Linear integration) — set after the action is mirrored as a
	// Linear Issue. The JSON action index (vault/.cortical/actions.json)
	// is the single source of truth, so this round-trips automatically
	// without store changes.
	LinearIssueID string    `json:"linear_issue_id,omitempty"`
	Created       time.Time `json:"created"`
	Updated       time.Time `json:"updated"`
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
