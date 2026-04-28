package projects

import (
	"strings"

	"github.com/google/uuid"
)

// CanonicalizeProjectIDs maps any mix of UUID, slug, or empty/duplicate
// strings into a deduplicated slice of UUIDs ready to be written to a
// note's `projects:` frontmatter. Unknown UUIDs (referring to deleted or
// not-yet-loaded projects) are dropped.
//
// Every site that writes a `projects:` array to disk should funnel
// through this function so:
//   - dangling UUIDs from deleted projects don't propagate forward,
//   - slugs introduced by hand-edited frontmatter resolve to the
//     canonical UUID,
//   - duplicates collapse,
//   - the on-disk wire format is uniformly UUIDs (post-Phase-1).
//
// Pass-through behavior when the store is nil (e.g. in tests that don't
// wire a Store): UUIDs are kept, non-UUIDs are dropped, duplicates collapse.
// This keeps tests honest about the post-migration shape.
func CanonicalizeProjectIDs(s *Store, raw []string) []string {
	if len(raw) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))

	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}

		var canonicalUUID string
		if s == nil {
			// No store available — accept UUIDs verbatim, drop slugs.
			if _, err := uuid.Parse(r); err == nil {
				canonicalUUID = r
			} else {
				continue
			}
		} else if p := s.GetByUUID(r); p != nil {
			canonicalUUID = p.UUID
		} else if p := s.GetBySlug(r); p != nil {
			canonicalUUID = p.UUID
		} else if _, err := uuid.Parse(r); err == nil {
			// Looks like a UUID but isn't in the store — dangling
			// reference (deleted project, race with Refresh, etc).
			// Drop it; canonicalizer's job is to keep the wire format
			// honest, not to resurrect missing projects.
			continue
		} else {
			// Looks like a slug but no project matches — likely a
			// hand-typed reference to a project that doesn't exist
			// yet. Drop it; SyncFromVault is the path that backfills
			// these, not write paths.
			continue
		}

		if seen[canonicalUUID] {
			continue
		}
		seen[canonicalUUID] = true
		out = append(out, canonicalUUID)
	}

	return out
}
