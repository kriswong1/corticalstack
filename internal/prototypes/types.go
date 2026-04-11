// Package prototypes turns product docs (pitches, shapes, use cases) into
// design-md specs that external AI design tools (v0, bolt.new, Figma Make,
// etc.) can consume to generate high-fidelity prototypes.
package prototypes

import "time"

// Prototype is a generated design spec stored in the vault.
type Prototype struct {
	ID            string    `json:"id"                      yaml:"id"`
	Title         string    `json:"title"                   yaml:"title"`
	Format        string    `json:"format"                  yaml:"format"` // screen-flow | component-spec | user-journey
	SourceRefs    []string  `json:"source_refs,omitempty"   yaml:"source_refs,omitempty"`
	SourceThread  string    `json:"source_thread,omitempty" yaml:"source_thread,omitempty"`
	Projects      []string  `json:"projects,omitempty"      yaml:"projects,omitempty"`
	Status        string    `json:"status"                  yaml:"status"` // draft | exported
	Spec          string    `json:"spec,omitempty"          yaml:"-"`
	FolderPath    string    `json:"folder_path,omitempty"   yaml:"-"`
	Created       time.Time `json:"created"                 yaml:"created"`
}

// CreateRequest is POST /api/prototypes.
type CreateRequest struct {
	Title        string   `json:"title"`
	SourcePaths  []string `json:"source_paths"`
	Format       string   `json:"format"`
	Hints        string   `json:"hints,omitempty"`
	ProjectIDs   []string `json:"project_ids,omitempty"`
	SourceThread string   `json:"source_thread,omitempty"`
}
