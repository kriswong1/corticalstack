// Package initiatives manages strategic initiatives stored inside the
// Obsidian vault at $VAULT_PATH/initiatives/<slug>/initiative.md.
//
// Initiatives are an optional tier above Projects, surfaced lazily in
// the UI: the Initiative sidebar group only appears once at least one
// initiative manifest exists. Mirrors internal/projects in shape so
// the same patterns (UUID-stable identity, slug rename, soft-delete to
// .trash) apply uniformly across both stores.
//
// In Linear's data model an Initiative carries a name, summary,
// description (long markdown), status, target date, owner, and optional
// parent initiative for nesting. CorticalStack mirrors this so the
// vault is the source of truth for content (Name / Description) while
// Linear remains the source of truth for workflow state (Status,
// TargetDate, Owner) — see docs/linear/README.md §3 Fork D conflict
// policy.
package initiatives

import "time"

// Status is the lifecycle state of an initiative. Mirrors the Project
// enum so consumers can apply the same color/badge logic across both
// tiers without forking on type.
type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

// Initiative is a strategic theme that groups one or more Projects.
//
// UUID is the canonical identity — referenced from a Project's
// optional `initiative_id` frontmatter field. Slug is a renameable
// display alias and the on-disk directory name.
type Initiative struct {
	UUID                string     `json:"uuid"                            yaml:"uuid"`
	Slug                string     `json:"slug"                            yaml:"id"` // yaml key stays "id" for visual parity with Project
	Name                string     `json:"name"                            yaml:"name"`
	Status              Status     `json:"status"                          yaml:"status"`
	Description         string     `json:"description,omitempty"           yaml:"description,omitempty"`
	TargetDate          *time.Time `json:"target_date,omitempty"           yaml:"target_date,omitempty"`
	Owner               string     `json:"owner,omitempty"                 yaml:"owner,omitempty"`
	ParentInitiativeID  *string    `json:"parent_initiative_id,omitempty"  yaml:"parent_initiative_id,omitempty"`
	TeamID              *string    `json:"team_id,omitempty"               yaml:"team_id,omitempty"`
	LinearID            string     `json:"linear_id,omitempty"             yaml:"linear_id,omitempty"`
	Created             time.Time  `json:"created"                         yaml:"created"`
}

// CreateRequest is the payload for POST /api/initiatives.
type CreateRequest struct {
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	Owner              string  `json:"owner,omitempty"`
	TargetDate         string  `json:"target_date,omitempty"` // RFC3339 or YYYY-MM-DD
	ParentInitiativeID *string `json:"parent_initiative_id,omitempty"`
	TeamID             *string `json:"team_id,omitempty"`
}

// UpdateRequest is the payload for PATCH /api/initiatives/{id}. All
// fields are optional — nil means "leave unchanged".
type UpdateRequest struct {
	Name               *string `json:"name,omitempty"`
	Description        *string `json:"description,omitempty"`
	Status             *Status `json:"status,omitempty"`
	Owner              *string `json:"owner,omitempty"`
	TargetDate         *string `json:"target_date,omitempty"` // RFC3339 / YYYY-MM-DD; "" clears
	ParentInitiativeID *string `json:"parent_initiative_id,omitempty"`
	TeamID             *string `json:"team_id,omitempty"`
}
