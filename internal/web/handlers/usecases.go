package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/kriswong/corticalstack/internal/usecases"
)

// UseCasesPage renders the /usecases page with a list of all generated cases.
func (h *Handler) UseCasesPage(w http.ResponseWriter, r *http.Request) {
	var list []*usecases.UseCase
	if h.UseCases != nil {
		var err error
		list, err = h.UseCases.List()
		if err != nil {
			slog.Warn("listing usecases", "error", err)
		}
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
		internalError(w, "usecase.list", err)
		return
	}
	writeJSON(w, list)
}

// QuestionsFromDoc handles POST /api/usecases/from-doc/questions.
func (h *Handler) QuestionsFromDoc(w http.ResponseWriter, r *http.Request) {
	if h.UseCases == nil || h.UseCaseGen == nil {
		http.Error(w, "usecase store not configured", http.StatusServiceUnavailable)
		return
	}
	var req usecases.QuestionsFromDocRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SourcePath == "" {
		http.Error(w, "source_path required", http.StatusBadRequest)
		return
	}
	// Traversal guard (CR-01).
	if _, err := h.Vault.SafeRelPath(req.SourcePath); err != nil {
		http.Error(w, "invalid source_path: "+err.Error(), http.StatusBadRequest)
		return
	}
	qs, err := h.UseCaseGen.QuestionsFromDoc(r.Context(), h.Vault, req)
	if err != nil {
		internalError(w, "usecase.questions_from_doc", err)
		return
	}
	writeJSON(w, map[string]interface{}{"questions": qs})
}

// QuestionsFromText handles POST /api/usecases/from-text/questions.
func (h *Handler) QuestionsFromText(w http.ResponseWriter, r *http.Request) {
	if h.UseCases == nil || h.UseCaseGen == nil {
		http.Error(w, "usecase store not configured", http.StatusServiceUnavailable)
		return
	}
	var req usecases.QuestionsFromTextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	qs, err := h.UseCaseGen.QuestionsFromText(r.Context(), req)
	if err != nil {
		internalError(w, "usecase.questions_from_text", err)
		return
	}
	writeJSON(w, map[string]interface{}{"questions": qs})
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
	if req.SourcePath == "" {
		http.Error(w, "source_path required", http.StatusBadRequest)
		return
	}
	// Traversal guard (CR-01).
	if _, err := h.Vault.SafeRelPath(req.SourcePath); err != nil {
		http.Error(w, "invalid source_path: "+err.Error(), http.StatusBadRequest)
		return
	}
	cases, err := h.UseCaseGen.FromDoc(r.Context(), h.Vault, req)
	if err != nil {
		internalError(w, "usecase.from_doc", err)
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
		internalError(w, "usecase.from_text", err)
		return
	}
	written := writeAll(h.UseCases, cases)
	writeJSON(w, usecases.GenerateResponse{Created: written})
}

func writeAll(store *usecases.Store, cases []*usecases.UseCase) []*usecases.UseCase {
	out := make([]*usecases.UseCase, 0, len(cases))
	for _, c := range cases {
		if err := store.Write(c); err != nil {
			slog.Warn("usecase write skipped", "error", err)
			continue
		}
		out = append(out, c)
	}
	return out
}
