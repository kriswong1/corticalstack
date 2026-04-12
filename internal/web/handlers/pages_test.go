package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/vault"
)

// renderCall captures what RenderPage was called with.
type renderCall struct {
	Template string
	Data     map[string]interface{}
}

// newPageTestHandler builds a Handler with every dependency that page
// handlers touch. Pipeline is intentionally nil — page handlers that
// require it (DashboardPage, IngestPage) are tested conditionally.
func newPageTestHandler(t *testing.T) (*Handler, *chi.Mux, *renderCall) {
	t.Helper()
	dir := t.TempDir()
	v := vault.New(dir)

	ps := projects.New(v)
	as := actions.New(v)
	if err := as.Load(); err != nil {
		t.Fatalf("actions.Load: %v", err)
	}
	pl := persona.New(v)
	if _, err := pl.InitIfMissing(); err != nil {
		t.Fatalf("persona.InitIfMissing: %v", err)
	}

	reg := integrations.NewRegistry()

	h := &Handler{
		Vault:    v,
		Registry: reg,
		Projects: ps,
		Actions:  as,
		Persona:  pl,
	}

	captured := &renderCall{}
	h.RenderPage = func(w http.ResponseWriter, contentTemplate string, data map[string]interface{}) {
		captured.Template = contentTemplate
		captured.Data = data
		w.WriteHeader(http.StatusOK)
	}

	r := chi.NewRouter()
	r.Get("/library", h.LibraryPage)
	r.Get("/config", h.ConfigPage)
	r.Get("/projects", h.ProjectsPage)
	r.Get("/actions", h.ActionsPage)
	r.Get("/persona/{name}", h.PersonaEditorPage)

	return h, r, captured
}

