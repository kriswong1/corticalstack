// Package web hosts the CorticalStack Chi dashboard: templates, static
// assets, SSE streaming, and the HTTP handlers that drive the pipeline.
package web

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/web/handlers"
	mw "github.com/kriswong/corticalstack/internal/web/middleware"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server is the CorticalStack dashboard HTTP server.
type Server struct {
	Router  *chi.Mux
	Handler *handlers.Handler
	tmpl    *template.Template
}

// NewServer wires the dashboard server. All dependencies are passed via
// handlers.Deps so main.go constructs each store/synthesizer once and hands
// them over here as a bundle.
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

	r.Use(mw.Recovery)
	r.Use(mw.Logger)

	// Static files (embedded)
	staticContent, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))

	// Pages
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
	})
	r.Get("/dashboard", s.Handler.DashboardPage)
	r.Get("/ingest", s.Handler.IngestPage)
	r.Get("/library", s.Handler.LibraryPage)
	r.Get("/config", s.Handler.ConfigPage)
	r.Get("/projects", s.Handler.ProjectsPage)
	r.Get("/actions", s.Handler.ActionsPage)
	r.Get("/product", s.Handler.ShapeUpPage)
	r.Get("/usecases", s.Handler.UseCasesPage)
	r.Get("/prototypes", s.Handler.PrototypesPage)
	r.Get("/prds", s.Handler.PRDsPage)

	// API: status & integrations
	r.Get("/api/status", s.Handler.Status)
	r.Get("/api/integrations", s.Handler.IntegrationStatus)

	// API: actions
	r.Get("/api/actions", s.Handler.ListActions)
	r.Get("/api/actions/counts", s.Handler.ActionCounts)
	r.Post("/api/actions/{id}/status", s.Handler.SetActionStatus)
	r.Post("/api/actions/reconcile", s.Handler.ReconcileActions)

	// API: projects
	r.Get("/api/projects", s.Handler.ListProjects)
	r.Post("/api/projects", s.Handler.CreateProject)
	r.Get("/api/projects/{id}", s.Handler.GetProject)

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
	r.Post("/api/shapeup/threads/{id}/advance", s.Handler.AdvanceShapeUpThread)

	// API: use cases
	r.Get("/api/usecases", s.Handler.ListUseCases)
	r.Post("/api/usecases/from-doc", s.Handler.GenerateUseCasesFromDoc)
	r.Post("/api/usecases/from-text", s.Handler.GenerateUseCasesFromText)

	// API: prototypes
	r.Get("/api/prototypes", s.Handler.ListPrototypes)
	r.Post("/api/prototypes", s.Handler.CreatePrototype)

	// API: PRDs
	r.Get("/api/prds", s.Handler.ListPRDs)
	r.Post("/api/prds", s.Handler.CreatePRD)
}
