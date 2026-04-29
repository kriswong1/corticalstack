package linear

// SyncTarget is the resolved (project, team, workspace) tuple a sync
// orchestration writes against. The fall-through chain is the §3 Fork B
// resolution chain from docs/linear/README.md — written once at L1 so
// L2 (Initiative tier), L3 (Project sync), and L7 (Workspace tier) all
// reuse it without rewriting sync code. M1 mode hits the config
// defaults at every step; later modes populate additional override
// layers.
type SyncTarget struct {
	// LinearProjectID is the cached Linear project identifier. Empty
	// when the local project has never synced — caller must Upsert
	// first and write the returned ID back to the local manifest.
	LinearProjectID string

	// TeamKey is the human-readable team key (e.g. "BCN"). Resolved
	// from per-project override → initiative override → workspace
	// override → config default. The orchestration layer translates
	// this to a Linear teamId via ListTeams at sync time.
	TeamKey string

	// WorkspaceID is the Linear organization ID. M1 leaves this empty;
	// the integration's API key implies a single workspace. Populated
	// in L7 (M3 mode) when multi-workspace dispatch matters.
	WorkspaceID string
}

// ResolveInput is the bag of optional overrides walked by Resolve.
// Each field may be empty; Resolve returns the first non-empty value
// at each tier.
type ResolveInput struct {
	// Per-project overrides (set on internal/projects.Project once L2
	// adds the optional TeamID field).
	ProjectLinearID string
	ProjectTeamKey  string

	// Initiative-level overrides (populated in L2 when the project's
	// linked Initiative carries a TeamID).
	InitiativeTeamKey string

	// Workspace-level overrides (populated in L7 when the project's
	// owning vault/workspaces/<slug>/ manifest carries
	// LinearTeamKey + LinearWorkspaceID).
	WorkspaceTeamKey    string
	WorkspaceLinearID   string

	// Config defaults — always present in M1 onwards. Sourced from
	// LINEAR_TEAM_KEY (and, in M3, LINEAR_DEFAULT_WORKSPACE_ID).
	DefaultTeamKey     string
	DefaultWorkspaceID string
}

// Resolve walks the fall-through chain documented in docs/linear/README.md §3:
//
//	teamId       ← project.TeamID
//	             ?? project.Initiative.TeamID
//	             ?? workspace.LinearTeamKey       (M3 with Wb)
//	             ?? config.DefaultTeamKey         (always present)
//	workspaceId  ← workspace.LinearWorkspaceID    (M3 with Wb)
//	             ?? config.DefaultWorkspaceID
//
// Mode-agnostic by design — adding L7 only populates additional
// override layers without touching this code.
func Resolve(in ResolveInput) SyncTarget {
	return SyncTarget{
		LinearProjectID: in.ProjectLinearID,
		TeamKey: firstNonEmpty(
			in.ProjectTeamKey,
			in.InitiativeTeamKey,
			in.WorkspaceTeamKey,
			in.DefaultTeamKey,
		),
		WorkspaceID: firstNonEmpty(
			in.WorkspaceLinearID,
			in.DefaultWorkspaceID,
		),
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
