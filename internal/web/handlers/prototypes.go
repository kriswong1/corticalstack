package handlers

import (
	"encoding/json"
	"errors"
	"html"
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
	// Traversal guard (CR-01). Every source_paths entry must resolve
	// inside the vault root.
	for _, p := range req.SourcePaths {
		if _, err := h.Vault.SafeRelPath(p); err != nil {
			http.Error(w, "invalid source_paths entry: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	qs, err := h.PrototypeSynth.Questions(r.Context(), h.Vault, req)
	if err != nil {
		slog.Error("prototype questions", "error", err)
		http.Error(w, "questions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"questions": qs})
}

// escapeForSrcdoc escapes the prototype HTML for safe inclusion inside an
// HTML attribute value. html.EscapeString covers &, <, >, ', and " — which
// is exactly what an attribute delimited by " needs.
//
// The escaped output is injected into an <iframe srcdoc="..."> that carries
// sandbox="allow-scripts" — deliberately WITHOUT allow-same-origin,
// allow-forms, or allow-top-navigation — which gives the prototype a null
// origin. It can still run JS to demo interactivity, but it cannot:
//   - read cookies / localStorage from the CorticalStack origin
//   - fetch() any CorticalStack API as the user (ambient credentials are
//     unavailable from a null origin)
//   - navigate the parent frame
//
// See docs/code-review-go.md CR-02.
func escapeForSrcdoc(body []byte) string {
	return html.EscapeString(string(body))
}

// ViewPrototypeHTML serves the prototype.html for an interactive-html
// prototype, wrapped in a sandboxed iframe so untrusted script cannot reach
// CorticalStack's origin. URL: GET /api/prototypes/{id}/html. See CR-02.
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

	// Strict CSP on the outer shell. The inner iframe's srcdoc gets a
	// null origin from the sandbox attribute and is not governed by this
	// CSP, so we only need to lock down the shell itself. 'unsafe-inline'
	// on style-src covers the tiny <style> block above; everything else
	// is 'none'.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; frame-src 'self' data:; style-src 'unsafe-inline'; sandbox allow-scripts")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")

	// Build the shell with the escaped prototype HTML injected into the
	// srcdoc attribute. The %s is expanded with fmt.Fprintf so the
	// outer template literal itself is a compile-time constant.
	_, _ = w.Write([]byte("<!DOCTYPE html>\n"))
	_, _ = w.Write([]byte(`<html lang="en"><head><meta charset="utf-8"><title>Prototype preview</title>`))
	_, _ = w.Write([]byte(`<style>html,body{margin:0;padding:0;height:100%;background:#111}iframe{border:0;width:100%;height:100%;display:block;background:#fff}</style>`))
	_, _ = w.Write([]byte(`</head><body><iframe sandbox="allow-scripts" srcdoc="`))
	_, _ = w.Write([]byte(escapeForSrcdoc(body)))
	_, _ = w.Write([]byte(`"></iframe></body></html>`))
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
	// Traversal guard (CR-01).
	for _, p := range req.SourcePaths {
		if _, err := h.Vault.SafeRelPath(p); err != nil {
			http.Error(w, "invalid source_paths entry: "+err.Error(), http.StatusBadRequest)
			return
		}
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
