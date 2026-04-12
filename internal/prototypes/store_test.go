package prototypes

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kriswong/corticalstack/internal/vault"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return New(vault.New(dir))
}

func TestEnsureFolder(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	dir := filepath.Join(s.vault.Path(), prototypesDir)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("prototypes dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("prototypes path is not a directory")
	}
}

func TestWriteGetRoundtrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	proto := &Prototype{
		Title:        "Dashboard screen flow",
		Format:       "screen-flow",
		SourceRefs:   []string{"product/pitch/2026-04-11_dark-mode.md"},
		SourceThread: "thread-abc",
		Projects:     []string{"corticalstack"},
		Spec:         "# Dashboard Screen Flow\n\nScreen 1: Login\nScreen 2: Dashboard",
	}

	if err := s.Write(proto); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if proto.ID == "" {
		t.Error("expected Write to assign an ID")
	}
	if proto.FolderPath == "" {
		t.Error("expected Write to set FolderPath")
	}
	if proto.Created.IsZero() {
		t.Error("expected Write to set Created")
	}
	if proto.Status != "draft" {
		t.Errorf("Status = %q, want %q", proto.Status, "draft")
	}

	// Verify spec.md exists
	specPath := filepath.Join(s.vault.Path(), proto.FolderPath, "spec.md")
	if _, err := os.Stat(specPath); err != nil {
		t.Fatalf("spec.md not found at %q: %v", specPath, err)
	}

	// Verify source-links.md exists
	linksPath := filepath.Join(s.vault.Path(), proto.FolderPath, "source-links.md")
	if _, err := os.Stat(linksPath); err != nil {
		t.Fatalf("source-links.md not found at %q: %v", linksPath, err)
	}

	// Read back via List (prototypes don't have a Get by path, they use List)
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 prototype, got %d", len(list))
	}
	got := list[0]
	if got.ID != proto.ID {
		t.Errorf("ID = %q, want %q", got.ID, proto.ID)
	}
	if got.Title != proto.Title {
		t.Errorf("Title = %q, want %q", got.Title, proto.Title)
	}
	if got.Format != proto.Format {
		t.Errorf("Format = %q, want %q", got.Format, proto.Format)
	}
	if got.Spec == "" {
		t.Error("expected non-empty Spec on read-back")
	}
	if len(got.Projects) != 1 || got.Projects[0] != "corticalstack" {
		t.Errorf("Projects = %v, want [corticalstack]", got.Projects)
	}
	if got.SourceThread != "thread-abc" {
		t.Errorf("SourceThread = %q, want %q", got.SourceThread, "thread-abc")
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	now := time.Now()
	titles := []string{"Proto Alpha", "Proto Beta", "Proto Gamma"}
	for i, title := range titles {
		proto := &Prototype{
			Title:   title,
			Format:  "component-spec",
			Created: now.Add(time.Duration(i) * time.Hour),
			Spec:    "# " + title,
		}
		if err := s.Write(proto); err != nil {
			t.Fatalf("Write(%s): %v", title, err)
		}
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 prototypes, got %d", len(list))
	}

	// Newest first
	if list[0].Title != "Proto Gamma" {
		t.Errorf("list[0].Title = %q, want %q (newest first)", list[0].Title, "Proto Gamma")
	}
	if list[2].Title != "Proto Alpha" {
		t.Errorf("list[2].Title = %q, want %q (oldest last)", list[2].Title, "Proto Alpha")
	}
}

func TestListEmpty(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 prototypes, got %d", len(list))
	}
}
