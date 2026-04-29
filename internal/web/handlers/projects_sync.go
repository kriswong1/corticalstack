package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/integrations/linear"
)

// SyncProjectToLinear handles POST /api/projects/{id}/sync.
//
// Default = dry-run (returns SyncPreview).
// `?confirm=1` executes and returns {preview, result}.
//
// Wire-format envelope chosen so the modal's two-step flow can render
// the same JSON shape on both calls without branching.
func (h *Handler) SyncProjectToLinear(w http.ResponseWriter, r *http.Request) {
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
	preview, result, err := orch.SyncProject(r.Context(), id, !confirm)
	if err != nil {
		internalError(w, "linear.sync_project", err)
		return
	}

	resp := map[string]interface{}{
		"preview": preview,
		"result":  result,
	}
	writeJSON(w, resp)
}
