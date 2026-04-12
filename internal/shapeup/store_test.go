package shapeup

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
