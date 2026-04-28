// Package intent classifies ingested content into one of five intentions
// using a single Claude CLI call. The result drives template selection
// and extraction strategy in later pipeline stages.
package intent

// Intention is one of five categories the user uses to describe why they
// saved a piece of content.
type Intention string

const (
	// Learning: content the user wants to absorb.
	Learning Intention = "learning"
	// Information: facts the user wants Claude to reuse later.
	Information Intention = "information"
	// Research: info in service of a project, with provenance.
	Research Intention = "research"
	// ProjectApplication: directly useful to an active project.
	ProjectApplication Intention = "project-application"
	// Other: catch-all; Claude proposes a structure.
	Other Intention = "other"
)

// All returns every supported intention in a stable order.
func All() []Intention {
	return []Intention{Learning, Information, Research, ProjectApplication, Other}
}

// IsValid reports whether s names a supported intention.
func IsValid(s string) bool {
	for _, i := range All() {
		if string(i) == s {
			return true
		}
	}
	return false
}

// PreviewResult is what the classifier returns for Claude's proposal.
// The dashboard shows it to the user as an editable preview.
//
// SuggestedProjects holds project UUIDs taken from the active set. When
// no active project fits but the content clearly belongs to a *new*
// project, Claude returns that name in ProposedProjectName instead — the
// FE preview surfaces a "Create new project «foo»?" affordance rather
// than auto-creating one (Phase 4 removed the silent-create behavior).
type PreviewResult struct {
	Intention            Intention `json:"intention"`
	Confidence           float64   `json:"confidence"`
	Summary              string    `json:"summary"`
	SuggestedTitle       string    `json:"suggested_title,omitempty"`
	SuggestedProjects    []string  `json:"suggested_project_ids,omitempty"`
	SuggestedTags        []string  `json:"suggested_tags,omitempty"`
	ProposedProjectName  string    `json:"proposed_project_name,omitempty"`
	Reasoning            string    `json:"reasoning,omitempty"`
}
