package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/prototypes"
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

// RefinePrototype handles POST /api/prototypes/{id}/refine. Runs
// synthesis first, archives the prior version only once the new
// content is in hand, then writes v{n+1} in place. Accepts optional
// {"hints": "...", "questions": [...], "answers": [...]} — hints are
// the primary input ("make the header bigger") while the Q&A flow
// lets Claude ask for clarifications first.
func (h *Handler) RefinePrototype(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil || h.PrototypeSynth == nil {
		http.Error(w, "prototype store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")

	// Find the existing prototype to copy its source metadata.
	list, err := h.Prototypes.List()
	if err != nil {
		internalError(w, "prototype.refine.list", err)
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

	// Parse optional body for hints/answers. EOF is fine — empty body
	// means "refine with no hints", which the button supports. Any
	// other decode error is a client bug and we surface 400 instead
	// of silently dropping the hints and running a blank refine.
	var body prototypes.RefineRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Defence in depth: the stored format came from disk; if someone
	// hand-edited the frontmatter to an unknown value, refuse to
	// synthesize rather than silently falling back to ScreenFlow.
	if !h.PrototypeSynth.Registry().Has(existing.Format) {
		http.Error(w, "prototype format is unknown: "+existing.Format+"; valid: "+strings.Join(h.PrototypeSynth.Registry().Names(), ", "), http.StatusBadRequest)
		return
	}

	// Filter SourceRefs to the ones that still exist in the vault. A
	// moved/renamed/deleted source shouldn't stop the refine — the
	// previous output still carries that content — but we log per
	// missing path so the user can see why a refine might have run
	// with less context than the original synthesis.
	sources := make([]string, 0, len(existing.SourceRefs))
	for _, ref := range existing.SourceRefs {
		if !h.Vault.Exists(ref) {
			slog.Warn("prototype.refine: source missing", "prototype_id", id, "path", ref)
			continue
		}
		sources = append(sources, ref)
	}

	// Pick the previous output to show Claude. Raw formats carry the
	// rendered HTML body; structured formats carry the spec markdown.
	// Either one gives Claude a concrete reference to modify instead
	// of re-generating blind.
	prev := existing.HTMLBody
	if prev == "" {
		prev = existing.Spec
	}

	req := prototypes.CreateRequest{
		Title:          existing.Title,
		SourcePaths:    sources,
		Format:         existing.Format,
		Hints:          body.Hints,
		SourceThread:   existing.SourceThread,
		ProjectIDs:     existing.Projects,
		Questions:      body.Questions,
		Answers:        body.Answers,
		PreviousOutput: prev,
		IsRefine:       true,
	}

	// Synthesize FIRST. The old ordering archived the live version
	// before the Claude call, which on synthesis failure left a ghost
	// archive identical to the still-live copy. Running synthesis
	// first means a failure leaves the prototype state completely
	// untouched.
	p, err := h.PrototypeSynth.Synthesize(r.Context(), h.Vault, req)
	if err != nil {
		var invalidPaths *prototypes.InvalidSourcePathsError
		if errors.As(err, &invalidPaths) {
			slog.Info("prototype.refine: bad source paths", "failures", invalidPaths.Failures)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		internalError(w, "prototype.refine.synthesize", err)
		return
	}

	// Archive the prior version now that the new content is in hand.
	// Uses the pre-refine version number (n); after this call the
	// archive at v{n} contains what was the live copy and the live
	// files will move to v{n+1} via Write below.
	if err := h.Prototypes.ArchiveCurrent(existing, body.Hints); err != nil {
		internalError(w, "prototype.refine.archive", err)
		return
	}

	// Preserve identity / folder so the URL stays stable, and bump
	// the version counter. Synthesize returns a fresh Prototype with
	// Version=1 by default, so we clobber it.
	p.ID = existing.ID
	p.Created = existing.Created
	p.FolderPath = existing.FolderPath
	p.Version = existing.Version + 1

	if err := h.Prototypes.Write(p); err != nil {
		internalError(w, "prototype.refine.write", err)
		return
	}
	writeJSON(w, p)
}

// ListPrototypeVersions handles GET /api/prototypes/{id}/versions.
// Returns the archived versions (v1 … v{n-1}); the current live
// version lives on the prototype itself.
func (h *Handler) ListPrototypeVersions(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil {
		writeJSON(w, []prototypes.VersionInfo{})
		return
	}
	id := chi.URLParam(r, "id")
	versions, err := h.Prototypes.ListVersions(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, []prototypes.VersionInfo{})
			return
		}
		// `prototype not found` isn't os.ErrNotExist, so check by
		// string to return a 404 instead of 500.
		if err.Error() == "prototype not found: "+id {
			http.Error(w, "prototype not found", http.StatusNotFound)
			return
		}
		internalError(w, "prototype.versions.list", err)
		return
	}
	writeJSON(w, versions)
}

// GetPrototypeVersionSpec handles GET /api/prototypes/{id}/versions/{v}/spec.
// Returns the archived spec.md body as text/markdown so the frontend
// can render it read-only in the content panel.
func (h *Handler) GetPrototypeVersionSpec(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil {
		http.Error(w, "prototype store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	vStr := chi.URLParam(r, "v")
	v, err := parseVersion(vStr)
	if err != nil {
		http.Error(w, "invalid version: "+vStr, http.StatusBadRequest)
		return
	}
	body, err := h.Prototypes.ReadVersionSpec(id, v)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "version not found", http.StatusNotFound)
			return
		}
		internalError(w, "prototype.version.spec", err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

// GetPrototypeVersionHTML handles GET /api/prototypes/{id}/versions/{v}/html.
// Returns the archived prototype.html; used by the version-preview
// iframe on the item pipeline page. Mirrors GetPrototypeHTML's CSP.
func (h *Handler) GetPrototypeVersionHTML(w http.ResponseWriter, r *http.Request) {
	if h.Prototypes == nil {
		http.Error(w, "prototype store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	vStr := chi.URLParam(r, "v")
	v, err := parseVersion(vStr)
	if err != nil {
		http.Error(w, "invalid version: "+vStr, http.StatusBadRequest)
		return
	}
	body, err := h.Prototypes.ReadVersionHTML(id, v)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "version not found", http.StatusNotFound)
			return
		}
		internalError(w, "prototype.version.html", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

func parseVersion(raw string) (int, error) {
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid version")
	}
	return v, nil
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
	// Format validation: a typo in the request previously synthesized
	// as ScreenFlow via the Registry's silent fallback while persisting
	// the bad name — creating a mismatch between stored metadata and
	// rendered content. Reject unknown formats at the boundary.
	if !h.PrototypeSynth.Registry().Has(req.Format) {
		http.Error(w, "unknown format: "+req.Format+"; valid: "+strings.Join(h.PrototypeSynth.Registry().Names(), ", "), http.StatusBadRequest)
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
