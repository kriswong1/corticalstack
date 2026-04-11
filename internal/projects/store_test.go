package projects

import (
	"os"
	"testing"

	"github.com/kriswong/corticalstack/internal/vault"
)

func newTempStore(t *testing.T) *Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "cortical-projects-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return New(vault.New(dir))
}

func TestCreateAndList(t *testing.T) {
	s := newTempStore(t)

	p, err := s.Create(CreateRequest{Name: "LicenseNinja", Description: "SaaS"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID != "licenseninja" {
		t.Errorf("id: got %q want %q", p.ID, "licenseninja")
	}
	if p.Status != StatusActive {
		t.Errorf("status: got %q want active", p.Status)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("list length: got %d want 1", len(list))
	}
	if list[0].Name != "LicenseNinja" {
		t.Errorf("list[0].Name: got %q", list[0].Name)
	}
}

func TestRefreshDiscoversExisting(t *testing.T) {
	s := newTempStore(t)
	_, _ = s.Create(CreateRequest{Name: "Alpha"})

	// Fresh store over the same vault should find the existing project.
	fresh := New(s.vault)
	if err := fresh.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if fresh.Get("alpha") == nil {
		t.Errorf("expected alpha to be discovered")
	}
}

func TestCreateDuplicateFails(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.Create(CreateRequest{Name: "Beta"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := s.Create(CreateRequest{Name: "Beta"}); err == nil {
		t.Errorf("expected duplicate create to fail")
	}
}
