package prds

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

	dir := filepath.Join(s.vault.Path(), prdsDir)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("prds dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("prds path is not a directory")
	}
}

func TestWriteGetRoundtrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	prd := &PRD{
		Title:       "Launch dark mode",
		SourcePitch: "product/pitch/2026-04-11_dark-mode.md",
		Projects:    []string{"corticalstack"},
		Body:        "# Launch dark mode\n\nPRD body content here.",
	}

	if err := s.Write(prd); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if prd.ID == "" {
		t.Error("expected Write to assign an ID")
	}
	if prd.Path == "" {
		t.Error("expected Write to set Path")
	}
	if prd.Created.IsZero() {
		t.Error("expected Write to set Created")
	}
	if prd.Version != 1 {
		t.Errorf("Version = %d, want 1", prd.Version)
	}
	if prd.Status != StatusDraft {
		t.Errorf("Status = %q, want %q", prd.Status, StatusDraft)
	}

	got, err := s.Get(prd.Path)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != prd.ID {
		t.Errorf("ID = %q, want %q", got.ID, prd.ID)
	}
	if got.Title != prd.Title {
		t.Errorf("Title = %q, want %q", got.Title, prd.Title)
	}
	if got.SourcePitch != prd.SourcePitch {
		t.Errorf("SourcePitch = %q, want %q", got.SourcePitch, prd.SourcePitch)
	}
	if got.Body == "" {
		t.Error("expected non-empty body on read-back")
	}
	if len(got.Projects) != 1 || got.Projects[0] != "corticalstack" {
		t.Errorf("Projects = %v, want [corticalstack]", got.Projects)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	now := time.Now()
	titles := []string{"PRD Alpha", "PRD Beta", "PRD Gamma"}
	for i, title := range titles {
		prd := &PRD{
			Title:       title,
			SourcePitch: "product/pitch/some-pitch.md",
			Created:     now.Add(time.Duration(i) * time.Hour),
			Body:        "# " + title + "\n\nContent.",
		}
		if err := s.Write(prd); err != nil {
			t.Fatalf("Write(%s): %v", title, err)
		}
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 PRDs, got %d", len(list))
	}

	// Newest first
	if list[0].Title != "PRD Gamma" {
		t.Errorf("list[0].Title = %q, want %q (newest first)", list[0].Title, "PRD Gamma")
	}
	if list[2].Title != "PRD Alpha" {
		t.Errorf("list[2].Title = %q, want %q (oldest last)", list[2].Title, "PRD Alpha")
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
		t.Errorf("expected 0 PRDs, got %d", len(list))
	}
}
