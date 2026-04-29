package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kriswong/corticalstack/internal/prds"
)

// GeneratePreview is the dry-run shape for the L6 button.
type GeneratePreview struct {
	PRDID                  string   `json:"prd_id"`
	IssuesToCreate         int      `json:"issues_to_create"`
	IssuesAlreadyMapped    int      `json:"issues_already_mapped"`
	MilestonesToCreate     int      `json:"milestones_to_create"`
	MilestonesAlreadyMapped int     `json:"milestones_already_mapped"`
	NewCriterionTexts      []string `json:"new_criterion_texts,omitempty"`
	NewMilestoneNames      []string `json:"new_milestone_names,omitempty"`
	Warnings               []string `json:"warnings,omitempty"`
}

// GenerateResult is the outcome of a confirmed generate run.
type GenerateResult struct {
	PRDID            string      `json:"prd_id"`
	IssuesCreated    []string    `json:"issues_created,omitempty"`     // criterion text snippets
	MilestonesCreated []string   `json:"milestones_created,omitempty"`
	Errors           []SyncError `json:"errors,omitempty"`
}

// linearSidecar is the per-PRD on-disk map.
//
// Keyed by criterionHash so a renamed criterion shows up as a new
// hash → new Issue (orphan handling is manual in Linear, per Q8).
type linearSidecar struct {
	Criteria   map[string]string `json:"criteria"`   // criterion hash → linear issue id
	Milestones map[string]string `json:"milestones"` // slice plan name → linear milestone id
}

func newSidecar() *linearSidecar {
	return &linearSidecar{
		Criteria:   map[string]string{},
		Milestones: map[string]string{},
	}
}

