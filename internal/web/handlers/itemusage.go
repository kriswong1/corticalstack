package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/itemusage"
	"github.com/kriswong/corticalstack/internal/stage"
)

// itemUsageProvider is the subset of *itemusage.Reader the handler
// uses. Defined as an interface so tests can pass a stub. Mirrors
// the dashboardProvider / usageProvider / meetingsProvider pattern.
type itemUsageProvider interface {
	Aggregate(itemusage.Filter) (itemusage.Aggregate, error)
}

// GetItemUsage handles GET /api/items/{type}/usage.
//
// Query parameters:
//   - ids:    comma-separated item IDs to filter by; empty means
//             "aggregate across all items of this type"
//   - window: duration string parseable by time.ParseDuration (e.g.
//             "24h", "168h"); empty means "all time"
//
// The {type} URL parameter must be one of: product, meeting,
// document, prototype. Anything else returns 400.
//
// Used by the dashboard card detail page: when the table has no
// selection it calls without ids (type-level aggregate); when rows
// are selected it calls with ids=a,b,c (filtered aggregate). The
// frontend's useQuery hook re-runs whenever the selection changes.
func (h *Handler) GetItemUsage(w http.ResponseWriter, r *http.Request) {
	if h.ItemUsage == nil {
		// Empty aggregate is a parseable response; the frontend
		// renders zeros without erroring. Mirrors the meetings/
		// documents nil-provider behavior.
		writeJSON(w, itemusage.Aggregate{ByModel: map[string]itemusage.ModelTotals{}})
		return
	}
	itemType := chi.URLParam(r, "type")
	// AllStages returns nil for unknown entity types, so this is the
	// cheapest entity-type validation we have without duplicating
	// the canonical list here.
	if stage.AllStages(stage.EntityType(itemType)) == nil {
		http.Error(w, "unknown item type: "+itemType, http.StatusBadRequest)
		return
	}

	filter := itemusage.Filter{Type: itemType}

	// ids: split a single ?ids=a,b,c parameter. Also accept the
	// repeated form ?ids=a&ids=b for clients that prefer it.
	if rawIDs := r.URL.Query()["ids"]; len(rawIDs) > 0 {
		var ids []string
		for _, raw := range rawIDs {
			for _, id := range strings.Split(raw, ",") {
				id = strings.TrimSpace(id)
				if id != "" {
					ids = append(ids, id)
				}
			}
		}
		filter.IDs = ids
	}

	if win := r.URL.Query().Get("window"); win != "" {
		d, err := time.ParseDuration(win)
		if err != nil {
			http.Error(w, "invalid window: "+err.Error(), http.StatusBadRequest)
			return
		}
		filter.Window = d
	}

	agg, err := h.ItemUsage.Aggregate(filter)
	if err != nil {
		serviceUnavailable(w, "items.usage", err)
		return
	}
	if agg.ByModel == nil {
		agg.ByModel = map[string]itemusage.ModelTotals{}
	}
	writeJSON(w, agg)
}
