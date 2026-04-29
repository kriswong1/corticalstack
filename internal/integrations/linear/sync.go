package linear

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/initiatives"
	"github.com/kriswong/corticalstack/internal/prds"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/workspaces"
)

// SyncPreview is the diff returned by a dry-run sync. The frontend
// renders this as the first step of the two-step modal so the user can
// confirm before any writes hit Linear.
type SyncPreview struct {
	ProjectName        string   `json:"project_name"`
	InitiativeAction   string   `json:"initiative_action,omitempty"` // "create" / "update" / "" (no initiative linked)
	InitiativeName     string   `json:"initiative_name,omitempty"`
	ProjectAction      string   `json:"project_action"`              // "create" / "update"
	DocumentsToCreate  int      `json:"documents_to_create"`
	DocumentsToUpdate  int      `json:"documents_to_update"`
	DocumentTitles     []string `json:"document_titles,omitempty"`
	IssuesToCreate     int      `json:"issues_to_create"`     // L4
	IssuesToUpdate     int      `json:"issues_to_update"`     // L4
	TeamKey            string   `json:"team_key"`
	Warnings           []string `json:"warnings,omitempty"`
}

// SyncError describes one entity that failed to sync. The orchestration
// continues past per-entity errors so a single document failure doesn't
// abort the whole operation.
type SyncError struct {
	Entity string `json:"entity"` // "initiative" / "project" / "document:<title>" / "issue:<id>"
	Err    string `json:"error"`
}

// SyncResult is the outcome of a confirmed sync.
type SyncResult struct {
	ProjectLinearID string      `json:"project_linear_id,omitempty"`
	Created         []string    `json:"created,omitempty"`
	Updated         []string    `json:"updated,omitempty"`
	Errors          []SyncError `json:"errors,omitempty"`
}

// SyncStores bundles the stores the orchestration writes back to.
type SyncStores struct {
	Projects    *projects.Store
	Initiatives *initiatives.Store
	Workspaces  *workspaces.Store // L7 — optional (M1/M2 leave it nil)
	PRDs        *prds.Store
	Actions     *actions.Store
}

// Orchestrator owns the SyncProject + SyncAction call paths. Wraps
// Client + SyncStores + the resolution chain so handlers don't have to
// thread every dep through.
type Orchestrator struct {
	Client      *Client
	Stores      SyncStores
	DefaultTeam string // LINEAR_TEAM_KEY
}

// NewOrchestrator wires a fresh orchestrator. Caller is responsible for
// ensuring Client.Configured(); SyncProject / SyncAction will return an
// error if the API key is missing.
func NewOrchestrator(c *Client, stores SyncStores, defaultTeam string) *Orchestrator {
	return &Orchestrator{Client: c, Stores: stores, DefaultTeam: defaultTeam}
}

// resolveContext walks the §3 Fork B resolution chain for a project +
// optional linked initiative and returns the Client + team key the
// sync call should use. When the project links to a workspace whose
// LinearAPIKeyEnv points at a populated env var, a fresh Client is
// constructed with that key; otherwise the orchestrator's default
// Client is reused.
//
// Returns nil error iff a non-empty team key was resolved at some
// layer of the chain.
func (o *Orchestrator) resolveContext(p *projects.Project, linkedInitiative *initiatives.Initiative) (*Client, string, error) {
	in := ResolveInput{DefaultTeamKey: strings.TrimSpace(o.DefaultTeam)}

	// Per-project Tb override (highest priority).
	if p != nil && p.TeamKey != nil && *p.TeamKey != "" {
		in.ProjectTeamKey = *p.TeamKey
	}
	// Initiative-level override.
	if linkedInitiative != nil && linkedInitiative.TeamKey != nil && *linkedInitiative.TeamKey != "" {
		in.InitiativeTeamKey = *linkedInitiative.TeamKey
	}

	// Workspace-level overrides + per-workspace API key.
	client := o.Client
	if p != nil && p.WorkspaceID != nil && *p.WorkspaceID != "" && o.Stores.Workspaces != nil {
		if w := o.Stores.Workspaces.GetByUUID(*p.WorkspaceID); w != nil {
			if w.LinearTeamKey != "" {
				in.WorkspaceTeamKey = w.LinearTeamKey
			}
			if w.LinearWorkspaceID != "" {
				in.WorkspaceLinearID = w.LinearWorkspaceID
			}
			if w.LinearAPIKeyEnv != "" {
				if key := os.Getenv(w.LinearAPIKeyEnv); key != "" {
					client = NewClient(key)
				} else {
					slog.Warn("linear: workspace api-key env unset, falling back to default",
						"workspace_uuid", w.UUID,
						"env", w.LinearAPIKeyEnv)
				}
			}
		}
	}

	target := Resolve(in)
	if target.TeamKey == "" {
		return nil, "", fmt.Errorf("linear: no team key resolved (set LINEAR_TEAM_KEY or workspace/initiative/project team_key)")
	}
	if client == nil || !client.configured() {
		return nil, "", fmt.Errorf("linear: not configured (no API key)")
	}
	return client, target.TeamKey, nil
}

