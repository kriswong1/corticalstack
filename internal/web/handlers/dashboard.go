package handlers

import (
	"log/slog"
	"net/http"

	"github.com/kriswong/corticalstack/internal/dashboard"
)

// dashboardProvider is the subset of *dashboard.Cache that the handler
// needs. Defined as an interface so tests can pass a stub without
// constructing a real aggregator or vault.
type dashboardProvider interface {
	Snapshot() (*dashboard.Snapshot, error)
}

// GetDashboard handles GET /api/dashboard. Returns the full aggregated
// snapshot in one response. On a failed recompute with a cached snapshot
// available, the cache returns that cached snapshot with Stale=true and
// the handler still emits 200 — the frontend banner handles the rest.
// Only a failed recompute with no cache yet produces a 503.
func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	if h.Dashboard == nil {
		http.Error(w, "dashboard not configured", http.StatusServiceUnavailable)
		return
	}
	snap, err := h.Dashboard.Snapshot()
	if err != nil {
		slog.Error("dashboard snapshot", "error", err)
		http.Error(w, "dashboard: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, snap)
}
