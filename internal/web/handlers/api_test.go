package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/vault"
)

// newAPITestHandler creates a Handler with a temp vault, real stores for
// projects/actions/persona, and a minimal integrations registry. It does
// NOT wire Pipeline or Jobs because the API endpoints under test here
// don't use them.
func newAPITestHandler(t *testing.T) (*Handler, *chi.Mux) {
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
	h.RenderPage = func(w http.ResponseWriter, _ string, _ map[string]interface{}) {
		w.WriteHeader(http.StatusOK)
	}

	r := chi.NewRouter()
	r.Get("/api/status", h.Status)
	r.Get("/api/integrations", h.IntegrationStatus)
	r.Get("/api/vault/tree", h.VaultTree)
	r.Get("/api/projects", h.ListProjects)
	r.Get("/api/projects/{id}", h.GetProject)
	r.Post("/api/projects", h.CreateProject)
	r.Get("/api/actions", h.ListActions)
	r.Get("/api/actions/counts", h.ActionCounts)
	r.Get("/api/persona/{name}", h.GetPersona)
	r.Post("/api/persona/{name}", h.SavePersona)

	return h, r
}

// --- Status ---

// TestStatusReturns200WithExpectedFields exercises the full Status handler.
// Status() calls h.Pipeline.ListTransformers() and ListDestinations(),
// which requires a non-nil Pipeline. Pipeline construction needs external
// tooling (claude CLI, deepgram), so this test skips when Pipeline is nil.
func TestStatusReturns200WithExpectedFields(t *testing.T) {
	h, r := newAPITestHandler(t)

	if h.Pipeline == nil {
		t.Skip("Status() requires a non-nil Pipeline; skipping")
	}

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	if resp["vault_path"] == nil || resp["vault_path"] == "" {
		t.Error("vault_path is empty")
	}
	if resp["server_time"] == nil || resp["server_time"] == "" {
		t.Error("server_time is empty")
	}
}

// --- IntegrationStatus ---

