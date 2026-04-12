// Package handlers contains HTTP handlers for the CorticalStack dashboard.
package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/prds"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/prototypes"
	"github.com/kriswong/corticalstack/internal/shapeup"
	"github.com/kriswong/corticalstack/internal/usecases"
	"github.com/kriswong/corticalstack/internal/vault"
	"github.com/kriswong/corticalstack/internal/web/jobs"
	"github.com/kriswong/corticalstack/internal/web/sse"
)

// pipelineInfo is the subset of *pipeline.Pipeline that handlers need.
type pipelineInfo interface {
	ListTransformers() []string
	ListDestinations() []string
}

// Deps bundles every optional dependency the handler struct uses. Grouped
// this way so `New` has a manageable signature instead of 12 arguments.
type Deps struct {
	Vault              *vault.Vault
	Pipeline           pipelineInfo
	Jobs               *jobs.Manager
	Bus                *sse.EventBus
	Registry           *integrations.Registry
	Projects           *projects.Store
	Actions            *actions.Store
	Persona            *persona.Loader
	PersonaInitCreated []persona.Name
	ShapeUp            *shapeup.Store
	ShapeUpAdvancer    *shapeup.Advancer
	UseCases           *usecases.Store
	UseCaseGen         *usecases.Generator
	Prototypes         *prototypes.Store
	PrototypeSynth     *prototypes.Synthesizer
	PRDs               *prds.Store
	PRDSynth           *prds.Synthesizer
}

// Handler bundles shared dependencies for all dashboard handlers.
type Handler struct {
	Vault    *vault.Vault
	Pipeline pipelineInfo
	Jobs     *jobs.Manager
	Bus      *sse.EventBus
	Registry *integrations.Registry
	Projects *projects.Store
	Actions  *actions.Store

	// Persona (SOUL/USER/MEMORY)
	Persona            *persona.Loader
	PersonaInitCreated []persona.Name // which files were bootstrapped this startup

	// v3 features
	ShapeUp         *shapeup.Store
	ShapeUpAdvancer *shapeup.Advancer
	UseCases        *usecases.Store
	UseCaseGen      *usecases.Generator
	Prototypes      *prototypes.Store
	PrototypeSynth  *prototypes.Synthesizer
	PRDs            *prds.Store
	PRDSynth        *prds.Synthesizer

	RenderPage func(w http.ResponseWriter, contentTemplate string, data map[string]interface{})
}

// New wires a handler struct (RenderPage is filled in later by the server).
func New(deps Deps) *Handler {
	return &Handler{
		Vault:              deps.Vault,
		Pipeline:           deps.Pipeline,
		Jobs:               deps.Jobs,
		Bus:                deps.Bus,
		Registry:           deps.Registry,
		Projects:           deps.Projects,
		Actions:            deps.Actions,
		Persona:            deps.Persona,
		PersonaInitCreated: deps.PersonaInitCreated,
		ShapeUp:            deps.ShapeUp,
		ShapeUpAdvancer:    deps.ShapeUpAdvancer,
		UseCases:           deps.UseCases,
		UseCaseGen:         deps.UseCaseGen,
		Prototypes:         deps.Prototypes,
		PrototypeSynth:     deps.PrototypeSynth,
		PRDs:               deps.PRDs,
		PRDSynth:           deps.PRDSynth,
	}
}

// --- Pages ---

// DashboardPage renders the main dashboard.
func (h *Handler) DashboardPage(w http.ResponseWriter, r *http.Request) {
	h.RenderPage(w, "dashboard", map[string]interface{}{
		"Title":       "Dashboard",
		"ActivePage":  "dashboard",
		"VaultPath":   h.Vault.Path(),
		"Transformers": h.Pipeline.ListTransformers(),
		"Destinations": h.Pipeline.ListDestinations(),
		"Integrations": h.Registry.Statuses(),
	})
}

// IngestPage renders the ingest form.
func (h *Handler) IngestPage(w http.ResponseWriter, r *http.Request) {
	h.RenderPage(w, "ingest", map[string]interface{}{
		"Title":       "Ingest",
		"ActivePage":  "ingest",
		"Transformers": h.Pipeline.ListTransformers(),
	})
}

// LibraryPage renders a view of the vault contents.
func (h *Handler) LibraryPage(w http.ResponseWriter, r *http.Request) {
	h.RenderPage(w, "library", map[string]interface{}{
		"Title":      "Library",
		"ActivePage": "library",
		"VaultPath":  h.Vault.Path(),
	})
}

// ConfigPage renders config settings.
func (h *Handler) ConfigPage(w http.ResponseWriter, r *http.Request) {
	h.RenderPage(w, "config", map[string]interface{}{
		"Title":       "Config",
		"ActivePage":  "config",
		"VaultPath":   h.Vault.Path(),
		"Integrations": h.Registry.Statuses(),
	})
}

// --- API: Ingest ---

type ingestTextRequest struct {
	Text  string `json:"text"`
	Title string `json:"title"`
}

type ingestURLRequest struct {
	URL string `json:"url"`
}

type ingestResponse struct {
	JobID string `json:"job_id"`
}

// IngestText handles POST /api/ingest/text with a JSON body.
func (h *Handler) IngestText(w http.ResponseWriter, r *http.Request) {
	var req ingestTextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}
	label := req.Title
	if label == "" {
		label = firstLine(req.Text, 60)
	}
	job, err := h.Jobs.Submit(label, &pipeline.RawInput{
		Kind:    pipeline.InputText,
		Content: []byte(req.Text),
		Title:   req.Title,
	})
	if err != nil {
		slog.Warn("ingest text: submit failed", "error", err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, ingestResponse{JobID: job.ID})
}

