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

// TestReconcile_FindsMovedSourceNote — LO-07 regression.
// When the user moves a source note in Obsidian (inbox/foo.md →
// projects/x/foo.md) the stored SourceNote becomes stale. Reconcile
// must still find the action ID at its new location and pick up the
// checkbox edit there, even though the stale SourceNote path no longer
// contains the marker.
func TestReconcile_FindsMovedSourceNote(t *testing.T) {
	s := newTempStore(t)

	a, err := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Finish report",
		Status:      StatusPending,
		SourceNote:  "inbox/report.md",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.Sync(a); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Simulate the user moving the note from inbox/report.md to
	// projects/x/report.md in Obsidian. Read the original file contents
	// (which Sync wrote), delete it, and write the same contents (with
	// the checkbox ticked) at the new path. The stored SourceNote stays
	// pointing at the old path — that's exactly the stale state LO-07
	// is about.
	oldPath := filepath.Join(s.VaultPath(), "inbox", "report.md")
	data, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("reading old source note: %v", err)
	}
	newPath := filepath.Join(s.VaultPath(), "projects", "x", "report.md")
	if err := os.MkdirAll(filepath.Dir(newPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	moved := strings.Replace(string(data), "- [ ]", "- [x]", 1)
	moved = strings.Replace(moved, "#status/pending", "#status/done", 1)
	if err := os.WriteFile(newPath, []byte(moved), 0o600); err != nil {
		t.Fatalf("writing new path: %v", err)
	}
	if err := os.Remove(oldPath); err != nil {
		t.Fatalf("removing old path: %v", err)
	}

	res, err := s.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("Updated = %d, want 1 (the vault walk should have found the moved note)", res.Updated)
	}

	got := s.Get(a.ID)
	if got == nil {
		t.Fatal("action not found after reconcile")
	}
	if got.Status != StatusDone {
		t.Errorf("Status = %q, want %q (picked up the checkbox tick from the moved note)", got.Status, StatusDone)
	}
}

// TestReconcile_SkipsDotDirs — LO-07 regression.
// Action markers inside dot-directories (.obsidian plugin state, .git,
// .cortical index) must not be scanned. They may contain files we don't
// own and aren't user notes.
func TestReconcile_SkipsDotDirs(t *testing.T) {
	s := newTempStore(t)

	// Put an ID marker inside a dot-directory — reconcile should NOT
	// see it.
	dotDir := filepath.Join(s.VaultPath(), ".obsidian", "plugins", "something")
	if err := os.MkdirAll(dotDir, 0o700); err != nil {
		t.Fatalf("mkdir dot dir: %v", err)
	}
	fakeID := "11111111-2222-3333-4444-555555555555"
	fakeLine := "- [ ] [Ghost] Should be ignored #status/pending <!-- id:" + fakeID + " -->"
	dotFile := filepath.Join(dotDir, "config.md")
	if err := os.WriteFile(dotFile, []byte(fakeLine), 0o600); err != nil {
		t.Fatalf("writing dot file: %v", err)
	}

	res, err := s.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, uid := range res.Unknown {
		if uid == fakeID {
			t.Errorf("reconcile scanned a dot-directory (found ID %q from %s)", fakeID, dotFile)
		}
	}
}
