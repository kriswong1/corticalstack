package shapeup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/questions"
)

// Advancer runs Claude to generate the next stage's content given all prior
// stages in a thread. The result is a filled markdown body; the store wraps
// it into a fresh Artifact with the correct parent/thread metadata.
type Advancer struct {
	workingDir string
	model      string
	persona    *persona.Loader
	asker      *questions.Asker
}

// NewAdvancer creates an advancer bound to a working directory.
// The persona loader is optional; pass nil to skip persona context injection.
func NewAdvancer(workingDir, model string, p *persona.Loader) *Advancer {
	return &Advancer{
		workingDir: workingDir,
		model:      model,
		persona:    p,
		asker:      questions.NewAsker(workingDir, model),
	}
}

// Questions asks Claude what a human should clarify before drafting the
// target stage. Returns an empty slice if the thread context is complete.
func (a *Advancer) Questions(ctx context.Context, thread *Thread, target Stage) ([]questions.Question, error) {
	if !IsValidStage(string(target)) {
		return nil, fmt.Errorf("invalid target stage: %s", target)
	}

	goal := fmt.Sprintf("Draft the `%s` stage of a ShapeUp thread titled %q. Ask clarifying questions that will materially change the draft — things like appetite, affected users, hidden constraints, rejected directions.", target, thread.Title)

	blocks := []questions.ContextBlock{
		{Heading: "Stage guidance", Body: stageGuidance(target)},
	}
	for _, art := range thread.Artifacts {
		body := art.Body
		if len(body) > 4000 {
			body = body[:4000] + "\n\n[...truncated]"
		}
		blocks = append(blocks, questions.ContextBlock{
			Heading: fmt.Sprintf("Prior stage: %s", art.Stage),
			Body:    body,
		})
	}

	return a.asker.Ask(ctx, goal, blocks)
}

// Advance asks Claude to draft the target stage of a thread. The prior
// artifacts must already be ordered raw → frame → ... by the caller.
// answers may be nil — callers that skipped the Q&A phase just pass hints.
//
// The Claude CLI call is tagged with the thread's ID so the unified
// dashboard's per-card detail page can compute "tokens spent on this
// product idea". Calls without a thread ID (none today, but defends
// future refactors) skip the item-tag branch silently.
func (a *Advancer) Advance(ctx context.Context, thread *Thread, target Stage, hints string, qs []questions.Question, answers []questions.Answer) (string, error) {
	if !IsValidStage(string(target)) {
		return "", fmt.Errorf("invalid target stage: %s", target)
	}

	answerBlock := questions.FormatAnswers(qs, answers)
	prompt := a.persona.BuildContextPrompt() + buildAdvancePrompt(thread, target, hints, answerBlock)

	ag := &agent.Agent{
		Model:      a.model,
		MaxTurns:   10,
		WorkingDir: a.workingDir,
		CallerHint: "shapeup.advance." + string(target),
		Item:       agent.ItemContext{Type: "product", ID: thread.ID},
	}
	raw, err := ag.RunSimple(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("advance claude call: %w", err)
	}
	return stripCodeFences(raw), nil
}

func buildAdvancePrompt(thread *Thread, target Stage, hints, answerBlock string) string {
	var b strings.Builder

	b.WriteString("You are a product strategist applying Ryan Singer's Shape Up methodology.\n\n")
	b.WriteString("IMPORTANT: All the context you need is provided below in the prior stages section. Do NOT attempt to read files, search code, or use any tools. Generate the output directly from the information given.\n\n")
	// MD-11: fence semantics for every embedded user-controlled block
	// below (hints, prior-stage bodies).
	b.WriteString(questions.UntrustedFenceNotice)
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("## Task\nDraft the `%s` stage for this thread, given all prior stages. Do not repeat prior content verbatim — synthesize. Write a complete, substantial document following the structure below.\n\n", target))

	b.WriteString(stageGuidance(target))
	b.WriteString("\n\n")

	if answerBlock != "" {
		b.WriteString(answerBlock)
	}

	if hints != "" {
		b.WriteString("## Hints from the user\n")
		b.WriteString(questions.FenceUntrustedBlock(hints))
		b.WriteString("\n")
	}

	if len(thread.Projects) > 0 {
		b.WriteString(fmt.Sprintf("## Associated projects\n- %s\n\n", strings.Join(thread.Projects, "\n- ")))
	}

	b.WriteString("## Prior stages in this thread\n\n")
	for _, art := range thread.Artifacts {
		b.WriteString(fmt.Sprintf("### [%s] %s\n", art.Stage, art.Title))
		body := art.Body
		if len(body) > 8000 {
			body = body[:8000] + "\n\n[...truncated]"
		}
		b.WriteString(questions.FenceUntrustedBlock(body))
		b.WriteString("\n---\n\n")
	}

	b.WriteString("## Output\n\n")
	b.WriteString("Respond with ONLY a Markdown document (no JSON, no code fences) that will be saved as the new ")
	b.WriteString(string(target))
	b.WriteString(" stage note. Start with a level-1 heading and use the section structure from the stage guidance above.\n")

	return b.String()
}

// stageGuidance explains what each ShapeUp stage should contain.
func stageGuidance(target Stage) string {
	switch target {
	case StageFrame:
		return `## Frame structure (required sections)
- # <title>
- ## Problem — what's wrong today, in concrete terms
- ## Affected users — who feels this pain
- ## Cost of not solving — business or user cost of the status quo
- ## Rough appetite — small-batch (1-2 weeks) or big-batch (6 weeks)
- ## Out of scope — what this explicitly will NOT address`
	case StageShape:
		return `## Shape structure (required sections)
- # <title>
- ## Problem — from the frame, re-stated crisply
- ## Appetite — small-batch or big-batch (pick one)
- ## Solution elements — the fat-marker sketch in prose
- ## Rabbit holes — risks we need to sidestep
- ## No-gos — explicitly rejected directions
- ## Sketches — fat-marker markdown (ASCII diagrams OK)`
	case StageBreadboard:
		return `## Breadboard structure (required sections)
- # <title>
- ## Places — the screens / states / surfaces involved
- ## Affordances — the actions a user can take at each place
- ## Connection lines — how places link to each other
- ## Key interactions — notes on the few interactions that matter most`
	case StagePitch:
		return `## Pitch structure (required sections)
- # <title>
- ## Problem — crisp 2-3 sentence framing
- ## Appetite — small-batch or big-batch
- ## Solution — the shaped approach, with any breadboard references
- ## Rabbit holes — known risks
- ## No-gos — explicit rejections
- ## Nice-to-haves — things we'd do if time allows`
	default:
		return "## Output\nFreeform markdown. Include a level-1 title."
	}
}

func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```markdown")
	s = strings.TrimPrefix(s, "```md")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// UnmarshalStructured attempts to parse a JSON-style response into a map.
// Kept for the usecase/prototype/prd packages to share, but returns nil
// when the response is plain markdown.
func UnmarshalStructured(raw string) map[string]interface{} {
	raw = stripCodeFences(raw)
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
