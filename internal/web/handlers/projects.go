package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/projects"
)

// ListProjects returns all projects as JSON.
func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.Projects.List())
}

// GetProject returns a single project by ID.
func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p := h.Projects.Get(id)
	if p == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	writeJSON(w, p)
}

// CreateProject handles POST /api/projects. Accepts JSON { name, description, tags }.
func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req projects.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	p, err := h.Projects.Create(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}

// SyncProjects handles POST /api/projects/sync — scans vault notes for
// project references and auto-creates any that don't exist in the store.
func (h *Handler) SyncProjects(w http.ResponseWriter, r *http.Request) {
	created, err := h.Projects.SyncFromVault()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"created":       created,
		"created_count": len(created),
	})
}

// ProjectsPage renders the projects page.
func (h *Handler) ProjectsPage(w http.ResponseWriter, r *http.Request) {
	h.RenderPage(w, "projects", map[string]interface{}{
		"Title":      "Projects",
		"ActivePage": "projects",
		"Projects":   h.Projects.List(),
	})
}
