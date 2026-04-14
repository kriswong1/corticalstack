package shapeup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kriswong/corticalstack/internal/vault"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return New(vault.New(dir))
}

func TestEnsureFolders(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	for _, stage := range AllStages() {
		dir := filepath.Join(s.vault.Path(), productDir, string(stage))
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("stage dir %q not created: %v", stage, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("stage path %q is not a directory", stage)
		}
	}
}

func TestCreateRawIdea(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	req := CreateIdeaRequest{
		Title:      "Add dark mode",
		Content:    "Users want a dark theme for the dashboard.",
		ProjectIDs: []string{"corticalstack"},
	}
	art, err := s.CreateRawIdea(req)
	if err != nil {
		t.Fatalf("CreateRawIdea: %v", err)
	}
	if art.ID == "" {
		t.Error("expected non-empty ID")
	}
	if art.Stage != StageRaw {
		t.Errorf("stage = %q, want %q", art.Stage, StageRaw)
	}
	if art.Thread == "" {
		t.Error("expected non-empty thread ID")
	}
	if art.Title != req.Title {
		t.Errorf("title = %q, want %q", art.Title, req.Title)
	}

	// Should be readable from the vault
	if !s.vault.Exists(art.Path) {
		t.Fatalf("artifact file not found at %q", art.Path)
	}
	readBack, err := s.readArtifact(art.Path)
	if err != nil {
		t.Fatalf("readArtifact: %v", err)
	}
	if readBack.ID != art.ID {
		t.Errorf("read-back ID = %q, want %q", readBack.ID, art.ID)
	}
	if readBack.Thread != art.Thread {
		t.Errorf("read-back thread = %q, want %q", readBack.Thread, art.Thread)
	}
}

func TestCreateRawIdeaEmptyTitle(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	_, err := s.CreateRawIdea(CreateIdeaRequest{
		Title:   "   ",
		Content: "some content",
	})
	if err == nil {
		t.Fatal("expected error for empty title, got nil")
	}
}

func TestWriteArtifact(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	threadID := NewThreadID()
	a := &Artifact{
		Stage:    StageFrame,
		Thread:   threadID,
		Title:    "Framed idea",
		ParentID: "parent-abc",
		Projects: []string{"proj1"},
		Body:     "# Framed idea\n\nSome framing content.",
	}
	if err := s.WriteArtifact(a); err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}
	if a.ID == "" {
		t.Error("expected WriteArtifact to assign an ID")
	}
	if a.Path == "" {
		t.Error("expected WriteArtifact to set Path")
	}

	// Read back via GetThread
	thread, err := s.GetThread(threadID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if len(thread.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact in thread, got %d", len(thread.Artifacts))
	}
	if thread.Artifacts[0].ID != a.ID {
		t.Errorf("artifact ID mismatch: got %q, want %q", thread.Artifacts[0].ID, a.ID)
	}
}

func TestGetThreadOrderByStage(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	threadID := NewThreadID()
	now := time.Now()

	// Write artifacts in reverse stage order
	stages := []Stage{StageBreadboard, StageRaw, StageFrame}
	for i, stage := range stages {
		a := &Artifact{
			Stage:   stage,
			Thread:  threadID,
			Title:   "Ordered test",
			Created: now.Add(time.Duration(i) * time.Second),
			Body:    "body",
		}
		if err := s.WriteArtifact(a); err != nil {
			t.Fatalf("WriteArtifact(%s): %v", stage, err)
		}
	}

	thread, err := s.GetThread(threadID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if len(thread.Artifacts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(thread.Artifacts))
	}

	// Should be ordered: raw, frame, breadboard
	wantOrder := []Stage{StageRaw, StageFrame, StageBreadboard}
	for i, want := range wantOrder {
		if thread.Artifacts[i].Stage != want {
			t.Errorf("artifact[%d].Stage = %q, want %q", i, thread.Artifacts[i].Stage, want)
		}
	}

	// CurrentStage should be the latest stage
	if thread.CurrentStage != StageBreadboard {
		t.Errorf("CurrentStage = %q, want %q", thread.CurrentStage, StageBreadboard)
	}
}

