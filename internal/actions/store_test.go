package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kriswong/corticalstack/internal/vault"
)

func newTempStore(t *testing.T) *Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "cortical-actions-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	s := New(vault.New(dir))
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.EnsureCentralFile(); err != nil {
		t.Fatalf("ensure central: %v", err)
	}
	return s
}

func TestUpsertAndSync(t *testing.T) {
	s := newTempStore(t)

	a, err := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Write tests",
		Status:      StatusPending,
		SourceNote:  "notes/2026-04-11_hello.md",
		ProjectIDs:  []string{"corticalstack"},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected non-empty id after upsert")
	}
	if err := s.Sync(a); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Central file should contain the line
	central := filepath.Join(s.VaultPath(), s.CentralFilePath())
	data, _ := os.ReadFile(central)
	if !strings.Contains(string(data), "id:"+a.ID) {
		t.Errorf("central file missing id marker; content:\n%s", data)
	}
	// Project file should contain the line
	project := filepath.Join(s.VaultPath(), s.ProjectFilePath("corticalstack"))
	data, _ = os.ReadFile(project)
	if !strings.Contains(string(data), "id:"+a.ID) {
		t.Errorf("project file missing id marker; content:\n%s", data)
	}
}

func TestSetStatusPropagates(t *testing.T) {
	s := newTempStore(t)

	a, _ := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Move status",
		Status:      StatusPending,
		SourceNote:  "notes/move.md",
		ProjectIDs:  []string{"p1"},
	})
	if err := s.Sync(a); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	updated, err := s.SetStatus(a.ID, StatusDoing)
	if err != nil {
		t.Fatalf("set status: %v", err)
	}
	if err := s.Sync(updated); err != nil {
		t.Fatalf("sync after status: %v", err)
	}

	// Central should now contain #status/doing
	central := filepath.Join(s.VaultPath(), s.CentralFilePath())
	data, _ := os.ReadFile(central)
	if !strings.Contains(string(data), "#status/doing") {
		t.Errorf("central file missing new status; content:\n%s", data)
	}
}

func TestCountByStatus(t *testing.T) {
	s := newTempStore(t)
	_, _ = s.Upsert(&Action{Owner: "a", Description: "1", Status: StatusPending}) // migrated to inbox
	_, _ = s.Upsert(&Action{Owner: "b", Description: "2", Status: StatusDone})
	_, _ = s.Upsert(&Action{Owner: "c", Description: "3", Status: StatusDone})

	counts := s.CountByStatus()
	if counts[StatusInbox] != 1 {
		t.Errorf("inbox: got %d want 1", counts[StatusInbox])
	}
	if counts[StatusDone] != 2 {
		t.Errorf("done: got %d want 2", counts[StatusDone])
	}
}
