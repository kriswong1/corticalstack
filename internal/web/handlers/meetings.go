package handlers

import (
	"net/http"

	"github.com/kriswong/corticalstack/internal/meetings"
)

// meetingsProvider is the subset of *meetings.Store the handler uses.
// Defined as an interface so tests can pass a stub. Mirrors the
// dashboardProvider / usageProvider pattern.
type meetingsProvider interface {
	List() ([]*meetings.Meeting, error)
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