// SyncProject upserts a CorticalStack project (and its linked
// Initiative + PRDs) into Linear.
//
// dryRun=true short-circuits before any GraphQL writes and returns a
// preview; the caller renders it and then calls again with
// dryRun=false to commit.
func (o *Orchestrator) SyncProject(ctx context.Context, projectIDOrSlug string, dryRun bool) (*SyncPreview, *SyncResult, error) {
	if o.Client == nil {
		return nil, nil, fmt.Errorf("linear: client not configured")
	}
	if !o.Client.configured() {
		return nil, nil, fmt.Errorf("linear: not configured (set LINEAR_API_KEY)")
	}
	if o.Stores.Projects == nil {
		return nil, nil, fmt.Errorf("linear: project store not wired")
	}

	p := o.Stores.Projects.Get(projectIDOrSlug)
	if p == nil {
		return nil, nil, fmt.Errorf("linear: project %q not found", projectIDOrSlug)
	}

	// --- Initiative side (load first so the resolution chain can pick
	// up any initiative-level team_key override) ---
	var linkedInitiative *initiatives.Initiative
	var initiativeAction string
	if p.InitiativeID != nil && *p.InitiativeID != "" && o.Stores.Initiatives != nil {
		linkedInitiative = o.Stores.Initiatives.GetByUUID(*p.InitiativeID)
		if linkedInitiative != nil {
			if linkedInitiative.LinearID == "" {
				initiativeAction = "create"
			} else {
				initiativeAction = "update"
			}
		}
	}

	// L7 resolution: per-project / initiative / workspace / config-default.
	client, teamKey, err := o.resolveContext(p, linkedInitiative)
	if err != nil {
		return nil, nil, err
	}
	teamID, err := client.ResolveTeamID(ctx, teamKey)
	if err != nil {
		return nil, nil, err
	}

	// --- Project side ---
	projectAction := "update"
	if p.LinearProjectID == "" {
		projectAction = "create"
	}

	// --- Documents (linked PRDs) ---
	linkedPRDs := o.linkedPRDs(p.UUID)
	var docTitles []string
	docCreate, docUpdate := 0, 0
	for _, prd := range linkedPRDs {
		docTitles = append(docTitles, prd.ID+"-shaped-prd")
		if prd.LinearDocumentID == "" {
			docCreate++
		} else {
			docUpdate++
		}
	}

	// --- Issues (linked Actions, L4) ---
	linkedActions := o.linkedActions(p.UUID)
	issueCreate, issueUpdate := 0, 0
	for _, a := range linkedActions {
		if a.LinearIssueID == "" {
			issueCreate++
		} else {
			issueUpdate++
		}
	}

	preview := &SyncPreview{
		ProjectName:       p.Name,
		ProjectAction:     projectAction,
		InitiativeAction:  initiativeAction,
		DocumentsToCreate: docCreate,
		DocumentsToUpdate: docUpdate,
		DocumentTitles:    docTitles,
		IssuesToCreate:    issueCreate,
		IssuesToUpdate:    issueUpdate,
		TeamKey:           teamKey,
	}
	if linkedInitiative != nil {
		preview.InitiativeName = linkedInitiative.Name
	}
	if p.InitiativeID != nil && *p.InitiativeID != "" && linkedInitiative == nil {
		preview.Warnings = append(preview.Warnings,
			fmt.Sprintf("project links to unknown initiative_id %q — proceeding without initiative", *p.InitiativeID))
	}

	if dryRun {
		return preview, nil, nil
	}

	result := &SyncResult{}

	// Step 1: upsert initiative if linked.
	var linearInitiativeID string
	if linkedInitiative != nil {
		linearInitiativeID = linkedInitiative.LinearID
		newID, err := client.UpsertInitiative(ctx, linkedInitiative.LinearID, InitiativeToInput(linkedInitiative))
		if err != nil {
			result.Errors = append(result.Errors, SyncError{Entity: "initiative:" + linkedInitiative.Name, Err: err.Error()})
		} else {
			linearInitiativeID = newID
			if linkedInitiative.LinearID == "" {
				result.Created = append(result.Created, "initiative: "+linkedInitiative.Name)
				if err := o.Stores.Initiatives.SetLinearID(linkedInitiative.UUID, newID); err != nil {
					slog.Warn("linear sync: failed to persist initiative LinearID", "uuid", linkedInitiative.UUID, "error", err)
				}
			} else {
				result.Updated = append(result.Updated, "initiative: "+linkedInitiative.Name)
			}
		}
	}

	// Step 2: upsert project.
	projectInput := ProjectToInput(p, primaryPRD(linkedPRDs), []string{teamID}, linearInitiativeID)
	newProjectID, err := client.UpsertProject(ctx, p.LinearProjectID, projectInput)
	if err != nil {
		result.Errors = append(result.Errors, SyncError{Entity: "project:" + p.Name, Err: err.Error()})
		return preview, result, nil
	}
	result.ProjectLinearID = newProjectID
	if p.LinearProjectID == "" {
		result.Created = append(result.Created, "project: "+p.Name)
	} else {
		result.Updated = append(result.Updated, "project: "+p.Name)
	}
	if err := o.Stores.Projects.SetLinearSyncMetadata(p.UUID, newProjectID, time.Now()); err != nil {
		slog.Warn("linear sync: failed to persist project LinearID", "uuid", p.UUID, "error", err)
	}

	// Step 3: upsert one Project Document per linked PRD.
	for _, prd := range linkedPRDs {
		docInput := PRDBodyToProjectDocument(newProjectID, prd)
		newDocID, err := client.UpsertProjectDocument(ctx, prd.LinearDocumentID, docInput)
		if err != nil {
			result.Errors = append(result.Errors, SyncError{Entity: "document:" + prd.ID, Err: err.Error()})
			continue
		}
		if prd.LinearDocumentID == "" {
			result.Created = append(result.Created, "document: "+prd.ID)
		} else {
			result.Updated = append(result.Updated, "document: "+prd.ID)
		}
		updated := *prd
		updated.LinearDocumentID = newDocID
		if err := o.Stores.PRDs.Write(&updated); err != nil {
			slog.Warn("linear sync: failed to persist PRD LinearDocumentID", "id", prd.ID, "error", err)
		}
	}

	// Step 4 (L4): upsert one Issue per linked Action.
	if len(linkedActions) > 0 {
		stateMap, err := o.loadOrBootstrapStateMap(ctx, client, teamKey, teamID)
		if err != nil {
			result.Errors = append(result.Errors, SyncError{Entity: "state_map", Err: err.Error()})
		} else {
			for _, a := range linkedActions {
				if err := o.syncOneAction(ctx, client, a, teamID, newProjectID, stateMap); err != nil {
					result.Errors = append(result.Errors, SyncError{Entity: "issue:" + a.ID, Err: err.Error()})
					continue
				}
				if a.LinearIssueID == "" {
					result.Created = append(result.Created, "issue: "+a.ID)
				} else {
					result.Updated = append(result.Updated, "issue: "+a.ID)
				}
			}
		}
	}

	return preview, result, nil
}

