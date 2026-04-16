package documents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kriswong/corticalstack/internal/stage"
	"github.com/kriswong/corticalstack/internal/vault"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	v := vault.New(dir)
	s := New(v)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}
	return s, dir
}

func writeNote(t *testing.T, root, relPath, content string) {
	t.Helper()
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestEnsureFolderCreatesDir(t *testing.T) {
	_, dir := newTestStore(t)
	if _, err := os.Stat(filepath.Join(dir, "documents")); err != nil {
		t.Errorf("documents dir not created: %v", err)
	}
}

func TestListEmptyVault(t *testing.T) {
	s, _ := newTestStore(t)
	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestListMissingFolder(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	s := New(v)
	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestListReadsFrontmatter(t *testing.T) {
	s, dir := newTestStore(t)
	writeNote(t, dir, "documents/2026-04-15_finished.md", `---
id: doc-1
title: Finished Reference
stage: note
created: 2026-04-15T10:00:00Z
projects:
  - alpha
tags:
  - reference
source: https://example.com/article
---
# Body of the document
`)
	writeNote(t, dir, "documents/2026-04-14_input.md", `---
id: doc-2
title: Raw Input
stage: input
created: 2026-04-14T09:00:00Z
---
draft body
`)

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}

	// Newest first.
	if got[0].ID != "doc-1" {
		t.Errorf("got[0].ID = %q", got[0].ID)
	}
	if got[0].Stage != stage.StageDocNote {
		t.Errorf("got[0].Stage = %q, want note", got[0].Stage)
	}
	if got[0].Source != "https://example.com/article" {
		t.Errorf("got[0].Source = %q", got[0].Source)
	}
	if len(got[0].Projects) != 1 || got[0].Projects[0] != "alpha" {
		t.Errorf("got[0].Projects = %v", got[0].Projects)
	}
	if got[1].Stage != stage.StageInput {
		t.Errorf("got[1].Stage = %q, want input", got[1].Stage)
	}
}

func TestListMissingStageFallsBackToInput(t *testing.T) {
	s, dir := newTestStore(t)
	writeNote(t, dir, "documents/raw.md", `---
title: Hand Dropped
---
body
`)

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Stage != stage.StageInput {
		t.Errorf("stage = %q, want input (fallback)", got[0].Stage)
	}
}

func TestListSkipsNonMarkdown(t *testing.T) {
	s, dir := newTestStore(t)
	writeNote(t, dir, "documents/note.md", "---\ntitle: Real\n---\nbody")
	writeNote(t, dir, "documents/image.png", "garbage")

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d, want 1 (only .md)", len(got))
	}
}

func TestSetStageRoundTrip(t *testing.T) {
	s, dir := newTestStore(t)
	writeNote(t, dir, "documents/example.md", `---
id: doc-1
title: Example
stage: input
created: 2026-04-15T10:00:00Z
---
body content
`)

	if err := s.SetStage("doc-1", stage.StageDocNote); err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	got, err := s.Get("doc-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Stage != stage.StageDocNote {
		t.Errorf("stage = %q, want note", got.Stage)
	}

	// Body should be preserved.
	full := filepath.Join(dir, "documents/example.md")
	raw, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "body content") {
		t.Error("body lost during SetStage write")
	}
	if !strings.Contains(string(raw), "note") {
		t.Error("new stage not persisted")
	}
	if !strings.Contains(string(raw), "updated:") {
		t.Error("updated timestamp not added")
	}
}

func TestSetStageRejectsInvalid(t *testing.T) {
	s, dir := newTestStore(t)
	writeNote(t, dir, "documents/example.md", `---
id: doc-1
title: Example
---
body
`)

	if err := s.SetStage("doc-1", "bogus"); err == nil {
		t.Error("SetStage with bogus stage should error")
	}
	if err := s.SetStage("doc-1", stage.StageFrame); err == nil {
		t.Error("SetStage with cross-entity stage should error")
	}
}

func TestSetStageUnknownDoc(t *testing.T) {
	s, _ := newTestStore(t)
	if err := s.SetStage("missing", stage.StageInput); err == nil {
		t.Error("SetStage on unknown id should error")
	}
}

func TestListNestedFolders(t *testing.T) {
	// Documents may live in nested subfolders (e.g. documents/articles/2026-04-15.md).
	// The store should still find them.
	s, dir := newTestStore(t)
	writeNote(t, dir, "documents/articles/2026-04-15_nested.md", `---
id: nested-1
title: Nested Article
stage: in_progress
created: 2026-04-15T10:00:00Z
---
body
`)

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ID != "nested-1" {
		t.Errorf("ID = %q", got[0].ID)
	}
}
