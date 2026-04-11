package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/vault"
)

// Generator runs Claude to extract structured UseCases from either a source
// document in the vault or free-form user input.
type Generator struct {
	workingDir string
	model      string
	persona    *persona.Loader
}

// NewGenerator creates a generator bound to a working directory.
// The persona loader is optional; pass nil to skip persona context injection.
func NewGenerator(workingDir, model string, p *persona.Loader) *Generator {
	return &Generator{workingDir: workingDir, model: model, persona: p}
}

// FromDoc reads a document out of the vault and asks Claude to extract a
// list of UseCases. One document may produce multiple scenarios.
func (g *Generator) FromDoc(ctx context.Context, v *vault.Vault, req FromDocRequest) ([]*UseCase, error) {
	body, err := v.ReadFile(req.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("reading source: %w", err)
	}
	prompt := g.persona.BuildContextPrompt() + buildFromDocPrompt(req.SourcePath, body, req.Hint)
	raw, err := g.runClaude(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return parseUseCases(raw, req.ProjectIDs, []SourceRef{{Type: "doc", Path: req.SourcePath}})
}

// FromText generates one or more UseCases from a free-form description.
func (g *Generator) FromText(ctx context.Context, req FromTextRequest) ([]*UseCase, error) {
	prompt := g.persona.BuildContextPrompt() + buildFromTextPrompt(req.Description, req.ActorsHint)
	raw, err := g.runClaude(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return parseUseCases(raw, req.ProjectIDs, []SourceRef{{Type: "freeform"}})
}

func (g *Generator) runClaude(ctx context.Context, prompt string) (string, error) {
	ag := &agent.Agent{
		Model:      g.model,
		MaxTurns:   1,
		WorkingDir: g.workingDir,
	}
	return ag.RunSimple(ctx, prompt)
}

func buildFromDocPrompt(sourcePath, body, hint string) string {
	var b strings.Builder
	b.WriteString("You are a product analyst. Read the source document and extract one or more standardized UseCases.\n\n")
	if hint != "" {
		b.WriteString(fmt.Sprintf("## User hint\n%s\n\n", hint))
	}
	b.WriteString(fmt.Sprintf("## Source document (`%s`)\n\n", sourcePath))
	if len(body) > 20000 {
		body = body[:20000] + "\n\n[...truncated]"
	}
	b.WriteString(body)
	b.WriteString("\n\n")
	b.WriteString(commonUseCasePromptTail())
	return b.String()
}

func buildFromTextPrompt(description, actorsHint string) string {
	var b strings.Builder
	b.WriteString("You are a product analyst. A user has described a scenario in free text; turn it into one or more standardized UseCases.\n\n")
	if actorsHint != "" {
		b.WriteString(fmt.Sprintf("## Actors hint\n%s\n\n", actorsHint))
	}
	b.WriteString("## User description\n\n")
	b.WriteString(description)
	b.WriteString("\n\n")
	b.WriteString(commonUseCasePromptTail())
	return b.String()
}

// commonUseCasePromptTail describes the output schema Claude must match.
func commonUseCasePromptTail() string {
	return `## Output format

Respond with ONLY a JSON array (no markdown fences, no prose) of UseCase objects. Each object:

` + "```" + `
{
  "title": "Short title in active voice",
  "actors": ["Primary actor role"],
  "secondary_actors": ["Optional secondary actors"],
  "preconditions": ["Required state before the flow starts"],
  "main_flow": ["Step 1", "Step 2", "Step 3"],
  "alternative_flows": [
    {"name": "Scenario name", "at_step": 2, "flow": ["Step A", "Step B"]}
  ],
  "postconditions": ["Required state after the flow completes"],
  "business_rules": ["Rule the system enforces"],
  "non_functional": ["Performance/availability/security constraints"],
  "tags": ["topic", "tags"]
}
` + "```" + `

Rules:
- Output a JSON array, even if there's only one UseCase.
- Identify DISTINCT scenarios. Don't cram unrelated flows into one UseCase.
- Be specific in the main_flow steps (actor + verb + object + system response).
- Omit empty arrays.
- Respond with ONLY the JSON array.
`
}

// parseUseCases turns the Claude response into filled UseCase structs.
func parseUseCases(raw string, projectIDs []string, sources []SourceRef) ([]*UseCase, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var parsed []*UseCase
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		// Try as a single object
		var single UseCase
		if err2 := json.Unmarshal([]byte(raw), &single); err2 == nil {
			parsed = []*UseCase{&single}
		} else {
			return nil, fmt.Errorf("parsing use cases: %w; raw: %.300s", err, raw)
		}
	}
	for _, u := range parsed {
		u.Projects = projectIDs
		u.Sources = sources
	}
	return parsed, nil
}
