package projects

import (
	"strings"
	"testing"
)

// TestCanvas_RoundTripPreservesUserContent covers the split-mode body
// composition: the deterministic header + footer regenerate on every
// write, but everything between the `## Canvas` heading and the next
// `## ` section round-trips untouched.
func TestCanvas_RoundTripPreservesUserContent(t *testing.T) {
	s := newTempStore(t)
	p, err := s.Create(CreateRequest{Name: "Test"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	const userText = "This is the bet.\n\nAppetite: 6 weeks.\n\nBoundaries:\n- no auth refactor\n- no schema changes"

	if err := s.SetCanvas(p.UUID, userText); err != nil {
		t.Fatalf("set canvas: %v", err)
	}

	got, err := s.Canvas(p.UUID)
	if err != nil {
		t.Fatalf("get canvas: %v", err)
	}
	if got != userText {
		t.Errorf("canvas mismatch\ngot:  %q\nwant: %q", got, userText)
	}

	// Trigger a non-canvas re-write (Update changes name) and confirm the
	// canvas survives. The writer composes header + canvas + footer fresh
	// each time; the canvas must be re-read off the existing manifest.
	newName := "Renamed Test"
	if _, err := s.Update(p.UUID, UpdateRequest{Name: &newName}); err != nil {
		t.Fatalf("update: %v", err)
	}
	gotAfterRename, err := s.Canvas(p.UUID)
	if err != nil {
		t.Fatalf("get canvas after rename: %v", err)
	}
	if gotAfterRename != userText {
		t.Errorf("canvas lost across rename\ngot:  %q\nwant: %q", gotAfterRename, userText)
	}
}

// TestCanvas_EmptyManifestReturnsEmpty confirms a freshly-created project
// (which has no canvas content yet) returns "" rather than the section
// heading or boilerplate.
func TestCanvas_EmptyManifestReturnsEmpty(t *testing.T) {
	s := newTempStore(t)
	p, _ := s.Create(CreateRequest{Name: "Fresh"})

	got, err := s.Canvas(p.UUID)
	if err != nil {
		t.Fatalf("canvas: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty canvas, got %q", got)
	}
}

// TestCanvas_ReadEmitsHeading verifies the writer produces a `## Canvas`
// heading even when the canvas content is empty, so users can find the
// section to type into when editing the file in Obsidian directly.
func TestCanvas_ReadEmitsHeading(t *testing.T) {
	s := newTempStore(t)
	p, _ := s.Create(CreateRequest{Name: "Heading Test"})

	rel := "projects/" + p.Slug + "/project.md"
	note, err := s.vault.ReadNote(rel)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(note.Body, canvasHeader) {
		t.Errorf("manifest body missing canvas heading\nbody:\n%s", note.Body)
	}
	if !strings.Contains(note.Body, "## Notes") {
		t.Errorf("manifest body missing Notes footer (deterministic)\nbody:\n%s", note.Body)
	}
}
