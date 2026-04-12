package usecases

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

	dir := filepath.Join(s.vault.Path(), useCasesDir)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("usecases dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("usecases path is not a directory")
	}
}

func TestWriteGetRoundtrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	uc := &UseCase{
		Title:    "User logs in",
		Actors:   []string{"End User"},
		MainFlow: []string{"Navigate to login", "Enter credentials", "Click submit"},
		Projects: []string{"corticalstack"},
		Tags:     []string{"auth"},
	}

	if err := s.Write(uc); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if uc.ID == "" {
		t.Error("expected Write to assign an ID")
	}
	if uc.Path == "" {
		t.Error("expected Write to set Path")
	}
	if uc.Created.IsZero() {
		t.Error("expected Write to set Created")
	}

	got, err := s.Get(uc.Path)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != uc.ID {
		t.Errorf("ID = %q, want %q", got.ID, uc.ID)
	}
	if got.Title != uc.Title {
		t.Errorf("Title = %q, want %q", got.Title, uc.Title)
	}
	if len(got.Actors) != 1 || got.Actors[0] != "End User" {
		t.Errorf("Actors = %v, want [End User]", got.Actors)
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
	titles := []string{"First UC", "Second UC", "Third UC"}
	for i, title := range titles {
		uc := &UseCase{
			Title:    title,
			Actors:   []string{"User"},
			MainFlow: []string{"step 1"},
			Created:  now.Add(time.Duration(i) * time.Hour),
		}
		if err := s.Write(uc); err != nil {
			t.Fatalf("Write(%s): %v", title, err)
		}
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 use cases, got %d", len(list))
	}

	// Newest first
	if list[0].Title != "Third UC" {
		t.Errorf("list[0].Title = %q, want %q (newest first)", list[0].Title, "Third UC")
	}
	if list[2].Title != "First UC" {
		t.Errorf("list[2].Title = %q, want %q (oldest last)", list[2].Title, "First UC")
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
		t.Errorf("expected 0 use cases, got %d", len(list))
	}
}

func TestGetNonexistent(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	_, err := s.Get("usecases/does-not-exist.md")
	if err == nil {
		t.Fatal("expected error for nonexistent use case, got nil")
	}
}
