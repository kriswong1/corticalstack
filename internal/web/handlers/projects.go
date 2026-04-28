package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/projects"
)

// ListProjects returns all projects as JSON.
func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.Projects.List())
}

// GetProject returns a single project by UUID or slug.
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

// UpdateProject handles PATCH /api/projects/{id}. Accepts a partial
// UpdateRequest. Renaming regenerates the slug and renames the on-disk
// folder, but the UUID stays stable so cross-references survive.
func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req projects.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	updated, err := h.Projects.Update(id, req)
	if err != nil {
		if errors.Is(err, projects.ErrProjectNotFound) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, projects.ErrProjectExists) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, updated)
}

// DeleteProject handles DELETE /api/projects/{id}. Soft-deletes by moving
// the project directory to vault/.trash/projects/<slug>-<unix>/. References
// in other notes' frontmatter are left alone — the canonicalizer drops them
// on next write, and restoring the project from .trash reanimates them.
func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Projects.Delete(id); err != nil {
		if errors.Is(err, projects.ErrProjectNotFound) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		internalError(w, "projects.delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetProjectContent fans out across every entity store and returns the
// items associated with this project. Powers the /projects/:id detail page.
func (h *Handler) GetProjectContent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.ProjectContent == nil {
		http.Error(w, "project content unavailable", http.StatusServiceUnavailable)
		return
	}
	content, ok := h.ProjectContent.ForProject(id)
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	writeJSON(w, content)
}

// SyncProjects handles POST /api/projects/sync — scans vault notes for
// project references and auto-creates any that don't exist in the store.
func (h *Handler) SyncProjects(w http.ResponseWriter, r *http.Request) {
	created, err := h.Projects.SyncFromVault()
	if err != nil {
		internalError(w, "projects.sync_from_vault", err)
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
