// Package shapeup implements Ryan Singer's ShapeUp product pipeline:
// Raw Idea → Frame → Shape → Breadboard → Pitch. Each stage is a markdown
// note in the vault, and every stage carries a shared thread UUID so the
// full arc of a single idea can be reconstructed.
package shapeup

import "time"

// Stage is one of the five ShapeUp stages.
type Stage string

const (
	StageRaw        Stage = "raw"
	StageFrame      Stage = "frame"
	StageShape      Stage = "shape"
	StageBreadboard Stage = "breadboard"
	StagePitch      Stage = "pitch"
)

// AllStages returns every stage in canonical order.
func AllStages() []Stage {
	return []Stage{StageRaw, StageFrame, StageShape, StageBreadboard, StagePitch}
}

// NextStage returns the stage that logically follows s, or empty string if
// s is already the last stage.
func NextStage(s Stage) Stage {
	order := AllStages()
	for i, v := range order {
		if v == s && i+1 < len(order) {
			return order[i+1]
		}
	}
	return ""
}

// IsValidStage reports whether s names a real stage.
func IsValidStage(s string) bool {
	for _, v := range AllStages() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// Artifact is a single stage document in the vault.
type Artifact struct {
	ID        string    `json:"id"                yaml:"id"`
	Stage     Stage     `json:"stage"             yaml:"stage"`
	Thread    string    `json:"thread"            yaml:"thread"`
	ParentID  string    `json:"parent_id,omitempty" yaml:"parent_id,omitempty"`
	Title     string    `json:"title"             yaml:"title"`
	Path      string    `json:"path"              yaml:"path"`
	Projects  []string  `json:"projects,omitempty" yaml:"projects,omitempty"`
	Appetite  string    `json:"appetite,omitempty" yaml:"appetite,omitempty"`
	Status    string    `json:"status"            yaml:"status"`
	Created   time.Time `json:"created"           yaml:"created"`
	Body      string    `json:"-"                 yaml:"-"`
}

// Thread groups all artifacts sharing the same thread UUID in canonical
// stage order (raw → frame → shape → breadboard → pitch).
type Thread struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Projects  []string    `json:"projects,omitempty"`
	CurrentStage Stage    `json:"current_stage"`
	Artifacts []*Artifact `json:"artifacts"`
}

// CreateIdeaRequest is POST /api/shapeup/idea.
type CreateIdeaRequest struct {
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	ProjectIDs []string `json:"project_ids,omitempty"`
}

// AdvanceRequest is POST /api/shapeup/threads/{thread}/advance.
type AdvanceRequest struct {
	TargetStage string `json:"target_stage"`
	Hints       string `json:"hints,omitempty"`
}
