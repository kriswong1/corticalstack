package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcile_NoChanges(t *testing.T) {
	s := newTempStore(t)

	a, err := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Already synced",
		Status:      StatusPending,
		SourceNote:  "notes/synced.md",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.Sync(a); err != nil {
		t.Fatalf("sync: %v", err)
	}

	res, err := s.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Updated != 0 {
		t.Errorf("Updated = %d, want 0", res.Updated)
	}
	if len(res.Missing) != 0 {
		t.Errorf("Missing = %v, want empty", res.Missing)
	}
	if len(res.Unknown) != 0 {
		t.Errorf("Unknown = %v, want empty", res.Unknown)
	}
}

func TestReconcile_CheckboxToggled(t *testing.T) {
	s := newTempStore(t)

	a, err := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Finish report",
		Status:      StatusPending,
		SourceNote:  "notes/report.md",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.Sync(a); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Simulate user ticking the checkbox in Obsidian: change "- [ ]" to "- [x]"
	// and update the status tag to done.
	notePath := filepath.Join(s.VaultPath(), a.SourceNote)
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("reading source note: %v", err)
	}
	modified := strings.Replace(string(data), "- [ ]", "- [x]", 1)
	modified = strings.Replace(modified, "#status/pending", "#status/done", 1)
	if err := os.WriteFile(notePath, []byte(modified), 0o600); err != nil {
		t.Fatalf("writing modified note: %v", err)
	}

	res, err := s.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("Updated = %d, want 1", res.Updated)
	}

	got := s.Get(a.ID)
	if got == nil {
		t.Fatal("action not found after reconcile")
	}
	if got.Status != StatusDone {
		t.Errorf("Status = %q, want %q", got.Status, StatusDone)
	}
}

func TestReconcile_UnknownID(t *testing.T) {
	s := newTempStore(t)

	// Write a line with an ID that does not exist in the index.
	fakeID := "00000000-0000-0000-0000-000000000000"
	fakeLine := "- [ ] [Nobody] Mystery task #status/pending <!-- id:" + fakeID + " -->"

	centralPath := filepath.Join(s.VaultPath(), s.CentralFilePath())
	data, err := os.ReadFile(centralPath)
	if err != nil {
		t.Fatalf("reading central file: %v", err)
	}
	updated := string(data) + "\n" + fakeLine + "\n"
	if err := os.WriteFile(centralPath, []byte(updated), 0o600); err != nil {
		t.Fatalf("writing central file: %v", err)
	}

	res, err := s.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	found := false
	for _, uid := range res.Unknown {
		if uid == fakeID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Unknown = %v, want it to contain %q", res.Unknown, fakeID)
	}
}

func TestReconcile_MissingID(t *testing.T) {
	s := newTempStore(t)

	// Add an action to the index but do NOT sync it to any file.
	a, err := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Ghost action",
		Status:      StatusPending,
		SourceNote:  "notes/ghost.md",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Deliberately skip s.Sync(a) so the ID is in the index but not on disk.

	res, err := s.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	found := false
	for _, mid := range res.Missing {
		if mid == a.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Missing = %v, want it to contain %q", res.Missing, a.ID)
	}
}
