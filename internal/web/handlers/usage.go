package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/telemetry"
)

// usageProvider is the subset of *telemetry.Reader the handlers need.
// Defined here as an interface so tests can inject a stub without
// touching the filesystem. Mirrors dashboardProvider in dashboard.go.
type usageProvider interface {
	Recent(limit int) ([]agent.Invocation, error)
	Summary(window time.Duration) (telemetry.Summary, error)
}

// GetUsageRecent returns the N most recent Claude CLI invocations as a
// bare JSON array, newest first. Default limit is 50; max is 1000 to
// keep the response bounded. ?limit=N overrides the default.
//
// Returns 503 if the usage provider isn't wired up (matches the
// dashboard handler's nil-guard semantic — fail loudly).
func (h *Handler) GetUsageRecent(w http.ResponseWriter, r *http.Request) {
	if h.Usage == nil {
		http.Error(w, "usage not configured", http.StatusServiceUnavailable)
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		if n > 1000 {
			n = 1000
		}
		limit = n
	}

	invs, err := h.Usage.Recent(limit)
	if err != nil {
		serviceUnavailable(w, "usage.recent", err)
		return
	}
	// Always return a non-nil slice so JSON encodes as [] not null.
	if invs == nil {
		invs = []agent.Invocation{}
	}
	writeJSON(w, invs)
}

// GetUsageSummary returns aggregated totals over the trailing window.
// ?window=24h overrides the default 24h. Accepts any string parseable
// by time.ParseDuration ("1h", "30m", "168h", etc.).
func (h *Handler) GetUsageSummary(w http.ResponseWriter, r *http.Request) {
	if h.Usage == nil {
		http.Error(w, "usage not configured", http.StatusServiceUnavailable)
		return
	}

	window := 24 * time.Hour
	if v := r.URL.Query().Get("window"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			http.Error(w, "window must be a positive duration (e.g. 24h, 7d → 168h)", http.StatusBadRequest)
			return
		}
		window = d
	}

	summary, err := h.Usage.Summary(window)
	if err != nil {
		serviceUnavailable(w, "usage.summary", err)
		return
	}
	writeJSON(w, summary)
}
