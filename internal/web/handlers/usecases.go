package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/kriswong/corticalstack/internal/usecases"
)

// UseCasesPage renders the /usecases page with a list of all generated cases.
func (h *Handler) UseCasesPage(w http.ResponseWriter, r *http.Request) {
	var list []*usecases.UseCase
	if h.UseCases != nil {
		list, _ = h.UseCases.List()
	}
	h.RenderPage(w, "usecases", map[string]interface{}{
		"Title":      "Use Cases",
		"ActivePage": "usecases",
		"UseCases":   list,
	})
}

// ListUseCases returns every stored UseCase as JSON.
func (h *Handler) ListUseCases(w http.ResponseWriter, r *http.Request) {
	if h.UseCases == nil {
		writeJSON(w, []any{})
		return
	}
	list, err := h.UseCases.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// GenerateUseCasesFromDoc handles POST /api/usecases/from-doc.
func (h *Handler) GenerateUseCasesFromDoc(w http.ResponseWriter, r *http.Request) {
	if h.UseCases == nil || h.UseCaseGen == nil {
		http.Error(w, "usecase store not configured", http.StatusServiceUnavailable)
		return
	}
	var req usecases.FromDocRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	cases, err := h.UseCaseGen.FromDoc(r.Context(), h.Vault, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	written := writeAll(h.UseCases, cases)
	writeJSON(w, usecases.GenerateResponse{Created: written})
}

// GenerateUseCasesFromText handles POST /api/usecases/from-text.
func (h *Handler) GenerateUseCasesFromText(w http.ResponseWriter, r *http.Request) {
	if h.UseCases == nil || h.UseCaseGen == nil {
		http.Error(w, "usecase store not configured", http.StatusServiceUnavailable)
		return
	}
	var req usecases.FromTextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	cases, err := h.UseCaseGen.FromText(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	written := writeAll(h.UseCases, cases)
	writeJSON(w, usecases.GenerateResponse{Created: written})
}

func writeAll(store *usecases.Store, cases []*usecases.UseCase) []*usecases.UseCase {
	out := make([]*usecases.UseCase, 0, len(cases))
	for _, c := range cases {
		if err := store.Write(c); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}
