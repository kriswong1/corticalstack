package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/intent"
	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/vault"
	"github.com/kriswong/corticalstack/internal/web/jobs"
	"github.com/kriswong/corticalstack/internal/web/sse"
)

// --- Stubs matching jobs package interfaces (unexported, but structural) ---

type stubPipeline struct {
	transformFn func(input *pipeline.RawInput) (*pipeline.TextDocument, string, error)
}

func (s *stubPipeline) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, string, error) {
	if s.transformFn != nil {
		return s.transformFn(input)
	}
	return &pipeline.TextDocument{
		Source:  "text",
		Title:   "stub doc",
		Content: "stub content",
	}, "stub-transformer", nil
}

func (s *stubPipeline) ExtractAndRoute(ctx context.Context, doc *pipeline.TextDocument, cfg pipeline.ExtractionConfig, name string) *pipeline.ProcessResult {
	return &pipeline.ProcessResult{
		Document:    doc,
		Transformer: name,
		Outputs:     map[string]string{"vault-note": "notes/stub.md"},
	}
}

type stubClassifier struct{}

func (s *stubClassifier) Classify(ctx context.Context, doc *pipeline.TextDocument, _ []*projects.Project) (*intent.PreviewResult, error) {
	return &intent.PreviewResult{
		Intention:  intent.Learning,
		Confidence: 0.9,
		Summary:    "stub preview",
	}, nil
}

type stubProjectList struct{}

func (s *stubProjectList) List() []*projects.Project { return nil }
func (s *stubProjectList) EnsureExists(id string)     {}

// newTestHandler wires a Handler with tmpdir vault and a real jobs.Manager
// backed by stubs. Routes through the same chi router the server uses so
// chi URL params resolve correctly.
func newTestHandler(t *testing.T) (*Handler, *chi.Mux, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	v := vault.New(dir)
	bus := sse.NewEventBus()

	ctx, cancel := context.WithCancel(context.Background())
	jm := jobs.New(ctx, &stubPipeline{}, bus, &stubClassifier{}, &stubProjectList{})

	h := New(Deps{
		Vault: v,
		Jobs:  jm,
		Bus:   bus,
	})
	h.RenderPage = func(w http.ResponseWriter, _ string, _ map[string]interface{}) {
		w.WriteHeader(http.StatusOK)
	}

	// Minimal router mirroring the API routes under test.
	r := chi.NewRouter()
	r.Post("/api/ingest/text", h.IngestText)
	r.Post("/api/ingest/url", h.IngestURL)
	r.Post("/api/ingest/file", h.IngestFile)
	r.Get("/api/jobs", h.ListJobs)
	r.Get("/api/jobs/{id}", h.GetJob)
	r.Post("/api/jobs/{id}/confirm", h.ConfirmJob)
	r.Get("/api/vault/file", h.VaultFile)

	return h, r, cancel
}

// --- Ingest: text ---

func TestIngestTextHappyPath(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	body := `{"text":"some content to ingest","title":"My Title"}`
	req := httptest.NewRequest("POST", "/api/ingest/text", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.JobID == "" {
		t.Error("empty JobID")
	}
}

func TestIngestTextEmptyBodyRejected(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	body := `{"text":"","title":""}`
	req := httptest.NewRequest("POST", "/api/ingest/text", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "text is required") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestIngestTextMalformedJSON(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	req := httptest.NewRequest("POST", "/api/ingest/text", strings.NewReader("{not json"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestIngestTextAfterShutdownReturns503(t *testing.T) {
	h, r, cancel := newTestHandler(t)
	defer cancel()

	shutCtx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	_ = h.Jobs.Shutdown(shutCtx)

	body := `{"text":"content"}`
	req := httptest.NewRequest("POST", "/api/ingest/text", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// --- Ingest: URL ---

func TestIngestURLHappyPath(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	body := `{"url":"https://example.com/article"}`
	req := httptest.NewRequest("POST", "/api/ingest/url", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestIngestURLEmptyRejected(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	body := `{"url":""}`
	req := httptest.NewRequest("POST", "/api/ingest/url", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
}

// --- Ingest: File ---

func TestIngestFileHappyPath(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	// Build a minimal multipart body.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "test.md")
	part.Write([]byte("# Hello\n\nfile contents"))
	mw.Close()

	req := httptest.NewRequest("POST", "/api/ingest/file", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestIngestFileMissingFileField(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("notfile", "something")
	mw.Close()

	req := httptest.NewRequest("POST", "/api/ingest/file", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
}

// --- Jobs query ---

func TestGetJobMissing(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	req := httptest.NewRequest("GET", "/api/jobs/nonexistent-id", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetJobFound(t *testing.T) {
	h, r, cancel := newTestHandler(t)
	defer cancel()

	job, err := h.Jobs.Submit("test", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("hi")})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/jobs/"+job.ID, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got jobs.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != job.ID {
		t.Errorf("ID = %q, want %q", got.ID, job.ID)
	}
}

func TestListJobsReturnsArray(t *testing.T) {
	h, r, cancel := newTestHandler(t)
	defer cancel()

	_, _ = h.Jobs.Submit("a", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("a")})
	time.Sleep(20 * time.Millisecond)
	_, _ = h.Jobs.Submit("b", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("b")})

	req := httptest.NewRequest("GET", "/api/jobs", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	var list []jobs.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("list len = %d, want 2", len(list))
	}
}

func TestConfirmJobMalformedJSON(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	req := httptest.NewRequest("POST", "/api/jobs/some-id/confirm", strings.NewReader("{not json"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestConfirmJobMissingID(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	body := `{"intention":"learning","project_ids":[]}`
	req := httptest.NewRequest("POST", "/api/jobs/nonexistent/confirm", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Manager.Confirm returns an error which handler maps to 400.
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not found") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

// --- VaultFile: path traversal guard ---

func TestVaultFileHappyPath(t *testing.T) {
	h, r, cancel := newTestHandler(t)
	defer cancel()

	// Write a test file in the tmpdir vault.
	content := "# Test Note\n\nHello from the vault."
	path := filepath.Join(h.Vault.Path(), "test.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/vault/file?path=test.md", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != content {
		t.Errorf("body = %q, want %q", rec.Body.String(), content)
	}
}

func TestVaultFileMissingPathParam(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	req := httptest.NewRequest("GET", "/api/vault/file", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
}

// TestVaultFilePathTraversalBlocked verifies the audit finding in
// Phase C — the current defense against `..` in path queries.
func TestVaultFilePathTraversalBlocked(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	dangerousPaths := []string{
		"../../etc/passwd",
		"../secret.txt",
		"notes/../../etc/passwd",
		"foo/../../bar",
	}

	for _, p := range dangerousPaths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/vault/file?path="+p, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code == http.StatusOK {
				t.Errorf("path %q was NOT blocked (status %d) — path traversal possible", p, rec.Code)
			}
		})
	}
}

func TestVaultFileNonexistentReturnsNotFound(t *testing.T) {
	_, r, cancel := newTestHandler(t)
	defer cancel()

	req := httptest.NewRequest("GET", "/api/vault/file?path=does-not-exist.md", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
