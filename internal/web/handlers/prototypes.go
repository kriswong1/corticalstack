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
	"github.com/kriswong/corticalstack/internal/questions"
	"github.com/kriswong/corticalstack/internal/stage"
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
		internalError(w, "prototype.list", err)
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
		internalError(w, "prototype.questions", err)
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
		// Non-NotExist errors here may be "permission denied: /abs/path/..."
		// from the filesystem — log server-side, return a generic 404 so we
		// don't leak the vault layout to the client.
		slog.Error("prototype.read_html", "id", id, "error", err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// The srcdoc iframe inherits the parent document's CSP, so we must
	// allow inline styles and scripts here. The iframe's HTML
	// sandbox="allow-scripts" (without allow-same-origin) is the real
	// security boundary — it gives the prototype a null origin so it
	// cannot access CorticalStack's cookies, localStorage, or APIs.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
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

// SetPrototypeStage handles POST /api/prototypes/{id}/stage with a
// JSON body of {"stage": "in_progress"}. Mirrors the documents and
// meetings stage endpoints — the dashboard's per-card UI uses all
// three to advance items through the per-pipeline flow.
func (h *Handler) SetPrototypeStage(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil {
		http.Error(w, "prototype store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	var req stageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	target, err := stage.Parse(stage.EntityPrototype, req.Stage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Prototypes.SetStage(id, target); err != nil {
		internalError(w, "prototypes.set_stage", err)
		return
	}
	writeJSON(w, map[string]string{"id": id, "stage": string(target)})
}

// RegeneratePrototype handles POST /api/prototypes/{id}/regenerate.
// It looks up the existing prototype's metadata, re-runs synthesis with
// the same source paths and format, and overwrites the files in-place.
// Accepts optional {"hints": "...", "questions": [...], "answers": [...]}
// to refine the output.
func (h *Handler) RegeneratePrototype(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil || h.PrototypeSynth == nil {
		http.Error(w, "prototype store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")

	// Find the existing prototype to copy its source metadata.
	list, err := h.Prototypes.List()
	if err != nil {
		internalError(w, "prototype.regenerate.list", err)
		return
	}
	var existing *prototypes.Prototype
	for _, p := range list {
		if p.ID == id {
			existing = p
			break
		}
	}
	if existing == nil {
		http.Error(w, "prototype not found", http.StatusNotFound)
		return
	}

	// Parse optional body for hints/answers.
	var body struct {
		Hints     string               `json:"hints,omitempty"`
		Questions []questions.Question `json:"questions,omitempty"`
		Answers   []questions.Answer   `json:"answers,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	req := prototypes.CreateRequest{
		Title:        existing.Title,
		SourcePaths:  existing.SourceRefs,
		Format:       existing.Format,
		Hints:        body.Hints,
		SourceThread: existing.SourceThread,
		ProjectIDs:   existing.Projects,
		Questions:    body.Questions,
		Answers:      body.Answers,
	}

	p, err := h.PrototypeSynth.Synthesize(r.Context(), h.Vault, req)
	if err != nil {
		internalError(w, "prototype.regenerate.synthesize", err)
		return
	}
	// Preserve the original ID and created date so the URL stays stable.
	p.ID = existing.ID
	p.Created = existing.Created
	p.FolderPath = existing.FolderPath

	if err := h.Prototypes.Write(p); err != nil {
		internalError(w, "prototype.regenerate.write", err)
		return
	}
	writeJSON(w, p)
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
		internalError(w, "prototype.synthesize", err)
		return
	}
	if err := h.Prototypes.Write(p); err != nil {
		internalError(w, "prototype.write", err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}
