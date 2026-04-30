// Package linear is the CorticalStack ↔ Linear GraphQL client.
//
// L1 ships only the read methods needed for the config card, the
// integration registry's HealthCheck, and the read-only sanity routes
// (`/api/integrations/linear/{teams,initiatives,projects}`). Write
// methods (UpsertProject, UpsertIssue, etc.) land in L3/L4.
//
// Auth: Linear personal API keys go into the Authorization header
// without a Bearer prefix. Endpoint: https://api.linear.app/graphql.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultEndpoint = "https://api.linear.app/graphql"

// Client is a thin GraphQL client for Linear. APIKey/OAuthToken are
// read on every call, so the running process picks up
// SetEnvAndPersist updates from the config card without
// re-registration.
//
// Auth precedence: OAuthToken (sent as `Bearer <token>`) wins over
// APIKey (sent raw, no prefix — Linear's personal-API-key convention).
type Client struct {
	APIKey     string
	OAuthToken string
	HTTP       *http.Client
	Endpoint   string // override for tests; empty = production
}

// NewClient builds a client with a personal API key.
func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:   apiKey,
		HTTP:     &http.Client{Timeout: 10 * time.Second},
		Endpoint: defaultEndpoint,
	}
}

// NewOAuthClient builds a client with an OAuth access token.
func NewOAuthClient(token string) *Client {
	return &Client{
		OAuthToken: token,
		HTTP:       &http.Client{Timeout: 10 * time.Second},
		Endpoint:   defaultEndpoint,
	}
}

// configured reports whether either credential is present.
func (c *Client) configured() bool {
	return strings.TrimSpace(c.APIKey) != "" || strings.TrimSpace(c.OAuthToken) != ""
}

// authHeader returns the value to send in the Authorization header.
// OAuth tokens use the "Bearer <token>" form; personal API keys are
// sent raw per Linear's documented convention.
func (c *Client) authHeader() string {
	if t := strings.TrimSpace(c.OAuthToken); t != "" {
		return "Bearer " + t
	}
	return c.APIKey
}

// gqlRequest is the Linear GraphQL request envelope.
type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// gqlError mirrors a single GraphQL error payload.
type gqlError struct {
	Message    string                 `json:"message"`
	Path       []interface{}          `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// gqlResponse is the wire envelope for every Linear response. We keep
// Data raw so each typed method can unmarshal into its own shape.
type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors,omitempty"`
}

// query is the single transport. All typed methods funnel through here.
func (c *Client) query(ctx context.Context, q string, vars map[string]interface{}, out interface{}) error {
	if !c.configured() {
		return fmt.Errorf("linear: not configured (set LINEAR_API_KEY or connect via OAuth)")
	}

	body, err := json.Marshal(gqlRequest{Query: q, Variables: vars})
	if err != nil {
		return fmt.Errorf("linear: marshal request: %w", err)
	}

	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("linear: build request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("linear: request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("linear: read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("linear: http %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}

	var env gqlResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("linear: parse response: %w", err)
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("linear: graphql error: %s", env.Errors[0].Message)
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("linear: decode data: %w", err)
		}
	}
	return nil
}

// --- Read types ---

// Viewer is the authenticated user + their organization (workspace).
type Viewer struct {
	ID           string
	Name         string
	Email        string
	Organization Organization
}

// Organization corresponds to Linear's "workspace" concept.
type Organization struct {
	ID     string
	Name   string
	URLKey string
}

// Team is a Linear team (Issue container).
type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

// Initiative is a workspace-scoped initiative.
type Initiative struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Status is a free-form string (Linear surfaces multiple state types
	// for initiatives — Active / Planned / Completed / etc.).
	Status string `json:"status,omitempty"`
}

// Project is a Linear project (PRD/feature container).
type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state,omitempty"`
	Description string `json:"description,omitempty"`
}

// --- Read methods ---

