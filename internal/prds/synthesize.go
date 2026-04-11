package prds

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/vault"
)

// Synthesizer runs the full PRD generation flow:
//
//  1. Read the pitch.
//  2. Retrieve context buckets deterministically.
//  3. Build a Claude prompt with the PRD schema + pitch + context.
//  4. Parse the JSON response into a Synthesis struct.
//  5. Render the markdown body.
//  6. Create action items for every OpenQuestion (optional, wired by handler).
type Synthesizer struct {
	workingDir  string
	model       string
	retriever   *Retriever
	actionStore *actions.Store
	persona     *persona.Loader
}

// NewSynthesizer wires a synthesizer bound to a retriever and action store.
// actionStore may be nil if you don't want open questions to flow into actions.
// personaLoader may be nil to skip persona context injection.
func NewSynthesizer(workingDir, model string, r *Retriever, as *actions.Store, p *persona.Loader) *Synthesizer {
	return &Synthesizer{
		workingDir:  workingDir,
		model:       model,
		retriever:   r,
		actionStore: as,
		persona:     p,
	}
}

// Synthesize produces a ready-to-store PRD from a pitch path plus extras.
func (s *Synthesizer) Synthesize(v *vault.Vault, req CreateRequest) (*PRD, error) {
	pitchBody, err := v.ReadFile(req.PitchPath)
	if err != nil {
		return nil, fmt.Errorf("reading pitch: %w", err)
	}

	context, err := s.retriever.Retrieve(req.ProjectIDs, req.ExtraContextTags, req.ExtraContextPaths)
	if err != nil {
		return nil, fmt.Errorf("retrieving context: %w", err)
	}

	prompt := s.persona.BuildContextPrompt() + buildPRDPrompt(req.PitchPath, pitchBody, context)

	ag := &agent.Agent{
		Model:      s.model,
		MaxTurns:   1,
		WorkingDir: s.workingDir,
	}
	raw, err := ag.RunSimple(ctx(), prompt)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}

	parsed, err := parseSynthesis(raw)
	if err != nil {
		return nil, err
	}

	contextRefs := make([]string, 0, len(context))
	for _, n := range context {
		contextRefs = append(contextRefs, n.Path)
	}

	prd := &PRD{
		Title:              parsed.Title,
		SourcePitch:        req.PitchPath,
		ContextRefs:        contextRefs,
		Projects:           req.ProjectIDs,
		OpenQuestionsCount: len(parsed.OpenQuestions),
		Body:               renderPRDBody(parsed),
	}

	// Optional: create action items for every open question.
	if s.actionStore != nil && len(parsed.OpenQuestions) > 0 {
		for _, q := range parsed.OpenQuestions {
			_, _ = s.actionStore.Upsert(&actions.Action{
				Description: "[PRD open question] " + q,
				Owner:       "TBD",
				Status:      actions.StatusPending,
				ProjectIDs:  req.ProjectIDs,
				SourceTitle: parsed.Title,
			})
		}
	}

	return prd, nil
}

// buildPRDPrompt creates the Claude prompt with schema, pitch, and context.
func buildPRDPrompt(pitchPath, pitch string, context []RetrievedNote) string {
	var b strings.Builder
	b.WriteString("You are a senior product manager. Turn the pitch below into a full PRD.\n\n")

	b.WriteString("## Pitch (primary source)\n\n")
	b.WriteString(fmt.Sprintf("*From `%s`*\n\n", pitchPath))
	if len(pitch) > 15000 {
		pitch = pitch[:15000] + "\n\n[...truncated]"
	}
	b.WriteString(pitch)
	b.WriteString("\n\n")

	if len(context) > 0 {
		b.WriteString("## Context (cite by path in References section)\n\n")
		for _, n := range context {
			b.WriteString(fmt.Sprintf("### [%s] %s (`%s`)\n\n", n.Bucket, n.Title, n.Path))
			body := n.Body
			if len(body) > 3000 {
				body = body[:3000] + "\n\n[...truncated]"
			}
			b.WriteString(body)
			b.WriteString("\n\n---\n\n")
		}
	}

	b.WriteString("## Output format\n\n")
	b.WriteString("Respond with ONLY a JSON object (no fences, no prose) matching:\n\n")
	b.WriteString("```\n{\n")
	b.WriteString(`  "title": "PRD title",` + "\n")
	b.WriteString(`  "problem": "2-3 sentence problem framing",` + "\n")
	b.WriteString(`  "goals": ["goal 1", "goal 2"],` + "\n")
	b.WriteString(`  "non_goals": ["non-goal 1"],` + "\n")
	b.WriteString(`  "user_stories": ["As a <role>, I want <capability> so that <outcome>"],` + "\n")
	b.WriteString(`  "functional_requirements": ["FR1", "FR2"],` + "\n")
	b.WriteString(`  "non_functional_requirements": ["NFR1"],` + "\n")
	b.WriteString(`  "design_considerations": ["grounded in the design context docs"],` + "\n")
	b.WriteString(`  "engineering_considerations": ["grounded in the engineering context docs"],` + "\n")
	b.WriteString(`  "rollout_plan": ["phase 1", "phase 2"],` + "\n")
	b.WriteString(`  "success_metrics": ["metric 1"],` + "\n")
	b.WriteString(`  "open_questions": ["question 1"],` + "\n")
	b.WriteString(`  "references": ["pitch path", "context doc paths you actually used"]` + "\n")
	b.WriteString("}\n```\n\n")
	b.WriteString("Rules:\n- Ground every design/engineering consideration in the context docs when possible.\n")
	b.WriteString("- Cite used context paths in references.\n")
	b.WriteString("- Flag everything you're unsure about as an open_question.\n")
	b.WriteString("- Respond with ONLY the JSON.\n")

	return b.String()
}

// parseSynthesis decodes Claude's JSON response into a Synthesis.
func parseSynthesis(raw string) (*Synthesis, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var s Synthesis
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, fmt.Errorf("parsing PRD synthesis: %w; raw: %.300s", err, raw)
	}
	if s.Title == "" {
		return nil, fmt.Errorf("PRD synthesis missing title")
	}
	return &s, nil
}

// renderPRDBody turns a parsed Synthesis into the canonical PRD markdown.
func renderPRDBody(s *Synthesis) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", s.Title))

	if s.Problem != "" {
		b.WriteString("## Problem\n\n")
		b.WriteString(s.Problem)
		b.WriteString("\n\n")
	}

	writeList(&b, "Goals", s.Goals)
	writeList(&b, "Non-goals", s.NonGoals)
	writeList(&b, "User Stories", s.UserStories)
	writeList(&b, "Functional Requirements", s.FunctionalReqs)
	writeList(&b, "Non-functional Requirements", s.NonFunctionalReqs)
	writeList(&b, "Design Considerations", s.DesignConsiderations)
	writeList(&b, "Engineering Considerations", s.EngineeringConsiderations)
	writeList(&b, "Rollout Plan", s.RolloutPlan)
	writeList(&b, "Success Metrics", s.SuccessMetrics)
	writeList(&b, "Open Questions", s.OpenQuestions)

	if len(s.References) > 0 {
		b.WriteString("## References\n\n")
		for _, ref := range s.References {
			// Obsidian wiki-link style
			b.WriteString(fmt.Sprintf("- [[%s]]\n", strings.TrimSuffix(ref, ".md")))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func writeList(b *strings.Builder, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString(fmt.Sprintf("## %s\n\n", heading))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// ctx returns a background context for the Claude call; split out to keep
// the Synthesize body readable.
func ctx() context.Context { return context.Background() }
