package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/workspaces"
)

// ListWorkspaces returns every workspace as JSON, sorted by name.
// GET /api/workspaces
func (h *Handler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if h.Workspaces == nil {
		writeJSON(w, []*workspaces.Workspace{})
		return
	}
	writeJSON(w, h.Workspaces.List())
}

// GetWorkspace returns a single workspace by UUID or slug.
func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Workspaces == nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	ws := h.Workspaces.Get(id)
	if ws == nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	writeJSON(w, ws)
}

// CreateWorkspace handles POST /api/workspaces.
func (h *Handler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	if h.Workspaces == nil {
		http.Error(w, "workspaces store not available", http.StatusServiceUnavailable)
		return
	}
	var req workspaces.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	ws, err := h.Workspaces.Create(req)
	if err != nil {
		if errors.Is(err, workspaces.ErrWorkspaceExists) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, ws)
}

// UpdateWorkspace handles PATCH /api/workspaces/{id}.
func (h *Handler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Workspaces == nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	var req workspaces.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	updated, err := h.Workspaces.Update(id, req)
	if err != nil {
		if errors.Is(err, workspaces.ErrWorkspaceNotFound) {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, workspaces.ErrWorkspaceExists) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, updated)
}

// DeleteWorkspace handles DELETE /api/workspaces/{id}.
func (h *Handler) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Workspaces == nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if err := h.Workspaces.Delete(id); err != nil {
		if errors.Is(err, workspaces.ErrWorkspaceNotFound) {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		internalError(w, "workspaces.delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// workspaceContent is the shape returned by GET /api/workspaces/{id}/content.
type workspaceContent struct {
	Workspace *workspaces.Workspace `json:"workspace"`
	Projects  []*linkedProject      `json:"projects"`
	Counts    workspaceCounts       `json:"counts"`
}

type workspaceCounts struct {
	Projects int `json:"projects"`
}

// GetWorkspaceContent fans out across the projects store and returns
// every project that links to this workspace.
func (h *Handler) GetWorkspaceContent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Workspaces == nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	ws := h.Workspaces.Get(id)
	if ws == nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	linked := []*linkedProject{}
	if h.Projects != nil {
		for _, p := range h.Projects.List() {
			if p.WorkspaceID == nil || *p.WorkspaceID != ws.UUID {
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
	writeJSON(w, workspaceContent{
		Workspace: ws,
		Projects:  linked,
		Counts:    workspaceCounts{Projects: len(linked)},
	})
}
