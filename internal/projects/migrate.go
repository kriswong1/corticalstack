package projects

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/vault"
)

// MigrateResult summarises a Migrate run.
type MigrateResult struct {
	ManifestsUpdated int      // project manifests that gained a uuid: field
	NotesUpdated     int      // notes whose `projects:` array was rewritten
	NotesScanned     int
	UnknownSlugs     []string // slug references encountered with no matching project
	UnknownUUIDs     []string // uuid references encountered with no matching project (dangling)
}

// Migrate is the one-shot vault walker that brings frontmatter `projects:`
// arrays to canonical UUID form. Idempotent — re-running on an already
// migrated vault is a no-op (NotesUpdated == 0).
//
// Pass 1: walks vault/projects/*/project.md, ensures every manifest has a
// `uuid:` field. The store's Refresh has already minted in-memory UUIDs
// for legacy manifests; this pass persists them.
//
// Pass 2: walks every other note in the vault, rewriting `projects:` arrays
// from slug-form to UUID-form. Unknown slugs auto-create a backing project
// (matches pre-Phase-1 SyncFromVault semantics so no associations are lost
// in the migration). Unknown UUIDs are dropped.
//
// Caller must have already called Refresh() on the store before invoking
// Migrate. Migrate calls Refresh() again at the end so the in-memory cache
// reflects any auto-created projects.
func Migrate(s *Store) (MigrateResult, error) {
	res := MigrateResult{}

	// Pass 1: persist UUIDs into manifests that lack them.
	for _, p := range s.List() {
		rel := filepath.ToSlash(filepath.Join(projectsFolder, p.Slug, manifestName))
		note, err := s.vault.ReadNote(rel)
		if err != nil {
			slog.Warn("migrate: read manifest", "slug", p.Slug, "error", err)
			continue
		}
		if u, ok := note.Frontmatter["uuid"].(string); ok && u != "" {
			continue // already has uuid
		}
		// Persist the in-memory UUID via the store's writer so the body
		// composition stays canonical.
		if err := s.writeManifest(p); err != nil {
			slog.Warn("migrate: write manifest", "slug", p.Slug, "error", err)
			continue
		}
		res.ManifestsUpdated++
	}

	// Pass 2: rewrite `projects:` arrays in every other note.
	unknownSlugs := map[string]bool{}
	unknownUUIDs := map[string]bool{}

	err := s.vault.Walk(func(relPath string, note *vault.Note) {
		// Skip the project manifests themselves (Pass 1 owns those).
		if strings.HasPrefix(relPath, projectsFolder+"/") &&
			filepath.Base(relPath) == manifestName {
			return
		}
		res.NotesScanned++

		raw := parseProjectsField(note.Frontmatter)
		if len(raw) == 0 {
			return
		}

		newList, changed, missingSlugs, missingUUIDs := rewriteRefs(s, raw)
		for _, slug := range missingSlugs {
			unknownSlugs[slug] = true
		}
		for _, u := range missingUUIDs {
			unknownUUIDs[u] = true
		}

		if !changed {
			return
		}
		// Write back. Use []string (not []interface{}) so the on-disk
		// shape matches what other writers produce.
		note.Frontmatter["projects"] = newList
		if err := s.vault.WriteNote(relPath, note); err != nil {
			slog.Warn("migrate: write note", "path", relPath, "error", err)
			return
		}
		res.NotesUpdated++
	})
	if err != nil {
		return res, fmt.Errorf("walk: %w", err)
	}

	for slug := range unknownSlugs {
		res.UnknownSlugs = append(res.UnknownSlugs, slug)
	}
	for u := range unknownUUIDs {
		res.UnknownUUIDs = append(res.UnknownUUIDs, u)
	}

	// Refresh so the cache picks up any auto-created projects.
	if err := s.Refresh(); err != nil {
		return res, fmt.Errorf("post-migrate refresh: %w", err)
	}

	return res, nil
}

// rewriteRefs maps each entry in raw to its canonical UUID (creating a
// new project for unknown slugs). Returns the rewritten list, whether any
// change occurred, and lists of references that couldn't be resolved
// (unknown slugs that auto-create successfully are NOT considered
// unresolved — they're materialized).
func rewriteRefs(s *Store, raw []string) (out []string, changed bool, missingSlugs []string, missingUUIDs []string) {
	seen := map[string]bool{}
	out = make([]string, 0, len(raw))
	for _, ref := range raw {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			changed = true
			continue
		}

		// Already a known UUID — keep.
		if p := s.GetByUUID(ref); p != nil {
			if !seen[p.UUID] {
				seen[p.UUID] = true
				out = append(out, p.UUID)
			} else {
				changed = true // dedupe is a change
			}
			continue
		}
		// Known slug — rewrite to UUID.
		if p := s.GetBySlug(ref); p != nil {
			if !seen[p.UUID] {
				seen[p.UUID] = true
				out = append(out, p.UUID)
			}
			changed = true
			continue
		}
		// Unknown UUID — drop (dangling reference).
		if _, err := uuid.Parse(ref); err == nil {
			missingUUIDs = append(missingUUIDs, ref)
			changed = true
			continue
		}
		// Unknown slug — auto-create. This preserves the pre-Phase-1
		// SyncFromVault semantics: any frontmatter slug becomes a project.
		// After the migration the user can rename or delete via the UI.
		project, _, err := s.CreateIfMissing(CreateRequest{Name: ref})
		if err != nil || project == nil {
			missingSlugs = append(missingSlugs, ref)
			changed = true
			continue
		}
		if !seen[project.UUID] {
			seen[project.UUID] = true
			out = append(out, project.UUID)
		}
		changed = true
	}
	return out, changed, missingSlugs, missingUUIDs
}

// LogMigrateResult emits a single slog.Info line summarising the result.
// Callers wanting machine-parseable output should use the MigrateResult
// fields directly.
func LogMigrateResult(res MigrateResult) {
	if res.ManifestsUpdated == 0 && res.NotesUpdated == 0 {
		slog.Info("projects: vault already at canonical UUID form",
			"notes_scanned", res.NotesScanned)
		return
	}
	slog.Info("projects: migrated frontmatter to UUID form",
		"manifests_updated", res.ManifestsUpdated,
		"notes_updated", res.NotesUpdated,
		"notes_scanned", res.NotesScanned,
		"unknown_slugs", len(res.UnknownSlugs),
		"unknown_uuids", len(res.UnknownUUIDs),
	)
	if len(res.UnknownUUIDs) > 0 {
		slog.Warn("projects: dropped dangling UUID refs during migration",
			"count", len(res.UnknownUUIDs),
			"sample", firstN(res.UnknownUUIDs, 5))
	}
}

func firstN(xs []string, n int) []string {
	if len(xs) <= n {
		return xs
	}
	return xs[:n]
}

