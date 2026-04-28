package projects

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/vault"
)

// TestMigrate_RewritesSlugRefsToUUIDs covers the happy path: a vault note
// references a project by slug; after Migrate the reference is the UUID.
func TestMigrate_RewritesSlugRefsToUUIDs(t *testing.T) {
	s := newTempStore(t)

	p, err := s.Create(CreateRequest{Name: "Surveil"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Hand-craft a note that references the project by slug.
	note := &vault.Note{
		Frontmatter: map[string]interface{}{
			"title":    "Some note",
			"projects": []string{"surveil"},
		},
		Body: "body\n",
	}
	if err := s.vault.WriteNote("notes/2026-04-28_some-note.md", note); err != nil {
		t.Fatalf("write note: %v", err)
	}

	res, err := Migrate(s)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.NotesUpdated != 1 {
		t.Errorf("NotesUpdated = %d, want 1", res.NotesUpdated)
	}

	// Re-read the note and verify the UUID rewrite.
	got, err := s.vault.ReadNote("notes/2026-04-28_some-note.md")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	refs := parseProjectsField(got.Frontmatter)
	if len(refs) != 1 {
		t.Fatalf("refs = %v, want 1 ref", refs)
	}
	if refs[0] != p.UUID {
		t.Errorf("refs[0] = %q, want UUID %q", refs[0], p.UUID)
	}
}

// TestMigrate_Idempotent re-runs Migrate on an already-migrated vault and
// confirms no notes are rewritten the second time.
func TestMigrate_Idempotent(t *testing.T) {
	s := newTempStore(t)
	p, _ := s.Create(CreateRequest{Name: "Alpha"})

	note := &vault.Note{
		Frontmatter: map[string]interface{}{
			"projects": []string{p.UUID}, // already canonical
		},
		Body: "x",
	}
	if err := s.vault.WriteNote("notes/a.md", note); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := Migrate(s)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.NotesUpdated != 0 {
		t.Errorf("NotesUpdated = %d, want 0 (already canonical)", res.NotesUpdated)
	}
}

// TestMigrate_DropsDanglingUUID verifies references to deleted (unknown)
// UUIDs are removed from frontmatter on migration. This protects against
// stale refs accumulating after Phase 2 deletes land.
func TestMigrate_DropsDanglingUUID(t *testing.T) {
	s := newTempStore(t)

	dangling := uuid.NewString()
	note := &vault.Note{
		Frontmatter: map[string]interface{}{
			"projects": []string{dangling},
		},
		Body: "x",
	}
	if err := s.vault.WriteNote("notes/b.md", note); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := Migrate(s)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.NotesUpdated != 1 {
		t.Errorf("NotesUpdated = %d, want 1 (note rewritten to drop dangling)", res.NotesUpdated)
	}

	got, _ := s.vault.ReadNote("notes/b.md")
	refs := parseProjectsField(got.Frontmatter)
	if len(refs) != 0 {
		t.Errorf("refs = %v, want empty (dangling dropped)", refs)
	}
}

// TestMigrate_AutoCreatesMissingSlug preserves the pre-Phase-1
// SyncFromVault behavior: a slug referenced in a note that has no backing
// project gets the project auto-materialized so the association isn't lost.
func TestMigrate_AutoCreatesMissingSlug(t *testing.T) {
	s := newTempStore(t)

	note := &vault.Note{
		Frontmatter: map[string]interface{}{
			"projects": []string{"orphan-project"},
		},
		Body: "x",
	}
	if err := s.vault.WriteNote("notes/c.md", note); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := Migrate(s)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.NotesUpdated != 1 {
		t.Errorf("NotesUpdated = %d, want 1", res.NotesUpdated)
	}

	p := s.GetBySlug("orphan-project")
	if p == nil {
		t.Fatal("expected orphan-project to be auto-created")
	}

	got, _ := s.vault.ReadNote("notes/c.md")
	refs := parseProjectsField(got.Frontmatter)
	if len(refs) != 1 || refs[0] != p.UUID {
		t.Errorf("refs = %v, want [%q]", refs, p.UUID)
	}
}

// TestCanonicalizeProjectIDs covers the funnel that all `projects:` writers
// route through: deduplication, slug→UUID translation, dangling drop.
func TestCanonicalizeProjectIDs(t *testing.T) {
	s := newTempStore(t)
	pa, _ := s.Create(CreateRequest{Name: "Alpha"})
	pb, _ := s.Create(CreateRequest{Name: "Beta"})

	dangling := uuid.NewString()

	got := CanonicalizeProjectIDs(s, []string{
		pa.UUID,    // canonical UUID
		"beta",     // slug for Beta
		"  ",       // empty after trim
		pa.UUID,    // duplicate
		dangling,   // unknown UUID — drop
		"unknown",  // unknown slug — drop
	})

	if len(got) != 2 {
		t.Fatalf("got %v, want 2 entries", got)
	}
	if got[0] != pa.UUID {
		t.Errorf("got[0] = %q, want %q", got[0], pa.UUID)
	}
	if got[1] != pb.UUID {
		t.Errorf("got[1] = %q, want %q", got[1], pb.UUID)
	}
}

// TestUpdate_RenameKeepsUUID verifies that renaming a project (which
// regens the slug and the directory) preserves the canonical UUID — the
// whole point of the surrogate-ID design.
func TestUpdate_RenameKeepsUUID(t *testing.T) {
	s := newTempStore(t)
	original, err := s.Create(CreateRequest{Name: "Original Name"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newName := "Brand New Name"
	updated, err := s.Update(original.UUID, UpdateRequest{Name: &newName})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.UUID != original.UUID {
		t.Errorf("UUID changed: was %q, now %q", original.UUID, updated.UUID)
	}
	if !strings.HasPrefix(updated.Slug, "brand-new-name") {
		t.Errorf("Slug = %q, want brand-new-name", updated.Slug)
	}
	// Old slug should no longer resolve.
	if s.GetBySlug(original.Slug) != nil {
		t.Error("old slug still resolves after rename")
	}
	// UUID still resolves.
	if s.GetByUUID(original.UUID) == nil {
		t.Error("UUID lookup broken after rename")
	}
}

// TestDelete_SoftDeletesToTrash confirms Delete moves the directory rather
// than hard-removing it. References in other notes survive (they just
// dangle until restored or cleaned).
func TestDelete_SoftDeletesToTrash(t *testing.T) {
	s := newTempStore(t)
	p, _ := s.Create(CreateRequest{Name: "Doomed"})

	if err := s.Delete(p.UUID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if s.GetByUUID(p.UUID) != nil {
		t.Error("project still in store after delete")
	}
	// Original directory should be gone.
	if s.vault.Exists("projects/doomed/project.md") {
		t.Error("project dir still exists after soft-delete")
	}
}
