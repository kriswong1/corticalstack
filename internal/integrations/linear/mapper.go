package linear

import (
	"regexp"
	"strings"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/initiatives"
	"github.com/kriswong/corticalstack/internal/prds"
	"github.com/kriswong/corticalstack/internal/projects"
)

// InitiativeToInput maps a CorticalStack initiative to the Linear
// initiativeCreate/initiativeUpdate input shape. Vault-owned fields
// only — workflow state (Status / TargetDate) flows inbound via L5
// webhooks per docs/linear/README.md §3 Fork D conflict policy.
func InitiativeToInput(i *initiatives.Initiative) InitiativeInput {
	return InitiativeInput{
		Name:        i.Name,
		Description: i.Description,
	}
}

// ProjectToInput maps a CorticalStack project to the Linear
// projectCreate/projectUpdate input shape.
//
// `description` carries the Pitch's Problem + Appetite paragraphs
// extracted from the linked PRD body (per Fork A2 — the full PRD body
// goes to a Project Document; the description stays scannable). If no
// linked PRD or extraction fails, falls back to the project's own
// description field.
func ProjectToInput(p *projects.Project, prd *prds.PRD, teamIDs []string, linearInitiativeID string) ProjectInput {
	desc := p.Description
	if prd != nil {
		if extracted := extractProblemAndAppetite(prd.Body); extracted != "" {
			desc = extracted
		}
	}
	return ProjectInput{
		Name:         p.Name,
		Description:  desc,
		TeamIDs:      teamIDs,
		InitiativeID: linearInitiativeID,
	}
}

// PRDBodyToProjectDocument maps a PRD to the Project Document input.
// Title is "<slug>-shaped-prd" so multiple PRDs on the same project
// don't collide; content is the full assembled markdown body.
func PRDBodyToProjectDocument(linearProjectID string, p *prds.PRD) DocumentInput {
	return DocumentInput{
		ProjectID: linearProjectID,
		Title:     p.ID + "-shaped-prd",
		Content:   p.Body,
	}
}

// problemHeadingPattern matches markdown headings whose text begins with
// "Problem" or "Appetite" (case-insensitive). Permissive about heading
// level (## or ### or even bolded) and trailing colon. Matches the
// shapedPRD output convention without being brittle to template tweaks.
var problemHeadingPattern = regexp.MustCompile(`(?i)^\s*(?:#{2,4}\s+)?\**\s*(problem|appetite)\b\s*:?\s*\**\s*$`)

// otherHeadingPattern matches any other "## "-or-deeper heading we'd
// stop at. Used to bound the section walk.
var anyHeadingPattern = regexp.MustCompile(`^\s*#{1,6}\s+\S`)

// extractProblemAndAppetite returns the Problem + Appetite paragraphs
// from a shaped-prd body, joined with a blank line. Empty string when
// neither section is found.
//
// Walks the body line-by-line: when it hits a "Problem" or "Appetite"
// heading it captures every following non-empty line until the next
// heading or the end of the document.
func extractProblemAndAppetite(body string) string {
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	var out []string
	var current []string
	captureMode := false
	flush := func() {
		if len(current) == 0 {
			return
		}
		text := strings.Trim(strings.Join(current, "\n"), "\n ")
		if text != "" {
			out = append(out, text)
		}
		current = nil
	}
	for _, line := range lines {
		if problemHeadingPattern.MatchString(line) {
			flush()
			captureMode = true
			continue
		}
		if captureMode && anyHeadingPattern.MatchString(line) {
			flush()
			captureMode = false
			continue
		}
		if captureMode {
			current = append(current, line)
		}
	}
	flush()
	return strings.Join(out, "\n\n")
}

// ActionToIssueInput maps a CorticalStack action to the Linear
// issueCreate/issueUpdate input shape. Caller is responsible for
// resolving Owner→AssigneeID (email lookup is not done here).
func ActionToIssueInput(a *actions.Action, teamID, projectID, stateID string) IssueInput {
	in := IssueInput{
		Title:       firstNonEmptyStr(a.Title, a.Description),
		Description: a.Description,
		TeamID:      teamID,
		ProjectID:   projectID,
		StateID:     stateID,
		Priority:    priorityToLinear(a.Priority),
		Estimate:    effortToLinear(a.Effort),
	}
	if a.Deadline != "" {
		in.DueDate = a.Deadline
	}
	return in
}

// priorityToLinear maps Action priority to Linear's 0–4 scale.
// Linear: 0 = no priority, 1 = urgent, 2 = high, 3 = normal, 4 = low.
// CorticalStack: p1 = high, p2 = medium, p3 = low.
func priorityToLinear(p actions.Priority) int {
	switch p {
	case actions.PriorityHigh:
		return 2
	case actions.PriorityMedium:
		return 3
	case actions.PriorityLow:
		return 4
	}
	return 0
}

// effortToLinear maps t-shirt sizes to Linear estimate points
// (Fibonacci-ish). 0 = unset.
func effortToLinear(e actions.Effort) int {
	switch e {
	case actions.EffortXS:
		return 1
	case actions.EffortS:
		return 2
	case actions.EffortM:
		return 3
	case actions.EffortL:
		return 5
	case actions.EffortXL:
		return 8
	}
	return 0
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
