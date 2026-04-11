// Package prds turns ShapeUp pitches into full PRDs by augmenting with
// engineering, design, and product context pulled deterministically from
// the vault. The synthesis step is a single Claude call that produces
// structured JSON matching the PRD template.
package prds

import "time"

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
type CreateRequest struct {
	PitchPath          string   `json:"pitch_path"`
	ExtraContextTags   []string `json:"extra_context_tags,omitempty"`
	ExtraContextPaths  []string `json:"extra_context_paths,omitempty"`
	ProjectIDs         []string `json:"project_ids,omitempty"`
}
