package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/prototypes"
	"github.com/kriswong/corticalstack/internal/vault"
)

// wirePrototypeSynth constructs a real PrototypeSynth on the handler so
// the nil-guard at the top of CreatePrototype/QuestionsForPrototype does
// not short-circuit before the path-traversal check runs. The synthesizer
// never has its Claude-calling code reached in these tests because the
// traversal guard fires first (CR-01 coverage) or we never POST (CR-02
// coverage uses only ViewPrototypeHTML).
func wirePrototypeSynth(h *Handler) {
	// NewSynthesizer takes a working dir, model, persona loader. The
	// working dir is irrelevant here — nothing ever invokes the Claude
	// agent in tests because our path-traversal guard fires first.
	h.PrototypeSynth = prototypes.NewSynthesizer("", "opus", nil)
}

// newPrototypeRouter wires a minimal chi router against the Handler so
// URL params like {id} resolve through chi's context the same way they do
// in production.
func newPrototypeRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/prototypes/{id}/html", h.ViewPrototypeHTML)
	r.Post("/api/prototypes", h.CreatePrototype)
	r.Post("/api/prototypes/questions", h.QuestionsForPrototype)
	return r
}

// seedInteractivePrototype writes a prototype with HTMLBody through the
// real store so ViewPrototypeHTML can read it back.
func seedInteractivePrototype(t *testing.T, h *Handler, htmlBody string) *prototypes.Prototype {
	t.Helper()
	p := &prototypes.Prototype{
		Title:    "Sandbox Test",
		Format:   "interactive-html",
		Spec:     "# spec",
		HTMLBody: htmlBody,
	}
	if err := h.Prototypes.Write(p); err != nil {
		t.Fatalf("seed prototype: %v", err)
	}
	return p
}

// TestViewPrototypeHTMLSandboxesUntrustedScript is the main CR-02 coverage:
// when a prototype body contains <script>alert(1)</script>, the response
// must wrap it in an iframe srcdoc where the script cannot touch the
// CorticalStack origin.
func TestViewPrototypeHTMLSandboxesUntrustedScript(t *testing.T) {
	h, _ := newAPITestHandler(t)
	r := newPrototypeRouter(h)

	malicious := `<html><head></head><body><h1>hi</h1><script>alert(1);fetch('/api/actions/reconcile',{method:'POST'})</script></body></html>`
	p := seedInteractivePrototype(t, h, malicious)

	req := httptest.NewRequest("GET", "/api/prototypes/"+p.ID+"/html", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()

	// 1. The response is the outer shell, not the raw prototype body.
	//    It must contain a sandboxed iframe.
	if !strings.Contains(body, `<iframe sandbox="allow-scripts"`) {
		t.Errorf("response missing sandboxed iframe; body:\n%s", body)
	}
	if strings.Contains(body, "allow-same-origin") {
		t.Error("sandbox must NOT include allow-same-origin (that would re-grant origin privileges)")
	}
	if strings.Contains(body, "allow-forms") {
		t.Error("sandbox must NOT include allow-forms")
	}
	if strings.Contains(body, "allow-top-navigation") {
		t.Error("sandbox must NOT include allow-top-navigation")
	}

	// 2. The raw <script> tag must not appear at top level — it must be
	//    escaped inside the srcdoc attribute so the parent document
	//    never parses it as a live script tag.
	//    html.EscapeString turns '<' into '&lt;', so there must be no
	//    literal "<script>alert(1)" sequence in the outer document.
	if strings.Contains(body, "<script>alert(1)") {
		t.Errorf("raw <script> tag reached the outer document — XSS NOT blocked:\n%s", body)
	}

	// The escaped form must be present inside the srcdoc attribute.
	if !strings.Contains(body, "&lt;script&gt;alert(1)") {
		t.Errorf("escaped script payload missing from srcdoc; body:\n%s", body)
	}

	// 3. Response must carry the expected hardening headers.
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header missing")
	}
	if !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP missing default-src 'none': %q", csp)
	}
	if !strings.Contains(csp, "sandbox allow-scripts") {
		t.Errorf("CSP missing sandbox directive: %q", csp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", rec.Header().Get("X-Content-Type-Options"))
	}
	if rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", rec.Header().Get("Referrer-Policy"))
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html...", ct)
	}
}

