package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kriswong/corticalstack/internal/vault"
	"github.com/kriswong/corticalstack/internal/web/handlers"
)

// newTestServer wires a Server with a minimal Deps bundle. Handlers
// that dereference nil dependencies should not be exercised by these
// tests (they cover RenderPage + NewServer only).
func newTestServer(t *testing.T) *Server {
	t.Helper()
	deps := handlers.Deps{
		Vault: vault.New(t.TempDir()),
	}
	srv, err := NewServer(deps)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func TestNewServerReturnsNonNil(t *testing.T) {
	srv := newTestServer(t)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.Router == nil {
		t.Error("Router is nil")
	}
	if srv.Handler == nil {
		t.Error("Handler is nil")
	}
	if srv.tmpl == nil {
		t.Error("tmpl is nil")
	}
}

func TestNewServerRoutesRegistered(t *testing.T) {
	srv := newTestServer(t)
	// Hit the root redirect — minimal path, no dep dereference.
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}
	if loc := rec.Header().Get("Location"); loc != "/dashboard" {
		t.Errorf("redirect = %q, want /dashboard", loc)
	}
}

func TestRenderPageKnownTemplate(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.RenderPage(rec, "library", map[string]interface{}{
		"Title":      "Library",
		"ActivePage": "library",
		"VaultPath":  "/tmp/test-vault",
	})

	body := rec.Body.String()
	wantHas := []string{
		"<!doctype html>",
		"<title>CorticalStack — Library</title>",
		"CorticalStack",
		"class=\"nav-link nav-link-active\"", // library is the active page
		"/tmp/test-vault",
	}
	for _, sub := range wantHas {
		if !strings.Contains(body, sub) {
			t.Errorf("body missing %q", sub)
		}
	}
}

func TestRenderPageUnknownTemplate(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.RenderPage(rec, "nonexistent-template-xyz", map[string]interface{}{
		"Title": "X",
	})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "template error") {
		t.Errorf("body missing 'template error': %q", rec.Body.String())
	}
}

// TestRenderPageAutoEscapesXSS verifies the template system HTML-escapes
// user-controlled fields. This is the test coverage for the Phase C audit
// finding about server.go:59 — confirms the template.HTML() wrap is safe
// because the inner template already escapes.
func TestRenderPageAutoEscapesXSS(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()

	xssTitle := `<script>alert('xss-1')</script>`
	xssVaultPath := `<img src=x onerror=alert('xss-2')>`

	srv.RenderPage(rec, "library", map[string]interface{}{
		"Title":      xssTitle,
		"ActivePage": "library",
		"VaultPath":  xssVaultPath,
	})

	body := rec.Body.String()

	// Raw payloads must NOT appear literally.
	if strings.Contains(body, xssTitle) {
		t.Errorf("raw Title payload leaked into output — XSS risk")
	}
	if strings.Contains(body, xssVaultPath) {
		t.Errorf("raw VaultPath payload leaked into output — XSS risk")
	}

	// Escaped forms MUST appear.
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("Title was not HTML-escaped; body excerpt: %q", body[:min(len(body), 500)])
	}
	if !strings.Contains(body, "&lt;img") {
		t.Errorf("VaultPath was not HTML-escaped")
	}
}

// TestRenderPageTemplateHTMLWrapIsSafe documents and verifies that the
// template.HTML() wrap of the content buffer in RenderPage does not
// bypass escaping for data passed into the inner template. The inner
// template escapes first; only the final rendered bytes are marked
// HTML-safe when injected into the layout.
func TestRenderPageTemplateHTMLWrapIsSafe(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()

	srv.RenderPage(rec, "library", map[string]interface{}{
		"Title":      "Safe Title",
		"ActivePage": "library",
		"VaultPath":  `<b>bold attempt</b>`,
	})

	body := rec.Body.String()
	if strings.Contains(body, "<b>bold attempt</b>") {
		t.Error("raw <b> tag leaked — escaping bypassed")
	}
	if !strings.Contains(body, "&lt;b&gt;") {
		t.Error("expected escaped <b>")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
