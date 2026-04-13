// Package usecases turns product docs or free-form input into standardized
// UseCase documents stored in the vault. Each UseCase is a first-class
// artifact with a stable ID that PRDs, prototypes, and test generators can
// reference.
package usecases

import (
	"time"

	"github.com/kriswong/corticalstack/internal/questions"
)

// AltFlow is an alternative flow that branches from a step in the main flow.
type AltFlow struct {
	Name   string   `json:"name"    yaml:"name"`
	AtStep int      `json:"at_step" yaml:"at_step"`
	Flow   []string `json:"flow"    yaml:"flow"`
}

// SourceRef is a backlink to a document the UseCase was derived from.
type SourceRef struct {
	Type string `json:"type" yaml:"type"` // pitch, shape, note, freeform, ...
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
}

// UseCase is the canonical schema.
type UseCase struct {
	ID              string      `json:"id"                         yaml:"id"`
	Title           string      `json:"title"                      yaml:"title"`
	Actors          []string    `json:"actors"                     yaml:"actors"`
	SecondaryActors []string    `json:"secondary_actors,omitempty" yaml:"secondary_actors,omitempty"`
	Preconditions   []string    `json:"preconditions,omitempty"    yaml:"preconditions,omitempty"`
	MainFlow        []string    `json:"main_flow"                  yaml:"main_flow"`
	AlternativeFlows []AltFlow  `json:"alternative_flows,omitempty" yaml:"alternative_flows,omitempty"`
	Postconditions  []string    `json:"postconditions,omitempty"   yaml:"postconditions,omitempty"`
	BusinessRules   []string    `json:"business_rules,omitempty"   yaml:"business_rules,omitempty"`
	NonFunctional   []string    `json:"non_functional,omitempty"   yaml:"non_functional,omitempty"`
	Sources         []SourceRef `json:"source,omitempty"           yaml:"source,omitempty"`
	Tags            []string    `json:"tags,omitempty"             yaml:"tags,omitempty"`
	Projects        []string    `json:"projects,omitempty"         yaml:"projects,omitempty"`
	Path            string      `json:"path,omitempty"             yaml:"-"`
	Created         time.Time   `json:"created"                    yaml:"created"`
}

// FromDocRequest is POST /api/usecases/from-doc.
type FromDocRequest struct {
	SourcePath string               `json:"source_path"`
	Hint       string               `json:"hint,omitempty"`
	ProjectIDs []string             `json:"project_ids,omitempty"`
	Questions  []questions.Question `json:"questions,omitempty"`
	Answers    []questions.Answer   `json:"answers,omitempty"`
}

// FromTextRequest is POST /api/usecases/from-text.
type FromTextRequest struct {
	Description string               `json:"description"`
	ActorsHint  string               `json:"actors_hint,omitempty"`
	ProjectIDs  []string             `json:"project_ids,omitempty"`
	Questions   []questions.Question `json:"questions,omitempty"`
	Answers     []questions.Answer   `json:"answers,omitempty"`
}

// QuestionsFromDocRequest is POST /api/usecases/from-doc/questions.
type QuestionsFromDocRequest struct {
	SourcePath string `json:"source_path"`
	Hint       string `json:"hint,omitempty"`
}

// QuestionsFromTextRequest is POST /api/usecases/from-text/questions.
type QuestionsFromTextRequest struct {
	Description string `json:"description"`
	ActorsHint  string `json:"actors_hint,omitempty"`
}

// GenerateResponse is what both generation endpoints return.
type GenerateResponse struct {
	Created []*UseCase `json:"created"`
	Errors  []string   `json:"errors,omitempty"`
}
