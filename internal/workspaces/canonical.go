package workspaces

import (
	"strings"

	"github.com/google/uuid"
)

// CanonicalizeWorkspaceID maps a UUID-or-slug input into its canonical
// UUID form, or returns "" when the reference is unknown. Mirrors
// initiatives.CanonicalizeInitiativeID for use at the project handler
// boundary.
func CanonicalizeWorkspaceID(s *Store, raw string) string {
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
	if w := s.GetByUUID(raw); w != nil {
		return w.UUID
	}
	if w := s.GetBySlug(raw); w != nil {
		return w.UUID
	}
	if _, err := uuid.Parse(raw); err == nil {
		return ""
	}
	return ""
}
