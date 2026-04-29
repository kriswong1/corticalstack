package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/initiatives"
)

// ListInitiatives returns every initiative as JSON, sorted by name.
// GET /api/initiatives
func (h *Handler) ListInitiatives(w http.ResponseWriter, r *http.Request) {
	if h.Initiatives == nil {
		writeJSON(w, []*initiatives.Initiative{})
		return
	}
	writeJSON(w, h.Initiatives.List())
}

// GetInitiative returns a single initiative by UUID or slug.
// GET /api/initiatives/{id}
func (h *Handler) GetInitiative(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Initiatives == nil {
		http.Error(w, "initiative not found", http.StatusNotFound)
		return
	}
	i := h.Initiatives.Get(id)
	if i == nil {
		http.Error(w, "initiative not found", http.StatusNotFound)
		return
	}
	writeJSON(w, i)
}

// CreateInitiative handles POST /api/initiatives.
func (h *Handler) CreateInitiative(w http.ResponseWriter, r *http.Request) {
	if h.Initiatives == nil {
		http.Error(w, "initiatives store not available", http.StatusServiceUnavailable)
		return
	}
	var req initiatives.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Canonicalize parent reference (slug → UUID, drop dangling).
	if req.ParentInitiativeID != nil {
		canonical := initiatives.CanonicalizeInitiativeID(h.Initiatives, *req.ParentInitiativeID)
		if canonical == "" {
			req.ParentInitiativeID = nil
		} else {
			req.ParentInitiativeID = &canonical
		}
	}
	i, err := h.Initiatives.Create(req)
	if err != nil {
		if errors.Is(err, initiatives.ErrInitiativeExists) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, i)
}

// UpdateInitiative handles PATCH /api/initiatives/{id}.
func (h *Handler) UpdateInitiative(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Initiatives == nil {
		http.Error(w, "initiative not found", http.StatusNotFound)
		return
	}
	var req initiatives.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ParentInitiativeID != nil && *req.ParentInitiativeID != "" {
		canonical := initiatives.CanonicalizeInitiativeID(h.Initiatives, *req.ParentInitiativeID)
		if canonical == "" {
			http.Error(w, "unknown parent_initiative_id", http.StatusBadRequest)
			return
		}
		req.ParentInitiativeID = &canonical
	}
	updated, err := h.Initiatives.Update(id, req)
	if err != nil {
		if errors.Is(err, initiatives.ErrInitiativeNotFound) {
			http.Error(w, "initiative not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, initiatives.ErrInitiativeExists) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, updated)
}

// DeleteInitiative handles DELETE /api/initiatives/{id}.
func (h *Handler) DeleteInitiative(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Initiatives == nil {
		http.Error(w, "initiative not found", http.StatusNotFound)
		return
	}
	if err := h.Initiatives.Delete(id); err != nil {
		if errors.Is(err, initiatives.ErrInitiativeNotFound) {
			http.Error(w, "initiative not found", http.StatusNotFound)
			return
		}
		internalError(w, "initiatives.delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// initiativeContent is the shape returned by GET /api/initiatives/{id}/content
// — the initiative plus the projects linked to it. Mirrors ProjectContent but
// at the tier above; populates the initiative detail page.
type initiativeContent struct {
	Initiative *initiatives.Initiative `json:"initiative"`
	Projects   []*linkedProject        `json:"projects"`
	Counts     initiativeCounts        `json:"counts"`
}

type initiativeCounts struct {
	Projects int `json:"projects"`
}

type linkedProject struct {
	UUID        string `json:"uuid"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

// GetInitiativeContent fans out across the projects store and returns
// every project that links to this initiative. Powers the
// /initiatives/:id detail page.
// GET /api/initiatives/{id}/content
func (h *Handler) GetInitiativeContent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Initiatives == nil {
		http.Error(w, "initiative not found", http.StatusNotFound)
		return
	}
	i := h.Initiatives.Get(id)
	if i == nil {
		http.Error(w, "initiative not found", http.StatusNotFound)
		return
	}
	linked := []*linkedProject{}
	if h.Projects != nil {
		for _, p := range h.Projects.List() {
			if p.InitiativeID == nil || *p.InitiativeID != i.UUID {
				continue
			}
			linked = append(linked, &linkedProject{
				UUID:        p.UUID,
				Slug:        p.Slug,
				Name:        p.Name,
				Status:      string(p.Status),
				Description: p.Description,
			})
		}
		sort.Slice(linked, func(a, b int) bool { return linked[a].Name < linked[b].Name })
	}
	writeJSON(w, initiativeContent{
		Initiative: i,
		Projects:   linked,
		Counts:     initiativeCounts{Projects: len(linked)},
	})
}