// IngestURL handles POST /api/ingest/url with a JSON body.
func (h *Handler) IngestURL(w http.ResponseWriter, r *http.Request) {
	var req ingestURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	job, err := h.Jobs.Submit(req.URL, &pipeline.RawInput{
		Kind: pipeline.InputURL,
		URL:  req.URL,
	})
	if err != nil {
		slog.Warn("ingest url: submit failed", "error", err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, ingestResponse{JobID: job.ID})
}

// IngestFile handles POST /api/ingest/file with a multipart upload.
func (h *Handler) IngestFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(config.MaxUploadBytes()); err != nil {
		http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		slog.Error("ingest file: reading upload", "error", err)
		http.Error(w, "reading upload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	job, err := h.Jobs.Submit(header.Filename, &pipeline.RawInput{
		Kind:     pipeline.InputFile,
		Filename: header.Filename,
		Content:  content,
		MIMEType: header.Header.Get("Content-Type"),
	})
	if err != nil {
		slog.Warn("ingest file: submit failed", "error", err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, ingestResponse{JobID: job.ID})
}

// --- API: Jobs ---

// ListJobs returns all tracked jobs as JSON.
func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.Jobs.List())
}

// GetJob returns a single job by ID.
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job := h.Jobs.Get(id)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, job)
}

// ConfirmJob resumes a job paused at awaiting_confirmation. The request
// body is a jobs.ConfirmPayload JSON object.
func (h *Handler) ConfirmJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload jobs.ConfirmPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Jobs.Confirm(id, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"status": "confirmed"})
}

// StreamJob streams SSE events to the client. The browser uses EventSource
// to subscribe. When the job has already completed, we send the final state
// as a single event and close.
func (h *Handler) StreamJob(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	id := chi.URLParam(r, "id")
	job := h.Jobs.Get(id)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	// Send the job's existing state immediately so late subscribers catch up.
	initial, err := sse.FormatSSE(sse.Event{Type: "job_snapshot", Data: job})
	if err == nil {
		if _, err := w.Write(initial); err != nil {
			return
		}
		flusher.Flush()
	}

	if job.Status == jobs.StatusCompleted || job.Status == jobs.StatusFailed {
		return
	}

	ch := h.Bus.Subscribe()
	defer h.Bus.Unsubscribe(ch)

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-ch:
			if !ok {
				return
			}
			// Only forward events about this specific job
			if evt, ok := event.Data.(jobs.Event); ok && evt.JobID != id {
				continue
			}
			buf, err := sse.FormatSSE(event)
			if err != nil {
				continue
			}
			if _, err := w.Write(buf); err != nil {
				return
			}
			flusher.Flush()
			if event.Type == "job_complete" || event.Type == "job_failed" {
				return
			}
		}
	}
}

// --- API: Vault browsing ---

// VaultTreeNode is a recursive entry for the vault tree API.
type VaultTreeNode struct {
	Name     string           `json:"name"`
	Path     string           `json:"path"`
	IsDir    bool             `json:"is_dir"`
	Children []*VaultTreeNode `json:"children,omitempty"`
}

// VaultTree returns the vault folder as a nested JSON tree.
func (h *Handler) VaultTree(w http.ResponseWriter, r *http.Request) {
	root := h.Vault.Path()
	tree, err := buildTree(root, "")
	if err != nil {
		http.Error(w, "reading vault: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, tree)
}

// VaultFile returns the raw content of a single file in the vault.
func (h *Handler) VaultFile(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if rel == "" {
		http.Error(w, "path query param required", http.StatusBadRequest)
		return
	}
	// Prevent traversal
	clean := filepath.Clean(rel)
	if strings.HasPrefix(clean, "..") || strings.Contains(clean, "..") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	content, err := h.Vault.ReadFile(clean)
	if err != nil {
		http.Error(w, "reading file: "+err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
}

// --- API: Integrations ---

// IntegrationStatus returns a JSON summary of registered integrations.
func (h *Handler) IntegrationStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.Registry.Statuses())
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func firstLine(text string, max int) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > max {
			return line[:max]
		}
		return line
	}
	return "Untitled"
}

func buildTree(root, rel string) (*VaultTreeNode, error) {
	full := filepath.Join(root, rel)
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	node := &VaultTreeNode{
		Name:  info.Name(),
		Path:  filepath.ToSlash(rel),
		IsDir: info.IsDir(),
	}
	if rel == "" {
		node.Name = filepath.Base(root)
	}
	if !info.IsDir() {
		return node, nil
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	// Sort: dirs first, then files, alphabetical within each group
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	for _, e := range entries {
		// Skip hidden files
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child, err := buildTree(root, filepath.Join(rel, e.Name()))
		if err != nil {
			continue
		}
		node.Children = append(node.Children, child)
	}
	return node, nil
}

// Status is a tiny health check for the dashboard.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"ok":            true,
		"vault_path":    h.Vault.Path(),
		"transformers":  h.Pipeline.ListTransformers(),
		"destinations":  h.Pipeline.ListDestinations(),
		"integrations":  h.Registry.Statuses(),
		"server_time":   time.Now().Format(time.RFC3339),
		"content_types": []string{"text", "file", "url"},
	})
}

