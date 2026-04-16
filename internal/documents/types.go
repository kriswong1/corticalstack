// Package documents models the vault/documents/ folder as a real
// store with stage-aware listing. Documents are markdown files
// produced by the ingest pipeline (or hand-dropped) that don't fit
// the more specific Product / Meeting / Prototype shapes — long-form
// articles, reference notes, drafts, finished pieces.
//
// Pre-refactor, documents had no model: the dashboard counted them
// by walking a folder. The unified dashboard's row-2 cards need a
// stage distribution and a clickable items table per type, so this
// package adds the minimum read surface to support that without
// touching the existing ingest writers (a document still lands on
// disk the same way as before).
package documents

import (
	"time"

	"github.com/kriswong/corticalstack/internal/stage"
)

// Document is one markdown note in vault/documents/. Stage is
// derived from frontmatter on read, falling back to stage.StageNeed
// for the (very common) case where the file has no stage field at
// all.
type Document struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Path     string      `json:"path"`
	Stage    stage.Stage `json:"stage"`
	Tags     []string    `json:"tags,omitempty"`
	Source   string      `json:"source,omitempty"` // source URL or original filename
	Projects []string    `json:"projects,omitempty"`
	Created  time.Time   `json:"created"`
	Updated  time.Time   `json:"updated,omitempty"`
}
