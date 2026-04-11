package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/actions"
)

// ListActions returns all tracked actions, optionally filtered by ?status=.
func (h *Handler) ListActions(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		writeJSON(w, []actions.Action{})
		return
	}
	status := r.URL.Query().Get("status")
	var out []*actions.Action
	if status != "" {
		out = h.Actions.ListByStatus(actions.Status(status))
	} else {
		out = h.Actions.List()
	}
	writeJSON(w, out)
}

// ActionCounts returns a status → count map.
func (h *Handler) ActionCounts(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		writeJSON(w, map[string]int{})
		return
	}
	writeJSON(w, h.Actions.CountByStatus())
}

// SetActionStatus handles POST /api/actions/{id}/status with JSON { status }.
func (h *Handler) SetActionStatus(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		http.Error(w, "action store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !actions.IsValid(body.Status) {
		http.Error(w, "invalid status: "+body.Status, http.StatusBadRequest)
		return
	}
	updated, err := h.Actions.SetStatus(id, actions.Status(body.Status))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	// Propagate the change to every markdown location.
	if err := h.Actions.Sync(updated); err != nil {
		http.Error(w, "sync: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, updated)
}

// ReconcileActions handles POST /api/actions/reconcile.
func (h *Handler) ReconcileActions(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		http.Error(w, "action store not configured", http.StatusServiceUnavailable)
		return
	}
	res, err := h.Actions.Reconcile()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// ActionsPage renders the global action tracker view.
func (h *Handler) ActionsPage(w http.ResponseWriter, r *http.Request) {
	var items []*actions.Action
	var counts map[actions.Status]int
	if h.Actions != nil {
		items = h.Actions.List()
		counts = h.Actions.CountByStatus()
	}
	h.RenderPage(w, "actions", map[string]interface{}{
		"Title":      "Actions",
		"ActivePage": "actions",
		"Items":      items,
		"Counts":     counts,
		"Statuses":   actions.AllStatuses(),
	})
}
