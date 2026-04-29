// Package web hosts the CorticalStack HTTP server: API handlers, SSE
// streaming, and the embedded React SPA.
package web

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/web/handlers"
	mw "github.com/kriswong/corticalstack/internal/web/middleware"
	"github.com/kriswong/corticalstack/internal/web/spa"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server is the CorticalStack HTTP server.
type Server struct {
	Router  *chi.Mux
	Handler *handlers.Handler
	tmpl    *template.Template
}

// NewServer wires the server. All dependencies are passed via handlers.Deps
// so main.go constructs each store/synthesizer once and hands them over.
func NewServer(deps handlers.Deps) (*Server, error) {
	tmpl, err := template.New("").ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	h := handlers.New(deps)

	s := &Server{
		Router:  chi.NewRouter(),
		Handler: h,
		tmpl:    tmpl,
	}
	h.RenderPage = s.RenderPage
	s.routes()
	return s, nil
}

// RenderPage renders a named content template inside the layout.
// Retained for handler compatibility and tests; pages are now served by the SPA.
func (s *Server) RenderPage(w http.ResponseWriter, contentTemplate string, data map[string]interface{}) {
	var contentBuf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&contentBuf, contentTemplate, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data["Content"] = template.HTML(contentBuf.String())
	if err := s.tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "layout error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) routes() {
	r := s.Router

	r.Use(mw.RequestID)
	r.Use(mw.Recovery)
	r.Use(mw.Logger)

	// API: status & integrations
	r.Get("/api/status", s.Handler.Status)
	r.Get("/api/integrations", s.Handler.IntegrationStatus)
	r.Get("/api/onboarding/status", s.Handler.OnboardingStatus)
	r.Post("/api/integrations/obsidian/test", s.Handler.TestObsidian)
	r.Post("/api/integrations/obsidian/save", s.Handler.SaveObsidian)
	r.Post("/api/integrations/deepgram/test", s.Handler.TestDeepgram)
	r.Post("/api/integrations/deepgram/save", s.Handler.SaveDeepgram)
	r.Get("/api/integrations/linear/status", s.Handler.LinearStatus)
	r.Get("/api/integrations/linear/teams", s.Handler.ListLinearTeams)
	r.Get("/api/integrations/linear/initiatives", s.Handler.ListLinearInitiatives)
	r.Get("/api/integrations/linear/projects", s.Handler.ListLinearProjects)
	r.Post("/api/integrations/linear/test", s.Handler.TestLinear)
	r.Post("/api/integrations/linear/save", s.Handler.SaveLinear)
	// Inbound webhook receiver. Mounted outside /api so the path
	// matches what users register in Linear's webhook UI.
	r.Post("/webhooks/linear", s.Handler.LinearWebhook)

	// API: dashboard operating view (single aggregator snapshot)
	r.Get("/api/dashboard", s.Handler.GetDashboard)

	// API: persona (SOUL / USER / MEMORY)
	r.Get("/api/persona/status", s.Handler.PersonaStatusAll)
	r.Post("/api/persona/setup", s.Handler.SetupPersona)
	r.Get("/api/persona/{name}", s.Handler.GetPersona)
	r.Post("/api/persona/{name}", s.Handler.SavePersona)
	r.Post("/api/persona/{name}/enhance", s.Handler.EnhancePersona)
	r.Post("/api/persona/{name}/enhance/questions", s.Handler.QuestionsForPersonaEnhance)
	r.Post("/api/persona/{name}/chat/start", s.Handler.StartPersonaChat)
	r.Post("/api/persona/{name}/chat/continue", s.Handler.ContinuePersonaChat)
	r.Post("/api/persona/{name}/chat/done", s.Handler.FinishPersonaChat)
	r.Post("/api/persona/{name}/chat/accept", s.Handler.AcceptPersonaChat)

	// API: actions
	r.Get("/api/actions", s.Handler.ListActions)
	r.Get("/api/actions/counts", s.Handler.ActionCounts)
	r.Put("/api/actions/{id}", s.Handler.UpdateAction)
	r.Post("/api/actions/{id}/status", s.Handler.SetActionStatus)
	r.Post("/api/actions/reconcile", s.Handler.ReconcileActions)

	// API: projects
	r.Get("/api/projects", s.Handler.ListProjects)
	r.Post("/api/projects", s.Handler.CreateProject)
	r.Post("/api/projects/sync", s.Handler.SyncProjects)
	r.Get("/api/projects/{id}", s.Handler.GetProject)
	r.Patch("/api/projects/{id}", s.Handler.UpdateProject)
	r.Delete("/api/projects/{id}", s.Handler.DeleteProject)
	r.Get("/api/projects/{id}/content", s.Handler.GetProjectContent)
	r.Get("/api/projects/{id}/canvas", s.Handler.GetProjectCanvas)
	r.Put("/api/projects/{id}/canvas", s.Handler.SetProjectCanvas)
	r.Post("/api/projects/{id}/sync", s.Handler.SyncProjectToLinear)
	r.Post("/api/projects/{id}/generate-issues-from-prd", s.Handler.GenerateIssuesFromPRD)

	// API: initiatives (L2 — strategic tier above Projects)
	r.Get("/api/initiatives", s.Handler.ListInitiatives)
	r.Post("/api/initiatives", s.Handler.CreateInitiative)
	r.Get("/api/initiatives/{id}", s.Handler.GetInitiative)
	r.Patch("/api/initiatives/{id}", s.Handler.UpdateInitiative)
	r.Delete("/api/initiatives/{id}", s.Handler.DeleteInitiative)
	r.Get("/api/initiatives/{id}/content", s.Handler.GetInitiativeContent)

	// API: usage telemetry
	r.Get("/api/usage/recent", s.Handler.GetUsageRecent)
	r.Get("/api/usage/summary", s.Handler.GetUsageSummary)

	// API: per-item usage (powers the dashboard card detail page)
	r.Get("/api/items/{type}/usage", s.Handler.GetItemUsage)

	// API: card detail (stage distribution + items table + aggregate
	// usage; one call powers the row-2 card drill-down page)
	r.Get("/api/cards/{type}", s.Handler.GetCardDetail)

	// API: meetings (transcript / audio / note pipeline)
	r.Get("/api/meetings", s.Handler.ListMeetings)
	r.Post("/api/meetings/{id}/stage", s.Handler.SetMeetingStage)

	// API: documents (input / note pipeline)
	r.Get("/api/documents", s.Handler.ListDocuments)
	r.Post("/api/documents", s.Handler.CreateDocument)
	r.Get("/api/documents/{id}", s.Handler.GetDocument)
	r.Post("/api/documents/{id}/stage", s.Handler.SetDocumentStage)

	// API: ingest
	r.Post("/api/ingest/text", s.Handler.IngestText)
	r.Post("/api/ingest/url", s.Handler.IngestURL)
	r.Post("/api/ingest/file", s.Handler.IngestFile)

	// API: jobs
	r.Get("/api/jobs", s.Handler.ListJobs)
	r.Get("/api/jobs/{id}", s.Handler.GetJob)
	r.Post("/api/jobs/{id}/confirm", s.Handler.ConfirmJob)
	r.Get("/api/jobs/{id}/stream", s.Handler.StreamJob)

	// API: vault browsing
	r.Get("/api/vault/tree", s.Handler.VaultTree)
	r.Get("/api/vault/file", s.Handler.VaultFile)

	// API: ShapeUp
	r.Get("/api/shapeup/threads", s.Handler.ListShapeUpThreads)
	r.Get("/api/shapeup/threads/{id}", s.Handler.GetShapeUpThread)
	r.Post("/api/shapeup/idea", s.Handler.CreateShapeUpIdea)
	r.Post("/api/shapeup/threads/{id}/questions", s.Handler.QuestionsForShapeUpThread)
	r.Post("/api/shapeup/threads/{id}/advance", s.Handler.AdvanceShapeUpThread)
	r.Get("/api/shapeup/threads/{id}/progress", s.Handler.GetAdvanceProgress)

	// API: use cases
	r.Get("/api/usecases", s.Handler.ListUseCases)
	r.Post("/api/usecases/from-doc", s.Handler.GenerateUseCasesFromDoc)
	r.Post("/api/usecases/from-text", s.Handler.GenerateUseCasesFromText)
	r.Post("/api/usecases/from-doc/questions", s.Handler.QuestionsFromDoc)
	r.Post("/api/usecases/from-text/questions", s.Handler.QuestionsFromText)

	// API: prototypes
	r.Get("/api/prototypes", s.Handler.ListPrototypes)
	r.Post("/api/prototypes", s.Handler.CreatePrototype)
	r.Post("/api/prototypes/questions", s.Handler.QuestionsForPrototype)
	r.Get("/api/prototypes/{id}/html", s.Handler.ViewPrototypeHTML)
	r.Post("/api/prototypes/{id}/stage", s.Handler.SetPrototypeStage)
	r.Post("/api/prototypes/{id}/refine", s.Handler.RefinePrototype)
	r.Get("/api/prototypes/{id}/versions", s.Handler.ListPrototypeVersions)
	r.Get("/api/prototypes/{id}/versions/{v}/spec", s.Handler.GetPrototypeVersionSpec)
	r.Get("/api/prototypes/{id}/versions/{v}/html", s.Handler.GetPrototypeVersionHTML)

	// API: PRDs
	r.Get("/api/prds", s.Handler.ListPRDs)
	r.Post("/api/prds", s.Handler.CreatePRD)
	r.Post("/api/prds/questions", s.Handler.QuestionsForPRD)
	r.Post("/api/prds/{id}/refine", s.Handler.RefinePRD)
	r.Get("/api/prds/{id}/versions", s.Handler.ListPRDVersions)
	r.Get("/api/prds/{id}/versions/{v}", s.Handler.GetPRDVersionBody)
	r.Post("/api/prds/{id}/status", s.Handler.SetPRDStatus)

	// SPA catch-all: serve Vite-built React app for all non-API routes.
	spaFS, _ := fs.Sub(spa.DistFS, "dist")
	r.Handle("/assets/*", http.StripPrefix("/", http.FileServer(http.FS(spaFS))))
	r.NotFound(spaHandler(spaFS))
}

// spaHandler serves index.html for any path not matched by API or asset
// routes, enabling React Router client-side navigation.
func spaHandler(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		// Try to serve the exact file (favicon.ico, robots.txt, etc.)
		if path != "" {
			if f, err := fsys.Open(path); err == nil {
				f.Close()
				http.FileServer(http.FS(fsys)).ServeHTTP(w, r)
				return
			}
		}
		// Fall back to index.html for SPA routing
		index, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.Error(w, "SPA not built — run: make ui-build", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	}
}
