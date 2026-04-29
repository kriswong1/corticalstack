package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/integrations/linear"
)

// linearClient pulls the registered Linear integration off the registry
// and returns its Client. Returns nil if the integration isn't
// registered for some reason (shouldn't happen post-main.go wiring).
func (h *Handler) linearClient() *linear.Client {
	i := h.Registry.Get("linear")
	if i == nil {
		return nil
	}
	if li, ok := i.(*linear.Integration); ok && li.Client != nil {
		return li.Client
	}
	return nil
}

// linearIntegration returns the registered *linear.Integration so
// handlers can call SetCredentials after a Save.
func (h *Handler) linearIntegration() *linear.Integration {
	i := h.Registry.Get("linear")
	if i == nil {
		return nil
	}
	if li, ok := i.(*linear.Integration); ok {
		return li
	}
	return nil
}

// LinearStatus reports whether Linear is configured and, if so, the
// resolved organization + team.
// GET /api/integrations/linear/status
func (h *Handler) LinearStatus(w http.ResponseWriter, r *http.Request) {
	li := h.linearIntegration()
	resp := map[string]interface{}{
		"configured":                false,
		"team_key":                  "",
		"webhook_secret_configured": config.LinearWebhookSecret() != "",
	}
	if h.LinearWebhooks != nil {
		if t := h.LinearWebhooks.LastReceivedAt(); !t.IsZero() {
			resp["last_webhook_at"] = t.Format("2006-01-02T15:04:05Z07:00")
		}
	}
	if li == nil || !li.Configured() {
		writeJSON(w, resp)
		return
	}
	resp["configured"] = true
	resp["team_key"] = li.CurrentTeamKey()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if v, err := li.Client.FetchViewer(ctx); err == nil {
		resp["organization"] = map[string]string{
			"id":      v.Organization.ID,
			"name":    v.Organization.Name,
			"url_key": v.Organization.URLKey,
		}
		resp["viewer"] = map[string]string{
			"id":    v.ID,
			"name":  v.Name,
			"email": v.Email,
		}
	} else {
		resp["error"] = err.Error()
	}
	writeJSON(w, resp)
}

// ListLinearTeams returns every team the configured key can see.
// GET /api/integrations/linear/teams
func (h *Handler) ListLinearTeams(w http.ResponseWriter, r *http.Request) {
	li := h.linearIntegration()
	if li == nil || !li.Configured() {
		http.Error(w, "linear not configured", http.StatusBadRequest)
		return
	}
	c := li.Client
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	teams, err := c.ListTeams(ctx)
	if err != nil {
		internalError(w, "linear.list_teams", err)
		return
	}
	writeJSON(w, teams)
}

// ListLinearInitiatives returns every initiative the configured key can see.
// GET /api/integrations/linear/initiatives
func (h *Handler) ListLinearInitiatives(w http.ResponseWriter, r *http.Request) {
	li := h.linearIntegration()
	if li == nil || !li.Configured() {
		http.Error(w, "linear not configured", http.StatusBadRequest)
		return
	}
	c := li.Client
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	inits, err := c.ListInitiatives(ctx)
	if err != nil {
		internalError(w, "linear.list_initiatives", err)
		return
	}
	writeJSON(w, inits)
}

// ListLinearProjects returns every project the configured key can see.
// GET /api/integrations/linear/projects
func (h *Handler) ListLinearProjects(w http.ResponseWriter, r *http.Request) {
	li := h.linearIntegration()
	if li == nil || !li.Configured() {
		http.Error(w, "linear not configured", http.StatusBadRequest)
		return
	}
	c := li.Client
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	projects, err := c.ListProjects(ctx)
	if err != nil {
		internalError(w, "linear.list_projects", err)
		return
	}
	writeJSON(w, projects)
}

// TestLinear validates a candidate API key by fetching the viewer.
// POST /api/integrations/linear/test  {api_key, team_key}
func (h *Handler) TestLinear(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey  string `json:"api_key"`
		TeamKey string `json:"team_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "API key is required"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	client := linear.NewClient(key)
	v, err := client.FetchViewer(ctx)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	// Optional team key validation: if supplied, confirm the key exists
	// in the workspace's team list. Soft-fails (warning, not error) so
	// the user can still save and adjust later.
	resp := map[string]interface{}{
		"ok":           true,
		"organization": v.Organization.Name,
		"viewer":       v.Name,
	}
	if teamKey := strings.TrimSpace(req.TeamKey); teamKey != "" {
		teams, err := client.ListTeams(ctx)
		if err != nil {
			resp["team_warning"] = "could not verify team key: " + err.Error()
		} else {
			found := false
			for _, t := range teams {
				if strings.EqualFold(t.Key, teamKey) {
					found = true
					resp["team_name"] = t.Name
					break
				}
			}
			if !found {
				resp["team_warning"] = "team key " + teamKey + " not found in workspace"
			}
		}
	}
	writeJSON(w, resp)
}

// SaveLinear persists API key + team key + webhook secret to .env and
// updates the live integration so the running process picks them up
// without a restart.
// POST /api/integrations/linear/save  {api_key, team_key, webhook_secret}
func (h *Handler) SaveLinear(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey        string `json:"api_key"`
		TeamKey       string `json:"team_key"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(req.APIKey)
	teamKey := strings.TrimSpace(req.TeamKey)
	webhookSecret := strings.TrimSpace(req.WebhookSecret)
	if key == "" {
		http.Error(w, "api_key required", http.StatusBadRequest)
		return
	}

	if err := config.SetEnvAndPersist("LINEAR_API_KEY", key); err != nil {
		internalError(w, "integrations.linear.save_key", err)
		return
	}
	if teamKey != "" {
		if err := config.SetEnvAndPersist("LINEAR_TEAM_KEY", teamKey); err != nil {
			internalError(w, "integrations.linear.save_team", err)
			return
		}
	}
	if webhookSecret != "" {
		if err := config.SetEnvAndPersist("LINEAR_WEBHOOK_SECRET", webhookSecret); err != nil {
			internalError(w, "integrations.linear.save_webhook", err)
			return
		}
	}
	if li := h.linearIntegration(); li != nil {
		li.SetCredentials(key, teamKey)
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}
