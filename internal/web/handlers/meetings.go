package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/meetings"
	"github.com/kriswong/corticalstack/internal/stage"
)

// meetingsProvider is the subset of *meetings.Store the handler uses.
// Defined as an interface so tests can pass a stub. Mirrors the
// dashboardProvider / usageProvider pattern.
type meetingsProvider interface {
	List() ([]*meetings.Meeting, error)
	SetStage(id string, target stage.Stage) error
}

// ListMeetings handles GET /api/meetings, returning every meeting note
// in the vault as a bare JSON array, newest first. Returns an empty
// array (not 503) on a missing folder so the dashboard renders cleanly
// on a fresh vault.
func (h *Handler) ListMeetings(w http.ResponseWriter, r *http.Request) {
	if h.Meetings == nil {
		writeJSON(w, []*meetings.Meeting{})
		return
	}
	list, err := h.Meetings.List()
	if err != nil {
		serviceUnavailable(w, "meetings.list", err)
		return
	}
	if list == nil {
		list = []*meetings.Meeting{}
	}
	writeJSON(w, list)
}

// SetMeetingStage handles POST /api/meetings/{id}/stage with a JSON
// body of {"stage": "note"}. Mirrors the documents and prototypes
// stage endpoints — the dashboard's per-card UI uses all three to
// advance items through the per-pipeline flow.
func (h *Handler) SetMeetingStage(w http.ResponseWriter, r *http.Request) {
	if h.Meetings == nil {
		http.Error(w, "meetings store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	var req stageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	target, err := stage.Parse(stage.EntityMeeting, req.Stage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Meetings.SetStage(id, target); err != nil {
		internalError(w, "meetings.set_stage", err)
		return
	}
	writeJSON(w, map[string]string{"id": id, "stage": string(target)})
}