// GenerateIssuesFromPRD reads the linked PRD's body, parses §S4.9 +
// §S4.5 + Slice Plan, and creates Linear Issues for every criterion
// not already in the sidecar map. Project Milestones for every Slice
// Plan row not already mapped. Existing entries are never modified
// (Q8 additive lock).
//
// dryRun=true short-circuits before any writes and returns a preview.
func (o *Orchestrator) GenerateIssuesFromPRD(ctx context.Context, projectIDOrSlug string, dryRun bool) (*GeneratePreview, *GenerateResult, error) {
	if o.Client == nil || !o.Client.configured() {
		return nil, nil, fmt.Errorf("linear: not configured")
	}
	if o.Stores.Projects == nil || o.Stores.PRDs == nil {
		return nil, nil, fmt.Errorf("linear: stores not wired")
	}
	p := o.Stores.Projects.Get(projectIDOrSlug)
	if p == nil {
		return nil, nil, fmt.Errorf("project %q not found", projectIDOrSlug)
	}
	if p.LinearProjectID == "" {
		return nil, nil, fmt.Errorf("project not yet synced to Linear (run Sync to Linear first)")
	}

	// Pick the most recent PRD linked to this project. The sidecar is
	// per-PRD, so a project with multiple PRDs would need this called
	// once per PRD — left to the caller (UI surfaces a button per PRD).
	linked := o.linkedPRDs(p.UUID)
	if len(linked) == 0 {
		return nil, nil, fmt.Errorf("project has no linked PRDs")
	}
	prd := primaryPRD(linked)

	preview := &GeneratePreview{PRDID: prd.ID}

	criteria := ParseAcceptanceCriteria(prd.Body)
	if len(criteria) == 0 {
		preview.Warnings = append(preview.Warnings, "no Acceptance Criteria checkboxes found in §S4.9")
	}
	examples := ParseBehaviorExamples(prd.Body)
	slices := ParseSlicePlan(prd.Body)

	sidecarPath := sidecarPathFor(o.Stores.PRDs, prd)
	sidecar, _ := readSidecar(sidecarPath)
	if sidecar == nil {
		sidecar = newSidecar()
	}

	// Diff: which criteria are new?
	var newCriteria []Criterion
	for _, c := range criteria {
		if _, ok := sidecar.Criteria[c.Hash]; ok {
			preview.IssuesAlreadyMapped++
			continue
		}
		newCriteria = append(newCriteria, c)
	}
	preview.IssuesToCreate = len(newCriteria)
	for _, c := range newCriteria {
		preview.NewCriterionTexts = append(preview.NewCriterionTexts, c.Text)
	}

	// Diff: which milestones are new?
	var newSlices []SlicePlanRow
	for _, s := range slices {
		if _, ok := sidecar.Milestones[s.Name]; ok {
			preview.MilestonesAlreadyMapped++
			continue
		}
		newSlices = append(newSlices, s)
	}
	preview.MilestonesToCreate = len(newSlices)
	for _, s := range newSlices {
		preview.NewMilestoneNames = append(preview.NewMilestoneNames, s.Name)
	}

	if dryRun {
		return preview, nil, nil
	}

	result := &GenerateResult{PRDID: prd.ID}
	teamID, err := o.Client.ResolveTeamID(ctx, o.DefaultTeam)
	if err != nil {
		return preview, result, fmt.Errorf("resolve team: %w", err)
	}

	// Step 1: create new milestones first so issues can reference them.
	milestoneIDs := map[string]string{}
	for k, v := range sidecar.Milestones {
		milestoneIDs[k] = v
	}
	for _, s := range newSlices {
		mID, err := o.Client.UpsertProjectMilestone(ctx, "", MilestoneInput{
			ProjectID:  p.LinearProjectID,
			Name:       s.Name,
			TargetDate: s.TargetDate,
		})
		if err != nil {
			result.Errors = append(result.Errors, SyncError{Entity: "milestone:" + s.Name, Err: err.Error()})
			continue
		}
		milestoneIDs[s.Name] = mID
		sidecar.Milestones[s.Name] = mID
		result.MilestonesCreated = append(result.MilestonesCreated, s.Name)
	}

	// Step 2: create new issues. Body = criterion text + behavior
	// examples (verbatim) so QA scenarios travel with the work item.
	if _, err := o.loadOrBootstrapStateMap(ctx, teamID); err != nil {
		// Non-fatal — generated issues land in the team's default
		// starting state when we can't compute the mapping. Log and
		// proceed.
		slog.Warn("linear generate: state map unavailable; using team defaults", "error", err)
	}

	for _, c := range newCriteria {
		body := c.Text
		if examples != "" {
			body = c.Text + "\n\n---\n\n" + examples
		}
		// Heuristic: if a slice plan row's name appears in the
		// criterion text (case-insensitive substring), attach the
		// matching milestone. Cheap and good enough until users
		// signal they want explicit cross-references.
		var milestoneID string
		for sliceName, mID := range milestoneIDs {
			if containsFold(c.Text, sliceName) {
				milestoneID = mID
				break
			}
		}
		issueIn := IssueInput{
			Title:              truncate(c.Text, 200),
			Description:        body,
			TeamID:             teamID,
			ProjectID:          p.LinearProjectID,
			ProjectMilestoneID: milestoneID,
		}
		newID, err := o.Client.UpsertIssue(ctx, "", issueIn)
		if err != nil {
			result.Errors = append(result.Errors, SyncError{Entity: "issue:" + c.Hash[:8], Err: err.Error()})
			continue
		}
		sidecar.Criteria[c.Hash] = newID
		result.IssuesCreated = append(result.IssuesCreated, c.Text)
	}

	// Persist sidecar. Best-effort — not fatal if disk write fails,
	// but the next run would create duplicates so log loudly.
	if err := writeSidecar(sidecarPath, sidecar); err != nil {
		slog.Error("linear: persist sidecar failed; next generate run may duplicate", "path", sidecarPath, "error", err)
		result.Errors = append(result.Errors, SyncError{Entity: "sidecar", Err: err.Error()})
	}

	return preview, result, nil
}

// sidecarPathFor returns the disk location of the linear-mapping
// sidecar for a PRD. Sibling of the PRD file, ".linear.json" suffix.
func sidecarPathFor(store *prds.Store, p *prds.PRD) string {
	if store == nil || p == nil {
		return ""
	}
	v := store.Vault()
	if v == nil {
		return ""
	}
	if p.Path != "" {
		return filepath.Join(v.Path(), p.Path+".linear.json")
	}
	return filepath.Join(v.Path(), "prds", p.ID+".linear.json")
}

func readSidecar(path string) (*linearSidecar, error) {
	if path == "" {
		return nil, fmt.Errorf("empty sidecar path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newSidecar(), nil
		}
		return nil, err
	}
	var s linearSidecar
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Criteria == nil {
		s.Criteria = map[string]string{}
	}
	if s.Milestones == nil {
		s.Milestones = map[string]string{}
	}
	return &s, nil
}

func writeSidecar(path string, s *linearSidecar) error {
	if path == "" {
		return fmt.Errorf("empty sidecar path")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func containsFold(haystack, needle string) bool {
	if haystack == "" || needle == "" {
		return false
	}
	return indexFold(haystack, needle) >= 0
}

func indexFold(s, sub string) int {
	if len(sub) > len(s) {
		return -1
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
