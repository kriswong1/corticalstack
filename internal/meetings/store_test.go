package meetings

import (
	"os"
	"path/filepath"
	"testing"

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

func TestEnsureFolderCreatesAllStages(t *testing.T) {
	_, dir := newTestStore(t)
	// Three canonical folders for the new three-stage pipeline.
	for _, name := range []string{"transcripts", "audio", "notes"} {
		if _, err := os.Stat(filepath.Join(dir, "meetings", name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
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

func TestListMissingFolderReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	s := New(v)
	// Don't call EnsureFolder.
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
	writeNote(t, dir, "meetings/transcripts/2026-04-14_kickoff.md", `---
id: meeting-1
title: Project Kickoff
stage: transcript
created: 2026-04-14T10:00:00Z
projects:
  - alpha
---
# Transcript

[00:00:01] Hello everyone.
`)
	// Legacy folder + legacy stage value — both should normalize to
	// StageNote so existing on-disk notes keep classifying correctly.
	writeNote(t, dir, "meetings/summaries/2026-04-14_kickoff-summary.md", `---
id: meeting-1-summary
title: Project Kickoff — Summary
stage: summary
source_id: meeting-1
created: 2026-04-14T11:00:00Z
---
# Summary

- Decision: ship by Q3
`)

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}

	// Newest first — summary was created later than transcript.
	if got[0].Stage != StageNote {
		t.Errorf("got[0].Stage = %q, want note (legacy summary alias)", got[0].Stage)
	}
	if got[0].SourceID != "meeting-1" {
		t.Errorf("source_id = %q", got[0].SourceID)
	}
	if got[1].Stage != StageTranscript {
		t.Errorf("got[1].Stage = %q, want transcript", got[1].Stage)
	}
	if got[1].Title != "Project Kickoff" {
		t.Errorf("title = %q", got[1].Title)
	}
	if len(got[1].Projects) != 1 || got[1].Projects[0] != "alpha" {
		t.Errorf("projects = %v", got[1].Projects)
	}
}

func TestListReadsAudioStage(t *testing.T) {
	s, dir := newTestStore(t)
	writeNote(t, dir, "meetings/audio/2026-04-15_call.md", `---
id: meeting-2
title: Discovery Call
stage: audio
created: 2026-04-15T09:00:00Z
---
# Audio capture
`)
	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Stage != StageAudio {
		t.Errorf("stage = %q, want audio", got[0].Stage)
	}
}

func TestSetStageRoundTrip(t *testing.T) {
	s, dir := newTestStore(t)
	writeNote(t, dir, "meetings/transcripts/example.md", `---
id: meeting-1
title: Example
stage: transcript
created: 2026-04-15T10:00:00Z
---
body
`)

	if err := s.SetStage("meeting-1", StageNote); err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Stage != StageNote {
		t.Errorf("stage = %q, want note", got[0].Stage)
	}
}

func TestSetStageRejectsInvalid(t *testing.T) {
	s, dir := newTestStore(t)
	writeNote(t, dir, "meetings/transcripts/example.md", `---
id: meeting-1
title: Example
---
body
`)
	if err := s.SetStage("meeting-1", "bogus"); err == nil {
		t.Error("SetStage with bogus value should error")
	}
	if err := s.SetStage("missing", StageNote); err == nil {
		t.Error("SetStage on unknown id should error")
	}
}

func TestListFallsBackToFolderForStage(t *testing.T) {
	// A note with no `stage` frontmatter should still classify
	// correctly based on the folder it lives in.
	s, dir := newTestStore(t)
	writeNote(t, dir, "meetings/transcripts/raw.md", `---
title: Raw Drop
---
# Raw transcript text
`)
	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Stage != StageTranscript {
		t.Errorf("stage = %q, want transcript (from folder)", got[0].Stage)
	}
}

func TestListSkipsNonMarkdownFiles(t *testing.T) {
	s, dir := newTestStore(t)
	writeNote(t, dir, "meetings/transcripts/note.md", "---\ntitle: Real\n---\nbody")
	writeNote(t, dir, "meetings/transcripts/audio.wav", "garbage")
	writeNote(t, dir, "meetings/transcripts/notes.txt", "also garbage")

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d, want 1 (only .md)", len(got))
	}
}

func TestIsValidStage(t *testing.T) {
	// All three canonical stages plus the legacy "summary" alias.
	for _, st := range []string{"transcript", "audio", "note", "summary"} {
		if !IsValidStage(st) {
			t.Errorf("IsValidStage(%q) = false", st)
		}
	}
	if IsValidStage("bogus") {
		t.Error("IsValidStage(bogus) = true")
	}
	if IsValidStage("") {
		t.Error("IsValidStage(empty) = true")
	}
}
