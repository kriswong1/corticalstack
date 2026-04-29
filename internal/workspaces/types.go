// Package workspaces manages multi-tenant workspace manifests stored
// inside the Obsidian vault at $VAULT_PATH/workspaces/<slug>/workspace.md.
//
// Workspaces are an optional top-level tier above Initiatives, surfaced
// lazily in the UI: the Workspace sidebar group only appears once at
// least one workspace manifest exists. M3 mode in docs/linear/README.md
// terms — agency or multi-product setups where projects sync to
// different Linear workspaces (or different teams within the same
// workspace) per their owning context.
//
// A workspace carries:
//   - LinearWorkspaceID — the Linear organization id this maps to (M3+)
//   - LinearTeamKey     — default team key for projects in this workspace
//   - LinearAPIKeyEnv   — optional env-var name holding a per-workspace
//                         API key. When set, sync calls for projects
//                         linked to this workspace read the key from
//                         os.Getenv(LinearAPIKeyEnv) instead of the
//                         global LINEAR_API_KEY.
//
// Mirrors the initiatives package shape so the same patterns apply
// uniformly (UUID-stable identity, slug rename, soft-delete to .trash).
package workspaces

import "time"

// Workspace is a top-level tenancy boundary for Linear sync. Optional —
// solo / single-product setups don't need to create one.
type Workspace struct {
	UUID              string    `json:"uuid"                              yaml:"uuid"`
	Slug              string    `json:"slug"                              yaml:"id"`
	Name              string    `json:"name"                              yaml:"name"`
	Description       string    `json:"description,omitempty"             yaml:"description,omitempty"`
	LinearWorkspaceID string    `json:"linear_workspace_id,omitempty"     yaml:"linear_workspace_id,omitempty"`
	LinearTeamKey     string    `json:"linear_team_key,omitempty"         yaml:"linear_team_key,omitempty"`
	LinearAPIKeyEnv   string    `json:"linear_api_key_env,omitempty"      yaml:"linear_api_key_env,omitempty"`
	Created           time.Time `json:"created"                           yaml:"created"`
}

// CreateRequest is the payload for POST /api/workspaces.
type CreateRequest struct {
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	LinearWorkspaceID string `json:"linear_workspace_id,omitempty"`
	LinearTeamKey     string `json:"linear_team_key,omitempty"`
	LinearAPIKeyEnv   string `json:"linear_api_key_env,omitempty"`
}

// UpdateRequest is the payload for PATCH /api/workspaces/{id}. All
// fields optional — nil/"" means "leave unchanged" except for the
// Linear-* fields where the empty string clears the value.
type UpdateRequest struct {
	Name              *string `json:"name,omitempty"`
	Description       *string `json:"description,omitempty"`
	LinearWorkspaceID *string `json:"linear_workspace_id,omitempty"`
	LinearTeamKey     *string `json:"linear_team_key,omitempty"`
	LinearAPIKeyEnv   *string `json:"linear_api_key_env,omitempty"`
}
