package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
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
		internalError(w, "prd.list", err)
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
	// Traversal guard — every user-supplied vault path must be validated
	// at the handler boundary so we return 400 before the synth store
	// reaches into the filesystem. See internal/vault.SafeRelPath and
	// docs/code-review-go.md CR-01.
	if _, err := h.Vault.SafeRelPath(req.PitchPath); err != nil {
		http.Error(w, "invalid pitch_path: "+err.Error(), http.StatusBadRequest)
		return
	}
	for _, p := range req.ExtraContextPaths {
		if _, err := h.Vault.SafeRelPath(p); err != nil {
			http.Error(w, "invalid extra_context_paths entry: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	qs, err := h.PRDSynth.Questions(r.Context(), h.Vault, req)
	if err != nil {
		internalError(w, "prd.questions", err)
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
	// Traversal guard at the handler boundary (CR-01).
	if _, err := h.Vault.SafeRelPath(req.PitchPath); err != nil {
		http.Error(w, "invalid pitch_path: "+err.Error(), http.StatusBadRequest)
		return
	}
	for _, p := range req.ExtraContextPaths {
		if _, err := h.Vault.SafeRelPath(p); err != nil {
			http.Error(w, "invalid extra_context_paths entry: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Phase 4 inheritance: if the caller didn't supply ProjectIDs, fall
	// back to the parent Pitch thread's Projects. Funnels through the
	// canonicalizer either way so we never write slugs or dangling UUIDs.
	var parentProjects []string
	if parent := findThreadByArtifactPath(h.ShapeUp, req.PitchPath); parent != nil {
		parentProjects = parent.Projects
	}
	req.ProjectIDs = resolveParentProjects(h.Projects, req.ProjectIDs, parentProjects)

	p, err := h.PRDSynth.Synthesize(r.Context(), h.Vault, req)
	if err != nil {
		internalError(w, "prd.synthesize", err)
		return
	}
	if err := h.PRDs.Write(p); err != nil {
		internalError(w, "prd.write", err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}

// RefinePRD handles POST /api/prds/{id}/refine. Runs synthesis with
// the previous PRD body shown as a reference, archives the prior
// version only after the new content is in hand, then overwrites the
// live file. Accepts optional {"hints": "...", "questions": [...],
// "answers": [...]} — hints are the primary input ("reframe the
// rollout phases around mobile first"); the Q&A flow is a secondary
// affordance for users who want Claude to surface what to clarify.
//
// Ordering mirrors RefinePrototype (#17): synthesize first, archive
// on success only, so a failed Claude call leaves the live PRD
// untouched and doesn't produce a ghost archive slot.
func (h *Handler) RefinePRD(w http.ResponseWriter, r *http.Request) {
	if h.PRDs == nil || h.PRDSynth == nil {
		http.Error(w, "prd store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}

	// Look up the existing PRD so we have the pitch path, context
	// refs, projects, and the body to show Claude as the reference.
	list, err := h.PRDs.List()
	if err != nil {
		internalError(w, "prd.refine.list", err)
		return
	}
	var existing *prds.PRD
	for _, p := range list {
		if p.ID == id {
			existing = p
			break
		}
	}
	if existing == nil {
		http.Error(w, "prd not found", http.StatusNotFound)
		return
	}

	// Parse optional body. EOF is fine (refine-without-hints is legal,
	// mirrors the prototype refine button). Any other decode error is
	// a client bug.
	var body prds.RefineRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Defence in depth for the pitch path — it came from disk, but
	// validate before passing to synthesis so a hand-edited frontmatter
	// with a traversal payload is rejected at the boundary.
	if _, err := h.Vault.SafeRelPath(existing.SourcePitch); err != nil {
		http.Error(w, "prd source_pitch is unsafe: "+err.Error(), http.StatusBadRequest)
		return
	}

	req := prds.CreateRequest{
		PitchPath:      existing.SourcePitch,
		ProjectIDs:     existing.Projects,
		Hints:          body.Hints,
		Questions:      body.Questions,
		Answers:        body.Answers,
		PreviousOutput: existing.Body,
		IsRefine:       true,
	}

	// Synthesize FIRST. On failure, the live PRD is untouched and no
	// archive is produced.
	refreshed, err := h.PRDSynth.Synthesize(r.Context(), h.Vault, req)
	if err != nil {
		internalError(w, "prd.refine.synthesize", err)
		return
	}

	// Archive the prior version now that the new content is in hand.
	if err := h.PRDs.ArchiveCurrent(existing, body.Hints); err != nil {
		internalError(w, "prd.refine.archive", err)
		return
	}

	// Preserve identity and path so the URL stays stable, bump version.
	// Synthesize returns a fresh PRD without an ID or path set — we
	// substitute the existing ones.
	refreshed.ID = existing.ID
	refreshed.Created = existing.Created
	refreshed.Path = existing.Path
	refreshed.Status = existing.Status
	refreshed.SourceThread = existing.SourceThread
	refreshed.Version = existing.Version + 1

	if err := h.PRDs.Write(refreshed); err != nil {
		internalError(w, "prd.refine.write", err)
		return
	}
	writeJSON(w, refreshed)
}

// ListPRDVersions handles GET /api/prds/{id}/versions. Returns the
// archived versions (v1 … v{n-1}); the current live version lives
// on the PRD itself.
func (h *Handler) ListPRDVersions(w http.ResponseWriter, r *http.Request) {
	if h.PRDs == nil {
		writeJSON(w, []prds.VersionInfo{})
		return
	}
	id := chi.URLParam(r, "id")
	versions, err := h.PRDs.ListVersions(id)
	if err != nil {
		internalError(w, "prd.versions.list", err)
		return
	}
	writeJSON(w, versions)
}

// GetPRDVersionBody handles GET /api/prds/{id}/versions/{v}. Returns
// the archived PRD body as text/markdown so the detail page's version
// switcher can render it read-only (parallel to the prototype
// GetPrototypeVersionSpec endpoint).
func (h *Handler) GetPRDVersionBody(w http.ResponseWriter, r *http.Request) {
	if h.PRDs == nil {
		http.Error(w, "prd store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	vStr := chi.URLParam(r, "v")
	v, err := parseVersion(vStr)
	if err != nil {
		http.Error(w, "invalid version: "+vStr, http.StatusBadRequest)
		return
	}
	body, err := h.PRDs.ReadVersion(id, v)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "version not found", http.StatusNotFound)
			return
		}
		internalError(w, "prd.version.body", err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

// SetPRDStatus handles POST /api/prds/{id}/status. Body is
// {"status": "draft|review|approved|shipped|archived"}.
func (h *Handler) SetPRDStatus(w http.ResponseWriter, r *http.Request) {
	if h.PRDs == nil {
		http.Error(w, "prd store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Status == "" {
		http.Error(w, "status required", http.StatusBadRequest)
		return
	}
	p, err := h.PRDs.SetStatus(id, prds.Status(body.Status))
	if err != nil {
		slog.Warn("setting prd status", "id", id, "status", body.Status, "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, p)
}
