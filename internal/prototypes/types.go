// Package prototypes turns product docs (pitches, shapes, use cases) into
// design-md specs that external AI design tools (v0, bolt.new, Figma Make,
// etc.) can consume to generate high-fidelity prototypes.
package prototypes

import (
	"time"

	"github.com/kriswong/corticalstack/internal/questions"
	"github.com/kriswong/corticalstack/internal/stage"
)

// Prototype is a generated design spec stored in the vault.
//
// Status is the legacy lifecycle field (`draft` | `exported`) that
// the older PrototypesPage still reads. Stage is the new dashboard-
// facing field (Need / In-Progress / Final) populated by Normalize on
// read so old notes still map to a sensible bucket — see fromNote in
// store.go for the migration path.
type Prototype struct {
	ID           string      `json:"id"                      yaml:"id"`
	Title        string      `json:"title"                   yaml:"title"`
	Format       string      `json:"format"                  yaml:"format"` // screen-flow | component-spec | user-journey | interactive-html
	SourceRefs   []string    `json:"source_refs,omitempty"   yaml:"source_refs,omitempty"`
	SourceThread string      `json:"source_thread,omitempty" yaml:"source_thread,omitempty"`
	Projects     []string    `json:"projects,omitempty"      yaml:"projects,omitempty"`
	Status       string      `json:"status"                  yaml:"status"` // legacy: draft | exported
	Stage        stage.Stage `json:"stage"                   yaml:"stage"`  // dashboard: need | in_progress | final
	// Version is the current iteration number. Starts at 1 on create
	// and increments on every successful refine. Past versions are
	// archived in `versions/v{n}/` under the prototype folder.
	Version    int       `json:"version"                 yaml:"version"`
	Spec       string    `json:"spec,omitempty"          yaml:"-"`
	HTMLBody   string    `json:"-"                       yaml:"-"` // populated for interactive-html, written as prototype.html
	HasHTML    bool      `json:"has_html"                yaml:"-"`
	FolderPath string    `json:"folder_path,omitempty"   yaml:"-"`
	Created    time.Time `json:"created"                 yaml:"created"`
	Updated    time.Time `json:"updated,omitempty"       yaml:"updated,omitempty"`
}

// VersionInfo describes one archived version of a prototype. Returned
// by Store.ListVersions so the UI can render the version switcher.
type VersionInfo struct {
	Version int       `json:"version"`
	Created time.Time `json:"created"`
	Hints   string    `json:"hints,omitempty"`
	HasHTML bool      `json:"has_html"`
}

// RefineRequest is POST /api/prototypes/{id}/refine.
//
// Only the three fields below are client-supplied. On refine, the
// server looks up the existing prototype and constructs an internal
// CreateRequest that *additionally* carries PreviousOutput (the prior
// spec or HTML body) and IsRefine=true — those fields are internal-only
// on CreateRequest (json:"-") and are not accepted from the wire.
type RefineRequest struct {
	Hints     string               `json:"hints,omitempty"`
	Questions []questions.Question `json:"questions,omitempty"`
	Answers   []questions.Answer   `json:"answers,omitempty"`
}

// CreateRequest is POST /api/prototypes.
//
// PreviousOutput + IsRefine are internal fields set by the refine
// handler so synthesis can show Claude the current version and ask
// for a modification rather than a fresh draft. They're not exposed
// on the public create endpoint (no json tag matches an inbound
// field name a caller would set).
type CreateRequest struct {
	Title          string               `json:"title"`
	SourcePaths    []string             `json:"source_paths"`
	Format         string               `json:"format"`
	Hints          string               `json:"hints,omitempty"`
	ProjectIDs     []string             `json:"project_ids,omitempty"`
	SourceThread   string               `json:"source_thread,omitempty"`
	Questions      []questions.Question `json:"questions,omitempty"`
	Answers        []questions.Answer   `json:"answers,omitempty"`
	PreviousOutput string               `json:"-"`
	IsRefine       bool                 `json:"-"`
}

// QuestionsRequest is POST /api/prototypes/questions.
type QuestionsRequest struct {
	Title       string   `json:"title"`
	Format      string   `json:"format"`
	SourcePaths []string `json:"source_paths"`
	Hints       string   `json:"hints,omitempty"`
}
