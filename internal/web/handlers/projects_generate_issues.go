package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/integrations/linear"
)

// GenerateIssuesFromPRD handles POST /api/projects/{id}/generate-issues-from-prd.
// Default = dry-run; ?confirm=1 executes the additive generate run.
//
// Per Q8 lock: existing Issues in the sidecar map are never modified
// or deleted on re-run; only new criterion hashes produce new Issues.
func (h *Handler) GenerateIssuesFromPRD(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	li := h.linearIntegration()
	if li == nil || !li.Configured() {
		http.Error(w, "linear not configured", http.StatusBadRequest)
		return
	}
	if li.CurrentTeamKey() == "" {
		http.Error(w, "linear team key not configured (set LINEAR_TEAM_KEY)", http.StatusBadRequest)
		return
	}

	stores := linear.SyncStores{
		Projects:    h.Projects,
		Initiatives: h.Initiatives,
		PRDs:        h.PRDs,
		Actions:     h.Actions,
	}
	orch := linear.NewOrchestrator(li.Client, stores, li.CurrentTeamKey())
	confirm := r.URL.Query().Get("confirm") == "1"
	preview, result, err := orch.GenerateIssuesFromPRD(r.Context(), id, !confirm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]interface{}{
		"preview": preview,
		"result":  result,
	})
}