func TestLibraryPageRendersCorrectTemplate(t *testing.T) {
	_, r, captured := newPageTestHandler(t)

	req := httptest.NewRequest("GET", "/library", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if captured.Template != "library" {
		t.Errorf("template = %q, want %q", captured.Template, "library")
	}
	if captured.Data["ActivePage"] != "library" {
		t.Errorf("ActivePage = %v, want %q", captured.Data["ActivePage"], "library")
	}
	if captured.Data["Title"] != "Library" {
		t.Errorf("Title = %v, want %q", captured.Data["Title"], "Library")
	}
	if captured.Data["VaultPath"] == nil || captured.Data["VaultPath"] == "" {
		t.Error("VaultPath should not be empty")
	}
}

func TestConfigPageRendersCorrectTemplate(t *testing.T) {
	_, r, captured := newPageTestHandler(t)

	req := httptest.NewRequest("GET", "/config", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if captured.Template != "config" {
		t.Errorf("template = %q, want %q", captured.Template, "config")
	}
	if captured.Data["ActivePage"] != "config" {
		t.Errorf("ActivePage = %v, want %q", captured.Data["ActivePage"], "config")
	}
	if captured.Data["Title"] != "Config" {
		t.Errorf("Title = %v, want %q", captured.Data["Title"], "Config")
	}
}

func TestProjectsPageRendersCorrectTemplate(t *testing.T) {
	_, r, captured := newPageTestHandler(t)

	req := httptest.NewRequest("GET", "/projects", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if captured.Template != "projects" {
		t.Errorf("template = %q, want %q", captured.Template, "projects")
	}
	if captured.Data["ActivePage"] != "projects" {
		t.Errorf("ActivePage = %v, want %q", captured.Data["ActivePage"], "projects")
	}
	if captured.Data["Title"] != "Projects" {
		t.Errorf("Title = %v, want %q", captured.Data["Title"], "Projects")
	}
	if captured.Data["Projects"] == nil {
		t.Error("Projects should not be nil")
	}
}

func TestActionsPageRendersCorrectTemplate(t *testing.T) {
	_, r, captured := newPageTestHandler(t)

	req := httptest.NewRequest("GET", "/actions", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if captured.Template != "actions" {
		t.Errorf("template = %q, want %q", captured.Template, "actions")
	}
	if captured.Data["ActivePage"] != "actions" {
		t.Errorf("ActivePage = %v, want %q", captured.Data["ActivePage"], "actions")
	}
	if captured.Data["Title"] != "Actions" {
		t.Errorf("Title = %v, want %q", captured.Data["Title"], "Actions")
	}
	if captured.Data["Statuses"] == nil {
		t.Error("Statuses should not be nil")
	}
}

func TestPersonaEditorPageRendersCorrectTemplate(t *testing.T) {
	_, r, captured := newPageTestHandler(t)

	req := httptest.NewRequest("GET", "/persona/soul", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if captured.Template != "persona_editor" {
		t.Errorf("template = %q, want %q", captured.Template, "persona_editor")
	}
	if captured.Data["ActivePage"] != "soul" {
		t.Errorf("ActivePage = %v, want %q", captured.Data["ActivePage"], "soul")
	}
	if captured.Data["Name"] != "soul" {
		t.Errorf("Name = %v, want %q", captured.Data["Name"], "soul")
	}
	if captured.Data["File"] != "SOUL.md" {
		t.Errorf("File = %v, want %q", captured.Data["File"], "SOUL.md")
	}
	budget, ok := captured.Data["Budget"].(int)
	if !ok || budget <= 0 {
		t.Errorf("Budget = %v, want positive int", captured.Data["Budget"])
	}
}

func TestPersonaEditorPageInvalidNameReturns404(t *testing.T) {
	_, r, _ := newPageTestHandler(t)

	req := httptest.NewRequest("GET", "/persona/bogus", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestPersonaEditorPageUserAndMemory(t *testing.T) {
	tests := []struct {
		name       string
		urlName    string
		wantFile   string
		wantTitle  string
	}{
		{"user", "user", "USER.md", "USER \u2014 Profile"},
		{"memory", "memory", "MEMORY.md", "MEMORY \u2014 Curated Index"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, r, captured := newPageTestHandler(t)

			req := httptest.NewRequest("GET", "/persona/"+tc.urlName, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if captured.Data["ActivePage"] != tc.urlName {
				t.Errorf("ActivePage = %v, want %q", captured.Data["ActivePage"], tc.urlName)
			}
			if captured.Data["File"] != tc.wantFile {
				t.Errorf("File = %v, want %q", captured.Data["File"], tc.wantFile)
			}
			if captured.Data["Title"] != tc.wantTitle {
				t.Errorf("Title = %v, want %q", captured.Data["Title"], tc.wantTitle)
			}
		})
	}
}

// DashboardPage and IngestPage require h.Pipeline (non-nil) to call
// ListTransformers/ListDestinations. Since Pipeline construction needs
// external tooling, we test these page handlers only when Pipeline is
// available. These tests document the expected template/ActivePage values.

func TestDashboardPageRequiresPipeline(t *testing.T) {
	h, _, captured := newPageTestHandler(t)

	if h.Pipeline == nil {
		t.Skip("DashboardPage requires non-nil Pipeline; skipping")
	}

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.DashboardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if captured.Template != "dashboard" {
		t.Errorf("template = %q, want %q", captured.Template, "dashboard")
	}
	if captured.Data["ActivePage"] != "dashboard" {
		t.Errorf("ActivePage = %v", captured.Data["ActivePage"])
	}
}

func TestIngestPageRequiresPipeline(t *testing.T) {
	h, _, captured := newPageTestHandler(t)

	if h.Pipeline == nil {
		t.Skip("IngestPage requires non-nil Pipeline; skipping")
	}

	req := httptest.NewRequest("GET", "/ingest", nil)
	rec := httptest.NewRecorder()
	h.IngestPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if captured.Template != "ingest" {
		t.Errorf("template = %q, want %q", captured.Template, "ingest")
	}
	if captured.Data["ActivePage"] != "ingest" {
		t.Errorf("ActivePage = %v", captured.Data["ActivePage"])
	}
}
