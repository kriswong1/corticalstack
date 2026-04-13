package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/prototypes"
)

// PrototypesPage renders /prototypes with the list and create form.
func (h *Handler) PrototypesPage(w http.ResponseWriter, r *http.Request) {
	var list []*prototypes.Prototype
	if h.Prototypes != nil {
		var err error
		list, err = h.Prototypes.List()
		if err != nil {
			slog.Warn("listing prototypes", "error", err)
		}
	}
	formats := []string{}
	if h.PrototypeSynth != nil {
		formats = h.PrototypeSynth.Registry().Names()
	}
	h.RenderPage(w, "prototypes", map[string]interface{}{
		"Title":      "Prototypes",
		"ActivePage": "prototypes",
		"Prototypes": list,
		"Formats":    formats,
	})
}

// ListPrototypes returns every prototype as JSON.
func (h *Handler) ListPrototypes(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil {
		writeJSON(w, []any{})
		return
	}
	list, err := h.Prototypes.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// QuestionsForPrototype handles POST /api/prototypes/questions. It returns
// clarifying questions Claude wants answered before synthesis.
func (h *Handler) QuestionsForPrototype(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil || h.PrototypeSynth == nil {
		http.Error(w, "prototype store not configured", http.StatusServiceUnavailable)
		return
	}
	var req prototypes.QuestionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	qs, err := h.PrototypeSynth.Questions(r.Context(), h.Vault, req)
	if err != nil {
		slog.Error("prototype questions", "error", err)
		http.Error(w, "questions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"questions": qs})
}

// ViewPrototypeHTML serves the prototype.html for an interactive-html
// prototype. URL: GET /api/prototypes/{id}/html.
func (h *Handler) ViewPrototypeHTML(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil {
		http.Error(w, "prototype store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	body, _, err := h.Prototypes.ReadHTML(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "this prototype has no HTML file", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Allow the HTML to run inline scripts (it's user-generated content from
	// their own Claude session served on localhost).
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(body)
}

// CreatePrototype handles POST /api/prototypes.
func (h *Handler) CreatePrototype(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil || h.PrototypeSynth == nil {
		http.Error(w, "prototype store not configured", http.StatusServiceUnavailable)
		return
	}
	var req prototypes.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.SourcePaths) == 0 {
		http.Error(w, "source_paths required", http.StatusBadRequest)
		return
	}
	p, err := h.PrototypeSynth.Synthesize(r.Context(), h.Vault, req)
	if err != nil {
		var invalidPaths *prototypes.InvalidSourcePathsError
		if errors.As(err, &invalidPaths) {
			slog.Info("prototype: bad source paths", "failures", invalidPaths.Failures)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slog.Error("prototype synthesize", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Prototypes.Write(p); err != nil {
		slog.Error("prototype write", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}
