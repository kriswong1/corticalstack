package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/shapeup"
)

// ShapeUpPage renders the /product page with the full list of threads.
func (h *Handler) ShapeUpPage(w http.ResponseWriter, r *http.Request) {
	var threads []*shapeup.Thread
	if h.ShapeUp != nil {
		threads, _ = h.ShapeUp.ListThreads()
	}
	h.RenderPage(w, "product", map[string]interface{}{
		"Title":      "Product",
		"ActivePage": "product",
		"Threads":    threads,
		"Stages":     shapeup.AllStages(),
	})
}

// ListShapeUpThreads returns every thread as JSON.
func (h *Handler) ListShapeUpThreads(w http.ResponseWriter, r *http.Request) {
	if h.ShapeUp == nil {
		writeJSON(w, []any{})
		return
	}
	threads, err := h.ShapeUp.ListThreads()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, threads)
}

// GetShapeUpThread returns a single thread by ID.
func (h *Handler) GetShapeUpThread(w http.ResponseWriter, r *http.Request) {
	if h.ShapeUp == nil {
		http.Error(w, "shapeup store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	thread, err := h.ShapeUp.GetThread(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, thread)
}

// CreateShapeUpIdea handles POST /api/shapeup/idea.
func (h *Handler) CreateShapeUpIdea(w http.ResponseWriter, r *http.Request) {
	if h.ShapeUp == nil {
		http.Error(w, "shapeup store not configured", http.StatusServiceUnavailable)
		return
	}
	var req shapeup.CreateIdeaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	art, err := h.ShapeUp.CreateRawIdea(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, art)
}

// AdvanceShapeUpThread handles POST /api/shapeup/threads/{id}/advance.
func (h *Handler) AdvanceShapeUpThread(w http.ResponseWriter, r *http.Request) {
	if h.ShapeUp == nil || h.ShapeUpAdvancer == nil {
		http.Error(w, "shapeup store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")

	var req shapeup.AdvanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !shapeup.IsValidStage(req.TargetStage) {
		http.Error(w, "invalid target stage: "+req.TargetStage, http.StatusBadRequest)
		return
	}

	thread, err := h.ShapeUp.GetThread(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if len(thread.Artifacts) == 0 {
		http.Error(w, "thread has no artifacts to advance from", http.StatusConflict)
		return
	}

	body, err := h.ShapeUpAdvancer.Advance(r.Context(), thread, shapeup.Stage(req.TargetStage), req.Hints)
	if err != nil {
		http.Error(w, "advance: "+err.Error(), http.StatusInternalServerError)
		return
	}

	newArtifact := &shapeup.Artifact{
		ID:       uuid.NewString(),
		Stage:    shapeup.Stage(req.TargetStage),
		Thread:   thread.ID,
		ParentID: thread.Artifacts[len(thread.Artifacts)-1].ID,
		Title:    fmt.Sprintf("%s · %s", thread.Title, req.TargetStage),
		Projects: thread.Projects,
		Status:   "draft",
		Created:  time.Now(),
		Body:     body,
	}
	if err := h.ShapeUp.WriteArtifact(newArtifact); err != nil {
		http.Error(w, "write: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, newArtifact)
}
