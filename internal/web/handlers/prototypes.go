package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/kriswong/corticalstack/internal/prototypes"
)

// PrototypesPage renders /prototypes with the list and create form.
func (h *Handler) PrototypesPage(w http.ResponseWriter, r *http.Request) {
	var list []*prototypes.Prototype
	if h.Prototypes != nil {
		list, _ = h.Prototypes.List()
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Prototypes.Write(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}