func TestIntegrationStatusReturnsJSONArray(t *testing.T) {
	_, r := newAPITestHandler(t)

	req := httptest.NewRequest("GET", "/api/integrations", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var list []integrations.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// With an empty registry, we expect an empty array.
	if list == nil {
		t.Error("expected non-nil array (even if empty)")
	}
}

// --- VaultTree ---

func TestVaultTreeReturnsNestedJSON(t *testing.T) {
	h, r := newAPITestHandler(t)

	// Seed the vault with a directory and a file.
	notesDir := filepath.Join(h.Vault.Path(), "notes")
	if err := os.MkdirAll(notesDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "hello.md"), []byte("# Hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/vault/tree", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var tree VaultTreeNode
	if err := json.Unmarshal(rec.Body.Bytes(), &tree); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !tree.IsDir {
		t.Error("root should be a directory")
	}

	// Find the notes directory in children.
	var notesNode *VaultTreeNode
	for _, child := range tree.Children {
		if child.Name == "notes" {
			notesNode = child
			break
		}
	}
	if notesNode == nil {
		t.Fatal("expected 'notes' directory in tree children")
	}
	if !notesNode.IsDir {
		t.Error("notes should be a directory")
	}

	// Find hello.md inside notes.
	var helloNode *VaultTreeNode
	for _, child := range notesNode.Children {
		if child.Name == "hello.md" {
			helloNode = child
			break
		}
	}
	if helloNode == nil {
		t.Fatal("expected 'hello.md' in notes children")
	}
	if helloNode.IsDir {
		t.Error("hello.md should not be a directory")
	}
	if helloNode.Path != "notes/hello.md" {
		t.Errorf("path = %q, want %q", helloNode.Path, "notes/hello.md")
	}
}

// --- ListProjects ---

func TestListProjectsReturnsArray(t *testing.T) {
	_, r := newAPITestHandler(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var list []*projects.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Empty store returns empty array.
	if len(list) != 0 {
		t.Errorf("len = %d, want 0", len(list))
	}
}

// --- GetProject ---

func TestGetProjectMissingReturns404(t *testing.T) {
	_, r := newAPITestHandler(t)

	req := httptest.NewRequest("GET", "/api/projects/nonexistent", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- CreateProject ---

func TestCreateProjectValidJSON(t *testing.T) {
	_, r := newAPITestHandler(t)

	body := `{"name":"Test Project","description":"A test project","tags":["test"]}`
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var p projects.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.ID == "" {
		t.Error("expected non-empty project ID")
	}
	if p.Name != "Test Project" {
		t.Errorf("name = %q, want %q", p.Name, "Test Project")
	}
	if p.Description != "A test project" {
		t.Errorf("description = %q", p.Description)
	}
}

func TestCreateProjectInvalidJSONReturns400(t *testing.T) {
	_, r := newAPITestHandler(t)

	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader("{not json"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateProjectEmptyNameReturns400(t *testing.T) {
	_, r := newAPITestHandler(t)

	body := `{"name":"","description":"no name"}`
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// --- ListActions ---

func TestListActionsReturnsArray(t *testing.T) {
	_, r := newAPITestHandler(t)

	req := httptest.NewRequest("GET", "/api/actions", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var list []*actions.Action
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// --- ActionCounts ---

func TestActionCountsReturnsStatusMap(t *testing.T) {
	_, r := newAPITestHandler(t)

	req := httptest.NewRequest("GET", "/api/actions/counts", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var counts map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &counts); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// --- GetPersona ---

func TestGetPersonaSoulReturnsJSON(t *testing.T) {
	_, r := newAPITestHandler(t)

	req := httptest.NewRequest("GET", "/api/persona/soul", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["name"] != "soul" {
		t.Errorf("name = %v, want %q", resp["name"], "soul")
	}
	if resp["file"] != "SOUL.md" {
		t.Errorf("file = %v, want %q", resp["file"], "SOUL.md")
	}
	if resp["content"] == nil {
		t.Error("content should not be nil")
	}
	budget, ok := resp["budget"].(float64)
	if !ok || budget <= 0 {
		t.Errorf("budget = %v, want positive number", resp["budget"])
	}
}

func TestGetPersonaInvalidReturns404(t *testing.T) {
	_, r := newAPITestHandler(t)

	req := httptest.NewRequest("GET", "/api/persona/invalid", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- SavePersona ---

func TestSavePersonaWithJSONBody(t *testing.T) {
	_, r := newAPITestHandler(t)

	body := `{"content":"# Updated soul\n\nNew content here."}`
	req := httptest.NewRequest("POST", "/api/persona/soul", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "saved" {
		t.Errorf("status = %q, want %q", resp["status"], "saved")
	}
	if resp["name"] != "soul" {
		t.Errorf("name = %q, want %q", resp["name"], "soul")
	}

	// Verify the content was actually saved by reading it back.
	getReq := httptest.NewRequest("GET", "/api/persona/soul", nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)

	var getResp map[string]interface{}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got, ok := getResp["content"].(string); !ok || got != "# Updated soul\n\nNew content here." {
		t.Errorf("saved content = %q", getResp["content"])
	}
}

func TestSavePersonaWithTextPlainBody(t *testing.T) {
	_, r := newAPITestHandler(t)

	content := "# Plain text soul content"
	req := httptest.NewRequest("POST", "/api/persona/soul", strings.NewReader(content))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "saved" {
		t.Errorf("status = %q, want %q", resp["status"], "saved")
	}
}

func TestSavePersonaInvalidNameReturns404(t *testing.T) {
	_, r := newAPITestHandler(t)

	req := httptest.NewRequest("POST", "/api/persona/bogus", strings.NewReader("data"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- ListActions nil guard ---

func TestListActionsNilStoreReturnsEmptyArray(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	h := &Handler{Vault: v}
	h.RenderPage = func(w http.ResponseWriter, _ string, _ map[string]interface{}) {
		w.WriteHeader(http.StatusOK)
	}

	req := httptest.NewRequest("GET", "/api/actions", nil)
	rec := httptest.NewRecorder()
	h.ListActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var list []actions.Action
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("len = %d, want 0", len(list))
	}
}

// --- ActionCounts nil guard ---

func TestActionCountsNilStoreReturnsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	h := &Handler{Vault: v}

	req := httptest.NewRequest("GET", "/api/actions/counts", nil)
	rec := httptest.NewRecorder()
	h.ActionCounts(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var counts map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &counts); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("len = %d, want 0", len(counts))
	}
}

// --- CreateProject then GetProject round-trip ---

func TestCreateThenGetProjectRoundTrip(t *testing.T) {
	_, r := newAPITestHandler(t)

	// Create a project.
	body := `{"name":"Round Trip","description":"rt"}`
	createReq := httptest.NewRequest("POST", "/api/projects", strings.NewReader(body))
	createRec := httptest.NewRecorder()
	r.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d (body: %s)", createRec.Code, createRec.Body.String())
	}

	var created projects.Project
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	// Fetch it back by ID.
	getReq := httptest.NewRequest("GET", "/api/projects/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Errorf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var fetched projects.Project
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("ID = %q, want %q", fetched.ID, created.ID)
	}
	if fetched.Name != "Round Trip" {
		t.Errorf("name = %q, want %q", fetched.Name, "Round Trip")
	}
}
