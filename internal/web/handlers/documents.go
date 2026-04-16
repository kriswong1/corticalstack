package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/documents"
	"github.com/kriswong/corticalstack/internal/stage"
)

// documentsProvider is the subset of *documents.Store the handler
// uses. Defined as an interface so tests can pass a stub. Mirrors
// the dashboardProvider / usageProvider / meetingsProvider pattern.
type documentsProvider interface {
	List() ([]*documents.Document, error)
	Get(id string) (*documents.Document, error)
	SetStage(id string, target stage.Stage) error
	Create(title, content string) (*documents.Document, error)
}

// createDocumentRequest is the JSON body for POST /api/documents.
type createDocumentRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// stageRequest is the JSON body for POST /api/documents/{id}/stage,
// /api/meetings/{id}/stage, and /api/prototypes/{id}/stage. Shared so
// the three endpoints stay consistent.
type stageRequest struct {
	Stage string `json:"stage"`
}

// CreateDocument handles POST /api/documents with a JSON body of
// {"title": "...", "content": "..."}. Creates a new markdown file in
// vault/documents/ at stage=input.
func (h *Handler) CreateDocument(w http.ResponseWriter, r *http.Request) {
	if h.Documents == nil {
		http.Error(w, "documents store not configured", http.StatusServiceUnavailable)
		return
	}
	var req createDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	doc, err := h.Documents.Create(req.Title, req.Content)
	if err != nil {
		internalError(w, "documents.create", err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, doc)
}

// ListDocuments returns every document as JSON, newest first. A
// missing folder returns an empty array (not 503) so the dashboard
// renders cleanly on a fresh vault.
func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	if h.Documents == nil {
		writeJSON(w, []*documents.Document{})
		return
	}
	list, err := h.Documents.List()
	if err != nil {
		serviceUnavailable(w, "documents.list", err)
		return
	}
	if list == nil {
		list = []*documents.Document{}
	}
	writeJSON(w, list)
}

// GetDocument returns a single document by ID.
func (h *Handler) GetDocument(w http.ResponseWriter, r *http.Request) {
	if h.Documents == nil {
		http.Error(w, "documents store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	doc, err := h.Documents.Get(id)
	if err != nil {
		serviceUnavailable(w, "documents.get", err)
		return
	}
	if doc == nil {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	writeJSON(w, doc)
}

// SetDocumentStage handles POST /api/documents/{id}/stage with a
// JSON body of {"stage": "in_progress"}.
func (h *Handler) SetDocumentStage(w http.ResponseWriter, r *http.Request) {
	if h.Documents == nil {
		http.Error(w, "documents store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	var req stageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	target, err := stage.Parse(stage.EntityDocument, req.Stage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Documents.SetStage(id, target); err != nil {
		// Either an unknown id or a disk write failure. The first is
		// a 404, the second a 500 — but we don't have a typed error
		// to distinguish. Fall back to 400 so the UI surfaces a clear
		// message; logging the error gives the operator the detail.
		internalError(w, "documents.set_stage", err)
		return
	}
	writeJSON(w, map[string]string{"id": id, "stage": string(target)})
}
