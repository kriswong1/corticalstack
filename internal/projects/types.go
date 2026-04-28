// Package projects manages user projects stored inside the Obsidian vault
// at $VAULT_PATH/projects/<slug>/project.md. The vault is the source of truth;
// this package is a thin reader/writer that keeps an in-memory cache.
//
// Identity: each project has a UUID (canonical, stable across rename) and a
// Slug (filesystem directory name, renameable display alias). Note frontmatter
// `projects:` arrays carry UUIDs after migration; the Slug only appears on
// the project's own manifest. The store caches by both keys.
package projects

import "time"

// Status is the lifecycle state of a project.
type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

// Project is a user-defined project the ingest pipeline can associate notes
// and action items with.
//
// UUID is the canonical identity — referenced from every other entity's
// `projects:` frontmatter. Slug is a renameable display alias and the
// on-disk directory name. Renaming a project (PATCH name) regenerates Slug
// and renames the directory, but UUID stays put so cross-references survive.
type Project struct {
	UUID        string    `json:"uuid"                  yaml:"uuid"`
	Slug        string    `json:"slug"                  yaml:"id"` // on-disk yaml key stays "id" for backward compat with hand-written manifests
	Name        string    `json:"name"                  yaml:"name"`
	Status      Status    `json:"status"                yaml:"status"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"        yaml:"tags,omitempty"`
	Created     time.Time `json:"created"               yaml:"created"`
}

// CreateRequest is the payload for POST /api/projects.
type CreateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// UpdateRequest is the payload for PATCH /api/projects/{id}. All fields
// are optional — nil means "leave unchanged". Name change triggers a
// directory rename (slug regen) but UUID stays stable.
type UpdateRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      *Status   `json:"status,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
}