// SyncAction upserts a single Action as a Linear Issue. Used by the
// post-mutation auto-sync goroutine triggered from the action handlers.
// No-op if the project hasn't been synced yet (no LinearProjectID).
func (o *Orchestrator) SyncAction(ctx context.Context, actionID string) error {
	if o.Stores.Actions == nil {
		return fmt.Errorf("linear: actions store not wired")
	}
	a := o.Stores.Actions.Get(actionID)
	if a == nil {
		return fmt.Errorf("linear: action %q not found", actionID)
	}
	// Find the first linked project that's already synced; bail if none.
	var sourceProject *projects.Project
	for _, projectUUID := range a.ProjectIDs {
		if o.Stores.Projects == nil {
			break
		}
		p := o.Stores.Projects.GetByUUID(projectUUID)
		if p != nil && p.LinearProjectID != "" {
			sourceProject = p
			break
		}
	}
	if sourceProject == nil {
		return nil // not an error — action's project just hasn't been synced yet
	}
	// L7 resolution — pick the right client + team for the source project.
	var linkedInitiative *initiatives.Initiative
	if sourceProject.InitiativeID != nil && *sourceProject.InitiativeID != "" && o.Stores.Initiatives != nil {
		linkedInitiative = o.Stores.Initiatives.GetByUUID(*sourceProject.InitiativeID)
	}
	client, teamKey, err := o.resolveContext(sourceProject, linkedInitiative)
	if err != nil {
		return err
	}
	teamID, err := client.ResolveTeamID(ctx, teamKey)
	if err != nil {
		return err
	}
	stateMap, err := o.loadOrBootstrapStateMap(ctx, client, teamKey, teamID)
	if err != nil {
		return err
	}
	return o.syncOneAction(ctx, client, a, teamID, sourceProject.LinearProjectID, stateMap)
}

