// Package prds turns ShapeUp pitches into full PRDs by augmenting with
// engineering, design, and product context pulled deterministically from
// the vault. The synthesis step is a single Claude call that produces
// structured JSON matching the PRD template.
package prds

import (
	"time"

	"github.com/kriswong/corticalstack/internal/questions"
)

// Status is the lifecycle state of a PRD.
type Status string

const (
	StatusDraft    Status = "draft"
	StatusReview   Status = "review"
	StatusApproved Status = "approved"
	StatusShipped  Status = "shipped"
	StatusArchived Status = "archived"
)

// PRD is a fully synthesized product requirements document.
type PRD struct {
	ID           string    `json:"id"                   yaml:"id"`
	Version      int       `json:"version"              yaml:"version"`
	Status       Status    `json:"status"               yaml:"status"`
	Title        string    `json:"title"                yaml:"title"`
	SourcePitch  string    `json:"source_pitch"         yaml:"source_pitch"`
	SourceThread string    `json:"source_thread,omitempty" yaml:"source_thread,omitempty"`
	ContextRefs  []string  `json:"context_refs,omitempty"  yaml:"context_refs,omitempty"`
	Projects     []string  `json:"projects,omitempty"      yaml:"projects,omitempty"`
	OpenQuestionsCount int `json:"open_questions_count"    yaml:"open_questions_count"`
	Body         string    `json:"-"                    yaml:"-"`
	Path         string    `json:"path,omitempty"       yaml:"-"`
	Created      time.Time `json:"created"              yaml:"created"`

	// L2 (Linear integration) — set by the L3 sync flow once the PRD
	// body is mirrored as a Linear Project Document. Empty until then.
	LinearDocumentID string `json:"linear_document_id,omitempty" yaml:"linear_document_id,omitempty"`
}

// Synthesis is the structured response Claude returns; each field maps
// to a PRD template section.
type Synthesis struct {
	Title              string   `json:"title"`
	Problem            string   `json:"problem"`
	Goals              []string `json:"goals"`
	NonGoals           []string `json:"non_goals"`
	UserStories        []string `json:"user_stories,omitempty"`
	FunctionalReqs     []string `json:"functional_requirements"`
	NonFunctionalReqs  []string `json:"non_functional_requirements,omitempty"`
	DesignConsiderations     []string `json:"design_considerations,omitempty"`
	EngineeringConsiderations []string `json:"engineering_considerations,omitempty"`
	RolloutPlan        []string `json:"rollout_plan,omitempty"`
	SuccessMetrics     []string `json:"success_metrics,omitempty"`
	OpenQuestions      []string `json:"open_questions,omitempty"`
	References         []string `json:"references,omitempty"`
}

// CreateRequest is POST /api/prds.
//
// PreviousOutput + IsRefine are internal fields set by the refine
// handler so synthesis can show Claude the current PRD body and ask
// for a modification rather than a fresh draft. They're not exposed
// on the public create endpoint (json:"-" — no inbound field names).
type CreateRequest struct {
	PitchPath         string               `json:"pitch_path"`
	ExtraContextTags  []string             `json:"extra_context_tags,omitempty"`
	ExtraContextPaths []string             `json:"extra_context_paths,omitempty"`
	ProjectIDs        []string             `json:"project_ids,omitempty"`
	Hints             string               `json:"hints,omitempty"`
	Questions         []questions.Question `json:"questions,omitempty"`
	Answers           []questions.Answer   `json:"answers,omitempty"`
	PreviousOutput    string               `json:"-"`
	IsRefine          bool                 `json:"-"`
}

// QuestionsRequest is POST /api/prds/questions.
type QuestionsRequest struct {
	PitchPath         string   `json:"pitch_path"`
	ExtraContextTags  []string `json:"extra_context_tags,omitempty"`
	ExtraContextPaths []string `json:"extra_context_paths,omitempty"`
	ProjectIDs        []string `json:"project_ids,omitempty"`
}

// RefineRequest is POST /api/prds/{id}/refine.
//
// Only the three fields below are client-supplied. On refine, the server
// looks up the existing PRD and constructs an internal CreateRequest
// that additionally carries PreviousOutput (the prior rendered body)
// and IsRefine=true — those fields are internal-only (json:"-") and
// not accepted from the wire.
type RefineRequest struct {
	Hints     string               `json:"hints,omitempty"`
	Questions []questions.Question `json:"questions,omitempty"`
	Answers   []questions.Answer   `json:"answers,omitempty"`
}

// VersionInfo describes one archived version of a PRD. Returned by
// Store.ListVersions so the UI can (eventually) render a switcher;
// for now the refine endpoint just writes archives and leaves the
// viewer work to a follow-up.
type VersionInfo struct {
	Version int       `json:"version"`
	Created time.Time `json:"created"`
	Hints   string    `json:"hints,omitempty"`
}
