// Package projects manages user projects stored inside the Obsidian vault
// at $VAULT_PATH/projects/<id>/project.md. The vault is the source of truth;
// this package is a thin reader/writer that keeps an in-memory cache.
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
type Project struct {
	ID          string    `json:"id"                    yaml:"id"`
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
