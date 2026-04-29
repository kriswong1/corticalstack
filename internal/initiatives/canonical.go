package initiatives

import (
	"strings"

	"github.com/google/uuid"
)

// CanonicalizeInitiativeID maps a single UUID-or-slug input into its
// canonical UUID form, or returns "" when the reference is unknown.
//
// Mirrors internal/projects.CanonicalizeProjectIDs's per-id semantics
// but operates on a single value: a Project carries at most one
// initiative_id, so the array form isn't needed. Use this whenever a
// project handler accepts an `initiative_id` field that may have come
// from a hand-typed slug or a stale UUID — the caller writes the
// canonical UUID to disk so the wire format stays uniform.
//
// Pass-through behavior when the store is nil: UUIDs accepted, slugs
// dropped (returns "").
func CanonicalizeInitiativeID(s *Store, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if s == nil {
		if _, err := uuid.Parse(raw); err == nil {
			return raw
		}
		return ""
	}
	if i := s.GetByUUID(raw); i != nil {
		return i.UUID
	}
	if i := s.GetBySlug(raw); i != nil {
		return i.UUID
	}
	// Looks like a UUID but isn't in the store — dangling. Drop.
	if _, err := uuid.Parse(raw); err == nil {
		return ""
	}
	// Looks like a slug but no initiative matches. Drop.
	return ""
}