func TestGetThreadNonexistent(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	_, err := s.GetThread("does-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent thread, got nil")
	}
}

func TestListThreads(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	// Create two threads, each with one artifact
	now := time.Now()
	thread1 := NewThreadID()
	thread2 := NewThreadID()

	a1 := &Artifact{
		Stage:   StageRaw,
		Thread:  thread1,
		Title:   "First idea",
		Created: now.Add(-1 * time.Hour),
		Body:    "body 1",
	}
	a2 := &Artifact{
		Stage:   StageRaw,
		Thread:  thread2,
		Title:   "Second idea",
		Created: now,
		Body:    "body 2",
	}
	if err := s.WriteArtifact(a1); err != nil {
		t.Fatalf("WriteArtifact(a1): %v", err)
	}
	if err := s.WriteArtifact(a2); err != nil {
		t.Fatalf("WriteArtifact(a2): %v", err)
	}

	threads, err := s.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}

	// Newest first: thread2 should come first
	if threads[0].ID != thread2 {
		t.Errorf("threads[0].ID = %q, want %q (newest first)", threads[0].ID, thread2)
	}
	if threads[1].ID != thread1 {
		t.Errorf("threads[1].ID = %q, want %q", threads[1].ID, thread1)
	}
}

func TestListThreadsEmpty(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	threads, err := s.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 0 {
		t.Errorf("expected 0 threads, got %d", len(threads))
	}
}

// TestCreateRawIdeaUniquePathOnSameDayTitle — HI-04 regression.
// Two raw ideas with the same title on the same day must not clobber each
// other. Before HI-04, both calls resolved to the same filename
// (<date>_<slug>.md) and the second silently overwrote the first. After
// the fix, the per-artifact UUID suffix guarantees unique paths.
func TestCreateRawIdeaUniquePathOnSameDayTitle(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	req := CreateIdeaRequest{
		Title:      "Add dark mode",
		Content:    "First body",
		ProjectIDs: []string{"corticalstack"},
	}
	first, err := s.CreateRawIdea(req)
	if err != nil {
		t.Fatalf("first CreateRawIdea: %v", err)
	}

	req2 := CreateIdeaRequest{
		Title:      "Add dark mode",
		Content:    "Second body — should not clobber the first",
		ProjectIDs: []string{"corticalstack"},
	}
	second, err := s.CreateRawIdea(req2)
	if err != nil {
		t.Fatalf("second CreateRawIdea: %v", err)
	}

	if first.Path == second.Path {
		t.Fatalf("expected distinct paths; both resolved to %q", first.Path)
	}
	if !s.vault.Exists(first.Path) {
		t.Errorf("first artifact missing on disk at %q", first.Path)
	}
	if !s.vault.Exists(second.Path) {
		t.Errorf("second artifact missing on disk at %q", second.Path)
	}

	// Read-back both and verify bodies are intact (no clobber).
	firstRead, err := s.readArtifact(first.Path)
	if err != nil {
		t.Fatalf("read first: %v", err)
	}
	secondRead, err := s.readArtifact(second.Path)
	if err != nil {
		t.Fatalf("read second: %v", err)
	}
	if firstRead.ID != first.ID {
		t.Errorf("first ID mismatch: got %q, want %q", firstRead.ID, first.ID)
	}
	if secondRead.ID != second.ID {
		t.Errorf("second ID mismatch: got %q, want %q", secondRead.ID, second.ID)
	}

	// Both artifacts should surface through ListThreads as independent threads.
	threads, err := s.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 2 {
		t.Errorf("expected 2 threads after two CreateRawIdea calls, got %d", len(threads))
	}
}

