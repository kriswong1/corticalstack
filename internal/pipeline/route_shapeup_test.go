package pipeline

import (
	"strings"
	"testing"

	"github.com/kriswong/corticalstack/internal/shapeup"
	"github.com/kriswong/corticalstack/internal/vault"
)

func newShapeUpTestStore(t *testing.T) *shapeup.Store {
	t.Helper()
	dir := t.TempDir()
	s := shapeup.New(vault.New(dir))
	if err := s.EnsureFolders(); err != nil {
		t.Fatalf("EnsureFolders: %v", err)
	}
	return s
}

func TestShapeUpIdeasDestinationNilStore(t *testing.T) {
	d := NewShapeUpIdeasDestination(nil)
	doc := &TextDocument{Title: "Demo"}
	ex := &Extracted{Ideas: []string{"An idea"}}
	out, err := d.Accept(doc, ex)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if out != "" {
		t.Errorf("nil store should return empty path, got %q", out)
	}
}

func TestShapeUpIdeasDestinationNoIdeas(t *testing.T) {
	s := newShapeUpTestStore(t)
	d := NewShapeUpIdeasDestination(s)

	out, err := d.Accept(&TextDocument{Title: "Demo"}, &Extracted{})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if out != "" {
		t.Errorf("no ideas should return empty path, got %q", out)
	}

	threads, err := s.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 0 {
		t.Errorf("expected 0 threads, got %d", len(threads))
	}
}

func TestShapeUpIdeasDestinationCreatesRawIdeas(t *testing.T) {
	s := newShapeUpTestStore(t)
	d := NewShapeUpIdeasDestination(s)

	doc := &TextDocument{
		Title:  "Planning meeting",
		Source: "vtt",
		Metadata: map[string]string{
			"note_path": "transcripts/2026-04-13_planning-meeting.md",
			"projects":  "corticalstack, secondbrain",
		},
	}
	ex := &Extracted{
		Ideas: []string{
			"Add a keyboard shortcut to jump between raw ideas",
			"  ",
			"Export action items as an ICS calendar feed.",
		},
	}

	out, err := d.Accept(doc, ex)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if out == "" {
		t.Error("expected last artifact path, got empty")
	}

	threads, err := s.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	// Two ideas created (the blank one is skipped).
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}

	// Each thread should be a single raw artifact with the right projects
	// and a body that links back to the source note.
	for _, th := range threads {
		if len(th.Artifacts) != 1 {
			t.Errorf("thread %q: expected 1 artifact, got %d", th.ID, len(th.Artifacts))
			continue
		}
		art := th.Artifacts[0]
		if art.Stage != shapeup.StageRaw {
			t.Errorf("artifact stage = %q, want raw", art.Stage)
		}
		if len(art.Projects) != 2 {
			t.Errorf("projects = %v, want [corticalstack secondbrain]", art.Projects)
		}
		if !strings.Contains(art.Body, "transcripts/2026-04-13_planning-meeting.md") {
			t.Errorf("body missing source backlink: %s", art.Body)
		}
		if !strings.Contains(art.Body, "_(vtt)_") {
			t.Errorf("body missing source tag: %s", art.Body)
		}
	}
}

func TestShapeUpIdeasDestinationName(t *testing.T) {
	d := NewShapeUpIdeasDestination(nil)
	if d.Name() != "shapeup-ideas" {
		t.Errorf("Name() = %q, want shapeup-ideas", d.Name())
	}
}

func TestIdeaTitleTruncation(t *testing.T) {
	long := strings.Repeat("a", 120) + "."
	got := ideaTitle(long)
	if len(got) != 80 {
		t.Errorf("title length = %d, want 80", len(got))
	}
	// Trailing punctuation trimmed.
	if strings.HasSuffix(ideaTitle("Hello world!"), "!") {
		t.Error("should trim trailing punctuation")
	}
}