// syncOneAction is the shared upsert path for both batch (SyncProject)
// and single-action (SyncAction) flows.
func (o *Orchestrator) syncOneAction(ctx context.Context, client *Client, a *actions.Action, teamID, projectLinearID string, stateMap map[actions.Status]string) error {
	stateID := stateMap[a.Status]
	in := ActionToIssueInput(a, teamID, projectLinearID, stateID)
	newID, err := client.UpsertIssue(ctx, a.LinearIssueID, in)
	if err != nil {
		return err
	}
	if a.LinearIssueID == newID {
		return nil
	}
	// Persist back via the actions store.
	patch := actions.ActionPatch{LinearIssueID: &newID}
	if _, err := o.Stores.Actions.Update(a.ID, patch); err != nil {
		slog.Warn("linear sync: failed to persist action LinearIssueID", "id", a.ID, "error", err)
	}
	return nil
}

func (o *Orchestrator) linkedPRDs(projectUUID string) []*prds.PRD {
	if o.Stores.PRDs == nil {
		return nil
	}
	all, err := o.Stores.PRDs.List()
	if err != nil {
		return nil
	}
	var out []*prds.PRD
	for _, p := range all {
		for _, ref := range p.Projects {
			if ref == projectUUID {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func (o *Orchestrator) linkedActions(projectUUID string) []*actions.Action {
	if o.Stores.Actions == nil {
		return nil
	}
	var out []*actions.Action
	for _, a := range o.Stores.Actions.List() {
		for _, ref := range a.ProjectIDs {
			if ref == projectUUID {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// primaryPRD picks the most relevant PRD for the project.description
// field. Heuristic: highest-version draft, falling back to first PRD.
// Empty input → nil; ProjectToInput then falls back to project.Description.
func primaryPRD(linked []*prds.PRD) *prds.PRD {
	if len(linked) == 0 {
		return nil
	}
	pick := linked[0]
	for _, p := range linked[1:] {
		if p.Version > pick.Version {
			pick = p
		}
	}
	return pick
}
