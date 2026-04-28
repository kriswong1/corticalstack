package handlers

import (
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/itemusage"
	"github.com/kriswong/corticalstack/internal/stage"
)

// CardDetail is the JSON returned by GET /api/cards/{type}. Combines
// the per-stage distribution, an aggregate usage for ALL items of
// that type (the default the frontend renders when nothing in the
// table is selected), and the items list itself for the table body.
//
// Per-item usage filtering happens via a separate
// /api/items/{type}/usage endpoint that the frontend re-fetches when
// the table selection changes — that keeps this endpoint simple and
// the selection-driven re-fetch fast.
type CardDetail struct {
	Type        string              `json:"type"`
	Label       string              `json:"label"`
	StageCounts []CardStageCount    `json:"stage_counts"`
	Aggregate   itemusage.Aggregate `json:"aggregate"`
	Items       []CardItem          `json:"items"`
}

// CardStageCount is one tile in the stage distribution row of the
// card detail page.
type CardStageCount struct {
	Stage string `json:"stage"`
	Count int    `json:"count"`
}

// CardItem is one row of the items table on the card detail page.
// ViewURL is the frontend route the View button should navigate to.
// Projects carries the entity's project UUIDs so the FE can filter by
// project (powers the shared <ProjectFilter /> on dashboard-card).
type CardItem struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Stage    string    `json:"stage"`
	Updated  time.Time `json:"updated,omitempty"`
	ViewURL  string    `json:"view_url"`
	Projects []string  `json:"projects,omitempty"`
}

// GetCardDetail handles GET /api/cards/{type}. The type URL param
// must be one of: product, meeting, document, prototype.
func (h *Handler) GetCardDetail(w http.ResponseWriter, r *http.Request) {
	rawType := chi.URLParam(r, "type")
	entityType := stage.EntityType(rawType)
	stages := stage.AllStages(entityType)
	if stages == nil {
		http.Error(w, "unknown card type: "+rawType, http.StatusBadRequest)
		return
	}

	detail := CardDetail{
		Type:        rawType,
		Label:       cardLabel(entityType),
		StageCounts: make([]CardStageCount, 0, len(stages)),
		Items:       []CardItem{},
		Aggregate:   itemusage.Aggregate{ByModel: map[string]itemusage.ModelTotals{}},
	}

	// Stage initialization: emit every stage with zero counts so the
	// frontend renders a stable column shape even when some stages
	// are empty.
	stageCount := make(map[stage.Stage]int, len(stages))
	stageOrder := make([]stage.Stage, 0, len(stages))
	for _, s := range stages {
		stageCount[s] = 0
		stageOrder = append(stageOrder, s)
	}

	switch entityType {
	case stage.EntityProduct:
		if h.ShapeUp != nil {
			threads, err := h.ShapeUp.ListThreads()
			if err != nil {
				slog.Warn("cards.product.list", "error", err)
			}
			for _, t := range threads {
				st := stage.Normalize(stage.EntityProduct, string(t.CurrentStage))
				stageCount[st]++
				updated := time.Time{}
				if len(t.Artifacts) > 0 {
					updated = t.Artifacts[len(t.Artifacts)-1].Created
				}
				detail.Items = append(detail.Items, CardItem{
					ID:       t.ID,
					Title:    t.Title,
					Stage:    string(st),
					Updated:  updated,
					ViewURL:  "/product?thread=" + t.ID,
					Projects: t.Projects,
				})
			}
		}

	case stage.EntityMeeting:
		if h.Meetings != nil {
			list, err := h.Meetings.List()
			if err != nil {
				slog.Warn("cards.meeting.list", "error", err)
			}
			for _, m := range list {
				st := stage.Normalize(stage.EntityMeeting, string(m.Stage))
				stageCount[st]++
				updated := m.Updated
				if updated.IsZero() {
					updated = m.Created
				}
				detail.Items = append(detail.Items, CardItem{
					ID:       m.ID,
					Title:    m.Title,
					Stage:    string(st),
					Updated:  updated,
					ViewURL:  "/meetings/" + m.ID,
					Projects: m.Projects,
				})
			}
		}

	case stage.EntityDocument:
		if h.Documents != nil {
			list, err := h.Documents.List()
			if err != nil {
				slog.Warn("cards.document.list", "error", err)
			}
			for _, d := range list {
				st := stage.Normalize(stage.EntityDocument, string(d.Stage))
				stageCount[st]++
				updated := d.Updated
				if updated.IsZero() {
					updated = d.Created
				}
				detail.Items = append(detail.Items, CardItem{
					ID:       d.ID,
					Title:    d.Title,
					Stage:    string(st),
					Updated:  updated,
					ViewURL:  "/documents/" + d.ID,
					Projects: d.Projects,
				})
			}
		}

	case stage.EntityPrototype:
		if h.Prototypes != nil {
			list, err := h.Prototypes.List()
			if err != nil {
				slog.Warn("cards.prototype.list", "error", err)
			}
			for _, p := range list {
				st := stage.Normalize(stage.EntityPrototype, string(p.Stage))
				if st == "" {
					st = stage.Normalize(stage.EntityPrototype, p.Status)
				}
				stageCount[st]++
				updated := p.Updated
				if updated.IsZero() {
					updated = p.Created
				}
				detail.Items = append(detail.Items, CardItem{
					ID:       p.ID,
					Title:    p.Title,
					Stage:    string(st),
					Updated:  updated,
					ViewURL:  "/prototypes?id=" + p.ID,
					Projects: p.Projects,
				})
			}
		}
	}

	// Stage counts in canonical order.
	for _, s := range stageOrder {
		detail.StageCounts = append(detail.StageCounts, CardStageCount{
			Stage: string(s),
			Count: stageCount[s],
		})
	}

	// Items newest first.
	sort.SliceStable(detail.Items, func(i, j int) bool {
		return detail.Items[i].Updated.After(detail.Items[j].Updated)
	})

	// Aggregate usage across all items of this type. The frontend
	// uses this as the default "nothing selected" state of the usage
	// card; selection-filtered aggregates are fetched separately
	// from /api/items/{type}/usage.
	if h.ItemUsage != nil {
		agg, err := h.ItemUsage.Aggregate(itemusage.Filter{Type: rawType})
		if err == nil {
			if agg.ByModel == nil {
				agg.ByModel = map[string]itemusage.ModelTotals{}
			}
			detail.Aggregate = agg
		}
	}

	writeJSON(w, detail)
}

// cardLabel returns the user-facing card title for an entity type.
// Kept here rather than in the stage package because labels are a
// presentation concern, not a data-model one.
func cardLabel(et stage.EntityType) string {
	switch et {
	case stage.EntityProduct:
		return "Product"
	case stage.EntityMeeting:
		return "Meetings"
	case stage.EntityDocument:
		return "Documents"
	case stage.EntityPrototype:
		return "Prototypes"
	}
	return string(et)
}

