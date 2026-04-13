package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/kriswong/corticalstack/internal/prds"
)

// PRDsPage renders /prds.
func (h *Handler) PRDsPage(w http.ResponseWriter, r *http.Request) {
	var list []*prds.PRD
	if h.PRDs != nil {
		var err error
		list, err = h.PRDs.List()
		if err != nil {
			slog.Warn("listing prds", "error", err)
		}
	}
	h.RenderPage(w, "prds", map[string]interface{}{
		"Title":      "PRDs",
		"ActivePage": "prds",
		"PRDs":       list,
	})
}

// ListPRDs returns every PRD as JSON.
func (h *Handler) ListPRDs(w http.ResponseWriter, r *http.Request) {
	if h.PRDs == nil {
		writeJSON(w, []any{})
		return
	}
	list, err := h.PRDs.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// QuestionsForPRD handles POST /api/prds/questions.
func (h *Handler) QuestionsForPRD(w http.ResponseWriter, r *http.Request) {
	if h.PRDs == nil || h.PRDSynth == nil {
		http.Error(w, "prd store not configured", http.StatusServiceUnavailable)
		return
	}
	var req prds.QuestionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.PitchPath == "" {
		http.Error(w, "pitch_path required", http.StatusBadRequest)
		return
	}
	qs, err := h.PRDSynth.Questions(r.Context(), h.Vault, req)
	if err != nil {
		slog.Error("prd questions", "error", err)
		http.Error(w, "questions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"questions": qs})
}

// CreatePRD handles POST /api/prds.
func (h *Handler) CreatePRD(w http.ResponseWriter, r *http.Request) {
	if h.PRDs == nil || h.PRDSynth == nil {
		http.Error(w, "prd store not configured", http.StatusServiceUnavailable)
		return
	}
	var req prds.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.PitchPath == "" {
		http.Error(w, "pitch_path required", http.StatusBadRequest)
		return
	}
	p, err := h.PRDSynth.Synthesize(r.Context(), h.Vault, req)
	if err != nil {
		slog.Error("prd synthesize", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.PRDs.Write(p); err != nil {
		slog.Error("prd write", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}