// TestWriteArtifactWithoutPresetID — HI-04 regression.
// WriteArtifact must assign an ID before computing the path so the ID
// suffix in the filename is always populated. Passing in an Artifact with
// ID == "" must still produce a valid, unique path that includes a real
// ID suffix (not the "noid" fallback placeholder).
func TestWriteArtifactWithoutPresetID(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	a := &Artifact{
		Stage:  StageFrame,
		Thread: NewThreadID(),
		Title:  "Auto-id frame",
		Body:   "# Auto-id frame\n\nbody",
	}
	if err := s.WriteArtifact(a); err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected WriteArtifact to assign an ID")
	}
	if a.Path == "" {
		t.Fatal("expected WriteArtifact to set Path")
	}

	// The filename must contain the 8-char ID prefix, proving the ordering
	// (ensure-ID then compute-path) is correct.
	wantSuffix := "_" + a.ID[:8] + ".md"
	if !strings.HasSuffix(a.Path, wantSuffix) {
		t.Errorf("path %q missing ID suffix %q", a.Path, wantSuffix)
	}
	// And explicitly NOT the "noid" fallback.
	if strings.Contains(a.Path, "_noid.md") {
		t.Errorf("path %q fell back to noid placeholder", a.Path)
	}
	if !s.vault.Exists(a.Path) {
		t.Errorf("artifact missing on disk at %q", a.Path)
	}
}

// TestListThreadsReadsBothOldAndNewFormat — HI-04 backward compatibility.
// Files written under the legacy `<date>_<slug>.md` naming (pre-HI-04)
// must continue to surface through ListThreads alongside files written in
// the new `<date>_<slug>_<idShort>.md` format. walkArtifacts reads every
// `.md` file under the stage dir without parsing the filename, so both
// coexist — this test pins that contract.
func TestListThreadsReadsBothOldAndNewFormat(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}

	// 1. New-format artifact via the public API.
	newArt, err := s.CreateRawIdea(CreateIdeaRequest{
		Title:   "New format idea",
		Content: "new body",
	})
	if err != nil {
		t.Fatalf("CreateRawIdea: %v", err)
	}

	// 2. Legacy-format artifact written directly at a hand-built path
	//    that matches the old naming convention (`<date>_<slug>.md`, no
	//    ID suffix). We use vault.WriteNote directly to bypass the
	//    collision-proof path builder.
	legacyThreadID := NewThreadID()
	legacyArtifactID := "legacy-11111111-2222-3333-4444-555555555555"
	legacyRelPath := filepath.ToSlash(filepath.Join(productDir, string(StageRaw), "2020-01-01_legacy-format-idea.md"))
	legacyNote := renderArtifact(&Artifact{
		ID:      legacyArtifactID,
		Stage:   StageRaw,
		Thread:  legacyThreadID,
		Title:   "Legacy format idea",
		Status:  "draft",
		Created: time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC),
		Body:    "legacy body",
	})
	if err := s.vault.WriteNote(legacyRelPath, legacyNote); err != nil {
		t.Fatalf("writing legacy file: %v", err)
	}

	// 3. Both should surface through ListThreads.
	threads, err := s.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads (one new, one legacy), got %d", len(threads))
	}

	// Confirm the legacy thread ID and the new thread ID both appear.
	seen := map[string]bool{}
	for _, th := range threads {
		seen[th.ID] = true
	}
	if !seen[newArt.Thread] {
		t.Errorf("new-format thread %q missing from ListThreads", newArt.Thread)
	}
	if !seen[legacyThreadID] {
		t.Errorf("legacy-format thread %q missing from ListThreads", legacyThreadID)
	}

	// And GetThread should be able to fetch the legacy one by ID.
	legacyThread, err := s.GetThread(legacyThreadID)
	if err != nil {
		t.Fatalf("GetThread(legacy): %v", err)
	}
	if len(legacyThread.Artifacts) != 1 {
		t.Fatalf("expected 1 legacy artifact, got %d", len(legacyThread.Artifacts))
	}
	if legacyThread.Artifacts[0].ID != legacyArtifactID {
		t.Errorf("legacy artifact ID = %q, want %q", legacyThread.Artifacts[0].ID, legacyArtifactID)
	}
}
