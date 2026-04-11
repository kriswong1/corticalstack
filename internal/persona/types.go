// Package persona manages the three vault-backed files — SOUL, USER, and
// MEMORY — that are injected into every Claude CLI call so output is
// tailored to the user's context and extraction preferences.
//
// On first install, the loader copies embedded templates into the vault so
// the user has working files to edit. Edits made either in the dashboard
// or directly in Obsidian are picked up on the next Claude call via an
// mtime-based cache.
package persona

// Name identifies one of the three persona files.
type Name string

const (
	NameSoul   Name = "soul"
	NameUser   Name = "user"
	NameMemory Name = "memory"
)

// AllNames returns every persona name in canonical order.
func AllNames() []Name {
	return []Name{NameSoul, NameUser, NameMemory}
}

// IsValid reports whether s names a real persona file.
func IsValid(s string) bool {
	for _, n := range AllNames() {
		if string(n) == s {
			return true
		}
	}
	return false
}

// File is the vault-relative filename for a persona, in the form `SOUL.md`.
func (n Name) File() string {
	switch n {
	case NameSoul:
		return "SOUL.md"
	case NameUser:
		return "USER.md"
	case NameMemory:
		return "MEMORY.md"
	}
	return ""
}

// Budget returns the maximum number of characters of this file that get
// sent to Claude in any single call. The on-disk file is never truncated;
// only the Claude-facing copy is trimmed.
func (n Name) Budget() int {
	switch n {
	case NameSoul:
		return 3500
	case NameUser:
		return 2000
	case NameMemory:
		return 2500
	}
	return 0
}

// InitResult reports which files were freshly bootstrapped during
// InitIfMissing. Useful for dashboard welcome banners.
type InitResult struct {
	Created []Name `json:"created"`
}