// FetchViewer hits the authenticated `viewer` query. Cheapest known
// endpoint that fails on a bad key, so we use it for HealthCheck.
func (c *Client) FetchViewer(ctx context.Context) (*Viewer, error) {
	const q = `query { viewer { id name email organization { id name urlKey } } }`
	var resp struct {
		Viewer struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Email        string `json:"email"`
			Organization struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				URLKey string `json:"urlKey"`
			} `json:"organization"`
		} `json:"viewer"`
	}
	if err := c.query(ctx, q, nil, &resp); err != nil {
		return nil, err
	}
	return &Viewer{
		ID:    resp.Viewer.ID,
		Name:  resp.Viewer.Name,
		Email: resp.Viewer.Email,
		Organization: Organization{
			ID:     resp.Viewer.Organization.ID,
			Name:   resp.Viewer.Organization.Name,
			URLKey: resp.Viewer.Organization.URLKey,
		},
	}, nil
}

// ListTeams returns every team the API key can see.
func (c *Client) ListTeams(ctx context.Context) ([]Team, error) {
	const q = `query { teams(first: 100) { nodes { id name key } } }`
	var resp struct {
		Teams struct {
			Nodes []Team `json:"nodes"`
		} `json:"teams"`
	}
	if err := c.query(ctx, q, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Teams.Nodes, nil
}

// ListInitiatives returns every initiative the API key can see.
// Linear's GraphQL query is `initiatives` (workspace-scoped).
func (c *Client) ListInitiatives(ctx context.Context) ([]Initiative, error) {
	const q = `query { initiatives(first: 100) { nodes { id name description status } } }`
	var resp struct {
		Initiatives struct {
			Nodes []Initiative `json:"nodes"`
		} `json:"initiatives"`
	}
	if err := c.query(ctx, q, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Initiatives.Nodes, nil
}

// ListProjects returns every project the API key can see.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	const q = `query { projects(first: 100) { nodes { id name state description } } }`
	var resp struct {
		Projects struct {
			Nodes []Project `json:"nodes"`
		} `json:"projects"`
	}
	if err := c.query(ctx, q, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Projects.Nodes, nil
}

// --- Write inputs (L3) ---

// InitiativeInput carries vault-owned fields per docs/linear/README.md
// §3 Fork D conflict policy. Status / TargetDate are Linear-owned and
// intentionally absent — those flow inbound via L5 webhooks.
type InitiativeInput struct {
	Name        string
	Description string
}

// ProjectInput carries vault-owned fields. The TeamIDs slice must be
// non-empty for projectCreate; subsequent updates may omit it.
type ProjectInput struct {
	Name         string
	Description  string
	TeamIDs      []string
	InitiativeID string // optional Linear initiative id; "" = no initiative link
}

// DocumentInput carries the Project Document body (full shaped-prd.md).
type DocumentInput struct {
	ProjectID string
	Title     string
	Content   string
}

// IssueInput carries fields that map from a CorticalStack Action to a
// Linear Issue at upsert time. Only Title + TeamID are required;
// everything else is optional. Empty strings / zero values mean "leave
// unchanged" on update; on create they fall back to Linear's defaults.
type IssueInput struct {
	Title              string
	Description        string
	TeamID             string
	ProjectID          string // optional
	StateID            string // workflow state id; resolved via state map
	AssigneeID         string // optional Linear user id
	DueDate            string // YYYY-MM-DD; "" = no due date
	Priority           int    // 0=none, 1=urgent, 2=high, 3=normal, 4=low
	Estimate           int    // 0 = unset
	ProjectMilestoneID string // optional
}

// --- Initiative upsert ---

// UpsertInitiative creates or updates a Linear initiative. If
// existingID is empty a fresh initiative is created; otherwise the
// existing one is updated.
func (c *Client) UpsertInitiative(ctx context.Context, existingID string, in InitiativeInput) (string, error) {
	if existingID == "" {
		const q = `mutation Create($input: InitiativeCreateInput!) { initiativeCreate(input: $input) { success initiative { id } } }`
		vars := map[string]interface{}{
			"input": map[string]interface{}{
				"name":        in.Name,
				"description": in.Description,
			},
		}
		var resp struct {
			InitiativeCreate struct {
				Success    bool `json:"success"`
				Initiative struct {
					ID string `json:"id"`
				} `json:"initiative"`
			} `json:"initiativeCreate"`
		}
		if err := c.query(ctx, q, vars, &resp); err != nil {
			return "", err
		}
		if !resp.InitiativeCreate.Success {
			return "", fmt.Errorf("linear: initiativeCreate returned success=false")
		}
		return resp.InitiativeCreate.Initiative.ID, nil
	}

	const q = `mutation Update($id: String!, $input: InitiativeUpdateInput!) { initiativeUpdate(id: $id, input: $input) { success initiative { id } } }`
	vars := map[string]interface{}{
		"id": existingID,
		"input": map[string]interface{}{
			"name":        in.Name,
			"description": in.Description,
		},
	}
	var resp struct {
		InitiativeUpdate struct {
			Success    bool `json:"success"`
			Initiative struct {
				ID string `json:"id"`
			} `json:"initiative"`
		} `json:"initiativeUpdate"`
	}
	if err := c.query(ctx, q, vars, &resp); err != nil {
		return "", err
	}
	if !resp.InitiativeUpdate.Success {
		return "", fmt.Errorf("linear: initiativeUpdate returned success=false")
	}
	return resp.InitiativeUpdate.Initiative.ID, nil
}

// --- Project upsert ---

// UpsertProject creates or updates a Linear project.
func (c *Client) UpsertProject(ctx context.Context, existingID string, in ProjectInput) (string, error) {
	input := map[string]interface{}{
		"name":        in.Name,
		"description": in.Description,
	}
	if len(in.TeamIDs) > 0 {
		input["teamIds"] = in.TeamIDs
	}
	if in.InitiativeID != "" {
		// Linear's projectCreate accepts initiativeId on >=2024 schemas.
		input["initiativeId"] = in.InitiativeID
	}

	if existingID == "" {
		if len(in.TeamIDs) == 0 {
			return "", fmt.Errorf("linear: projectCreate requires at least one teamId")
		}
		const q = `mutation Create($input: ProjectCreateInput!) { projectCreate(input: $input) { success project { id } } }`
		var resp struct {
			ProjectCreate struct {
				Success bool `json:"success"`
				Project struct {
					ID string `json:"id"`
				} `json:"project"`
			} `json:"projectCreate"`
		}
		if err := c.query(ctx, q, map[string]interface{}{"input": input}, &resp); err != nil {
			return "", err
		}
		if !resp.ProjectCreate.Success {
			return "", fmt.Errorf("linear: projectCreate returned success=false")
		}
		return resp.ProjectCreate.Project.ID, nil
	}

	// Update path: don't re-send teamIds (Linear treats it as immutable).
	delete(input, "teamIds")
	const q = `mutation Update($id: String!, $input: ProjectUpdateInput!) { projectUpdate(id: $id, input: $input) { success project { id } } }`
	var resp struct {
		ProjectUpdate struct {
			Success bool `json:"success"`
			Project struct {
				ID string `json:"id"`
			} `json:"project"`
		} `json:"projectUpdate"`
	}
	if err := c.query(ctx, q, map[string]interface{}{"id": existingID, "input": input}, &resp); err != nil {
		return "", err
	}
	if !resp.ProjectUpdate.Success {
		return "", fmt.Errorf("linear: projectUpdate returned success=false")
	}
	return resp.ProjectUpdate.Project.ID, nil
}

// --- Document upsert ---

// UpsertProjectDocument creates or updates a Project Document. Documents
// hold the assembled shaped-prd.md per docs/linear/README.md §3 Fork A2.
func (c *Client) UpsertProjectDocument(ctx context.Context, existingID string, in DocumentInput) (string, error) {
	if existingID == "" {
		const q = `mutation Create($input: DocumentCreateInput!) { documentCreate(input: $input) { success document { id } } }`
		vars := map[string]interface{}{
			"input": map[string]interface{}{
				"projectId": in.ProjectID,
				"title":     in.Title,
				"content":   in.Content,
			},
		}
		var resp struct {
			DocumentCreate struct {
				Success  bool `json:"success"`
				Document struct {
					ID string `json:"id"`
				} `json:"document"`
			} `json:"documentCreate"`
		}
		if err := c.query(ctx, q, vars, &resp); err != nil {
			return "", err
		}
		if !resp.DocumentCreate.Success {
			return "", fmt.Errorf("linear: documentCreate returned success=false")
		}
		return resp.DocumentCreate.Document.ID, nil
	}

	const q = `mutation Update($id: String!, $input: DocumentUpdateInput!) { documentUpdate(id: $id, input: $input) { success document { id } } }`
	vars := map[string]interface{}{
		"id": existingID,
		"input": map[string]interface{}{
			"title":   in.Title,
			"content": in.Content,
		},
	}
	var resp struct {
		DocumentUpdate struct {
			Success  bool `json:"success"`
			Document struct {
				ID string `json:"id"`
			} `json:"document"`
		} `json:"documentUpdate"`
	}
	if err := c.query(ctx, q, vars, &resp); err != nil {
		return "", err
	}
	if !resp.DocumentUpdate.Success {
		return "", fmt.Errorf("linear: documentUpdate returned success=false")
	}
	return resp.DocumentUpdate.Document.ID, nil
}

// ResolveTeamID looks up a teamId by team key (e.g. "BCN" → uuid).
// Cached per call; sync orchestration calls this once per SyncProject
// to translate the configured LINEAR_TEAM_KEY.
func (c *Client) ResolveTeamID(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("linear: empty team key")
	}
	teams, err := c.ListTeams(ctx)
	if err != nil {
		return "", err
	}
	for _, t := range teams {
		if t.Key == key {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("linear: team key %q not found in workspace", key)
}

// --- Issue upsert (L4) ---

// UpsertIssue creates or updates a Linear issue.
func (c *Client) UpsertIssue(ctx context.Context, existingID string, in IssueInput) (string, error) {
	input := map[string]interface{}{}
	if in.Title != "" {
		input["title"] = in.Title
	}
	if in.Description != "" {
		input["description"] = in.Description
	}
	if in.TeamID != "" {
		input["teamId"] = in.TeamID
	}
	if in.ProjectID != "" {
		input["projectId"] = in.ProjectID
	}
	if in.StateID != "" {
		input["stateId"] = in.StateID
	}
	if in.AssigneeID != "" {
		input["assigneeId"] = in.AssigneeID
	}
	if in.DueDate != "" {
		input["dueDate"] = in.DueDate
	}
	if in.Priority > 0 {
		input["priority"] = in.Priority
	}
	if in.Estimate > 0 {
		input["estimate"] = in.Estimate
	}
	if in.ProjectMilestoneID != "" {
		input["projectMilestoneId"] = in.ProjectMilestoneID
	}

	if existingID == "" {
		if in.TeamID == "" {
			return "", fmt.Errorf("linear: issueCreate requires teamId")
		}
		const q = `mutation Create($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { id } } }`
		var resp struct {
			IssueCreate struct {
				Success bool `json:"success"`
				Issue   struct {
					ID string `json:"id"`
				} `json:"issue"`
			} `json:"issueCreate"`
		}
		if err := c.query(ctx, q, map[string]interface{}{"input": input}, &resp); err != nil {
			return "", err
		}
		if !resp.IssueCreate.Success {
			return "", fmt.Errorf("linear: issueCreate returned success=false")
		}
		return resp.IssueCreate.Issue.ID, nil
	}

	// teamId is immutable on issueUpdate — drop it.
	delete(input, "teamId")
	const q = `mutation Update($id: String!, $input: IssueUpdateInput!) { issueUpdate(id: $id, input: $input) { success issue { id } } }`
	var resp struct {
		IssueUpdate struct {
			Success bool `json:"success"`
			Issue   struct {
				ID string `json:"id"`
			} `json:"issue"`
		} `json:"issueUpdate"`
	}
	if err := c.query(ctx, q, map[string]interface{}{"id": existingID, "input": input}, &resp); err != nil {
		return "", err
	}
	if !resp.IssueUpdate.Success {
		return "", fmt.Errorf("linear: issueUpdate returned success=false")
	}
	return resp.IssueUpdate.Issue.ID, nil
}

// MilestoneInput carries fields for a Project Milestone create/update.
type MilestoneInput struct {
	ProjectID  string
	Name       string
	TargetDate string // YYYY-MM-DD; "" = no target
}

// UpsertProjectMilestone creates or updates a Project Milestone.
func (c *Client) UpsertProjectMilestone(ctx context.Context, existingID string, in MilestoneInput) (string, error) {
	input := map[string]interface{}{
		"projectId": in.ProjectID,
		"name":      in.Name,
	}
	if in.TargetDate != "" {
		input["targetDate"] = in.TargetDate
	}
	if existingID == "" {
		const q = `mutation Create($input: ProjectMilestoneCreateInput!) { projectMilestoneCreate(input: $input) { success projectMilestone { id } } }`
		var resp struct {
			ProjectMilestoneCreate struct {
				Success          bool `json:"success"`
				ProjectMilestone struct {
					ID string `json:"id"`
				} `json:"projectMilestone"`
			} `json:"projectMilestoneCreate"`
		}
		if err := c.query(ctx, q, map[string]interface{}{"input": input}, &resp); err != nil {
			return "", err
		}
		if !resp.ProjectMilestoneCreate.Success {
			return "", fmt.Errorf("linear: projectMilestoneCreate returned success=false")
		}
		return resp.ProjectMilestoneCreate.ProjectMilestone.ID, nil
	}

	delete(input, "projectId")
	const q = `mutation Update($id: String!, $input: ProjectMilestoneUpdateInput!) { projectMilestoneUpdate(id: $id, input: $input) { success projectMilestone { id } } }`
	var resp struct {
		ProjectMilestoneUpdate struct {
			Success          bool `json:"success"`
			ProjectMilestone struct {
				ID string `json:"id"`
			} `json:"projectMilestone"`
		} `json:"projectMilestoneUpdate"`
	}
	if err := c.query(ctx, q, map[string]interface{}{"id": existingID, "input": input}, &resp); err != nil {
		return "", err
	}
	if !resp.ProjectMilestoneUpdate.Success {
		return "", fmt.Errorf("linear: projectMilestoneUpdate returned success=false")
	}
	return resp.ProjectMilestoneUpdate.ProjectMilestone.ID, nil
}

// WorkflowState is one Linear workflow state for a team.
type WorkflowState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // backlog / unstarted / started / completed / canceled
}

// ListWorkflowStates returns every workflow state for the given team.
// Used by the L4 state-map bootstrap to compute defaults.
func (c *Client) ListWorkflowStates(ctx context.Context, teamID string) ([]WorkflowState, error) {
	const q = `query Q($teamId: String!) { workflowStates(filter: { team: { id: { eq: $teamId } } }, first: 50) { nodes { id name type } } }`
	var resp struct {
		WorkflowStates struct {
			Nodes []WorkflowState `json:"nodes"`
		} `json:"workflowStates"`
	}
	if err := c.query(ctx, q, map[string]interface{}{"teamId": teamID}, &resp); err != nil {
		return nil, err
	}
	return resp.WorkflowStates.Nodes, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