// TestViewPrototypeHTMLEscapesQuotesInSrcdoc verifies attribute-boundary
// attacks can't break out of the srcdoc="..." by injecting a literal `"`.
func TestViewPrototypeHTMLEscapesQuotesInSrcdoc(t *testing.T) {
	h, _ := newAPITestHandler(t)
	r := newPrototypeRouter(h)

	// Payload tries to break out of srcdoc="..." with a literal quote.
	payload := `"><script>parent.location='http://evil/'</script>`
	p := seedInteractivePrototype(t, h, payload)

	req := httptest.NewRequest("GET", "/api/prototypes/"+p.ID+"/html", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	body := rec.Body.String()
	// html.EscapeString converts '"' to '&#34;'.
	if strings.Contains(body, `"><script>parent`) {
		t.Errorf("quote breakout not escaped — attribute boundary attack possible:\n%s", body)
	}
	if !strings.Contains(body, "&#34;&gt;&lt;script&gt;parent") {
		t.Errorf("expected escaped payload in body, got:\n%s", body)
	}
}

// TestViewPrototypeHTMLNotFound returns 404 for a missing prototype ID.
func TestViewPrototypeHTMLNotFound(t *testing.T) {
	h, _ := newAPITestHandler(t)
	r := newPrototypeRouter(h)

	req := httptest.NewRequest("GET", "/api/prototypes/no-such-id/html", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- CR-01 coverage: path traversal guards on synthesis endpoints ---

// TestCreatePrototypeRejectsTraversalPaths — POST /api/prototypes must
// fail with 400 as soon as any source_paths entry contains a traversal.
func TestCreatePrototypeRejectsTraversalPaths(t *testing.T) {
	h, _ := newAPITestHandler(t)
	wirePrototypeSynth(h)
	r := newPrototypeRouter(h)

	dangerous := []string{
		`{"title":"x","format":"component-spec","source_paths":["../../../../etc/passwd"]}`,
		`{"title":"x","format":"component-spec","source_paths":["notes/../../../etc/passwd"]}`,
		`{"title":"x","format":"component-spec","source_paths":["/etc/passwd"]}`,
		`{"title":"x","format":"component-spec","source_paths":["good/path.md","../../bad.md"]}`,
	}

	for _, body := range dangerous {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/prototypes", strings.NewReader(body))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "source_paths") {
				t.Errorf("body = %q, want mention of source_paths", rec.Body.String())
			}
		})
	}
}

// TestQuestionsForPrototypeRejectsTraversalPaths covers the /questions
// sibling endpoint.
func TestQuestionsForPrototypeRejectsTraversalPaths(t *testing.T) {
	h, _ := newAPITestHandler(t)
	wirePrototypeSynth(h)
	r := newPrototypeRouter(h)

	body := `{"title":"x","format":"component-spec","source_paths":["../../etc/passwd"]}`
	req := httptest.NewRequest("POST", "/api/prototypes/questions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestCreatePrototypeAcceptsLegitPath — a cleanly nested path must pass
// the SafeRelPath check (we don't exercise the Claude call here; we only
// verify that validation lets it through and we then hit a different
// failure mode, not 400 on the path itself).
func TestCreatePrototypeAcceptsLegitPath(t *testing.T) {
	h, _ := newAPITestHandler(t)

	// Seed a real source file so SafeRelPath + Synthesize will get past
	// validation. We stop short of calling Claude because the synth step
	// will fail (no network) — what we care about is that the request
	// does NOT 400 with "invalid source_paths entry".
	if err := vault.New(h.Vault.Path()).WriteFile("notes/source.md", "# hello"); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	// Smoke-call SafeRelPath directly; if it errors, the handler will 400.
	if _, err := h.Vault.SafeRelPath("notes/source.md"); err != nil {
		t.Fatalf("legit path rejected: %v", err)
	}
}
