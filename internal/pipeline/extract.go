package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/persona"
)

// ClaudeExtractor uses the Claude CLI (Paperclip pattern) to pull
// structured artifacts from any TextDocument. Zero cost on Claude Max.
type ClaudeExtractor struct {
	workingDir string
	model      string
	persona    *persona.Loader
}

// NewClaudeExtractor creates an extractor that shells out to `claude --print`.
// The persona loader is optional; pass nil to skip persona context injection.
func NewClaudeExtractor(workingDir, model string, p *persona.Loader) *ClaudeExtractor {
	return &ClaudeExtractor{workingDir: workingDir, model: model, persona: p}
}

// Extract calls Claude to analyze a document and return structured data.
// The prompt adapts based on cfg.Intention so fields line up with the
// intention-specific template that will render the body.
func (e *ClaudeExtractor) Extract(doc *TextDocument, cfg ExtractionConfig) (*Extracted, error) {
	prompt := e.persona.BuildContextPrompt() + buildExtractionPrompt(doc, cfg)

	ag := &agent.Agent{
		Model:      e.model,
		MaxTurns:   1,
		WorkingDir: e.workingDir,
	}

	ctx := context.Background()
	raw, err := ag.RunSimple(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("extraction via claude cli: %w", err)
	}
	return parseExtractionResult(raw)
}

func buildExtractionPrompt(doc *TextDocument, cfg ExtractionConfig) string {
	var b strings.Builder

	b.WriteString("You are a document analysis system. Extract structured data from the following document.\n\n")

	if cfg.Intention != "" {
		b.WriteString(fmt.Sprintf("## User's intention: %s\n\n", cfg.Intention))
		b.WriteString(intentionGuidance(cfg.Intention))
		b.WriteString("\n")
	}
	if cfg.Why != "" {
		b.WriteString(fmt.Sprintf("## Why the user saved this\n%s\n\n", cfg.Why))
	}
	if len(cfg.Projects) > 0 {
		b.WriteString(fmt.Sprintf("## Associated projects\n- %s\n\n", strings.Join(cfg.Projects, "\n- ")))
	}

	b.WriteString("## Document Context\n")
	b.WriteString(fmt.Sprintf("- Source: %s\n", doc.Source))
	if doc.Title != "" {
		b.WriteString(fmt.Sprintf("- Title: %s\n", doc.Title))
	}
	if doc.URL != "" {
		b.WriteString(fmt.Sprintf("- URL: %s\n", doc.URL))
	}
	if !doc.Date.IsZero() {
		b.WriteString(fmt.Sprintf("- Date: %s\n", doc.Date.Format("2006-01-02")))
	}
	if len(doc.Authors) > 0 {
		b.WriteString(fmt.Sprintf("- Participants: %s\n", strings.Join(doc.Authors, ", ")))
	}
	b.WriteString("\n")

	b.WriteString("## Document Content\n\n")
	content := doc.Content
	if len(content) > 50000 {
		content = content[:50000] + "\n\n[...content truncated at 50,000 characters]"
	}
	b.WriteString(content)
	b.WriteString("\n\n")

	b.WriteString("## Output format\n\n")
	b.WriteString("Respond with ONLY a JSON object (no markdown fences, no prose) containing these fields:\n\n")
	b.WriteString("```\n{\n")
	b.WriteString(`  "summary": "2-4 sentence prose summary",` + "\n")
	b.WriteString(`  "key_points": ["point 1", "point 2"],` + "\n")
	b.WriteString(`  "actions": [{"owner": "Name", "description": "task", "deadline": "optional"}],` + "\n")
	b.WriteString(`  "tags": ["topic", "tags"],` + "\n")
	b.WriteString(`  "domain": "engineering | product | design | operations | finance | ...",` + "\n")
	b.WriteString(`  "triggers": ["when to surface this"],` + "\n")
	b.WriteString(intentionFieldHints(cfg.Intention))
	b.WriteString("}\n```\n\n")

	if doc.URL != "" {
		b.WriteString(fmt.Sprintf("Source URL: %s — include as \"source_url\".\n\n", doc.URL))
	}

	b.WriteString("Rules:\n")
	b.WriteString("- Only extract what's in the document. Never invent.\n")
	b.WriteString("- For actions, include an owner ('TBD' if unclear).\n")
	b.WriteString("- Omit empty arrays and empty strings. Only include fields you actually populate.\n")
	b.WriteString("- Generate 2-5 triggers — specific scenarios where this knowledge should surface.\n")
	b.WriteString("- Respond with ONLY the JSON.\n")

	return b.String()
}

// intentionGuidance returns prose describing what the extractor should
// emphasize for each intention.
func intentionGuidance(intention string) string {
	switch intention {
	case "learning":
		return `The user wants to learn from this content. Focus on: key ideas, how they apply to the user's projects or situation, and open questions worth exploring. Populate key_points, how_this_applies, open_questions.`
	case "information":
		return `The user wants this as structured reference material. Focus on: verifiable facts, claims with context, and definitions. Populate facts, claims, definitions.`
	case "research":
		return `The user is researching this in service of a project. Focus on: findings, sources/citations, relevance to the associated projects, and next research steps. Populate findings, sources, relevance, next_steps.`
	case "project-application":
		return `The user sees this as directly useful for an active project. Focus on: impact on the project, action items, integration notes, and next steps. Populate impact, actions, integration_notes, next_steps.`
	case "other":
		return `No pre-defined intention. Propose a structure under "proposed_structure" — a map of heading → bullet strings — that best represents this content. Always include a summary and key_points.`
	default:
		return `Extract a summary, key points, and any obvious action items.`
	}
}

// intentionFieldHints adds intention-specific JSON field examples to the prompt.
func intentionFieldHints(intention string) string {
	switch intention {
	case "learning":
		return `  "how_this_applies": "2-3 sentences on how this helps the user's projects",` + "\n" +
			`  "open_questions": ["question 1", "question 2"]` + "\n"
	case "information":
		return `  "facts": ["fact 1", "fact 2"],` + "\n" +
			`  "claims": ["claim 1"],` + "\n" +
			`  "definitions": ["term: meaning"]` + "\n"
	case "research":
		return `  "findings": ["finding 1"],` + "\n" +
			`  "sources": ["source 1"],` + "\n" +
			`  "relevance": "2-3 sentences on relevance to the projects",` + "\n" +
			`  "next_steps": ["next step 1"]` + "\n"
	case "project-application":
		return `  "impact": "how this affects the project",` + "\n" +
			`  "integration_notes": "how to integrate this",` + "\n" +
			`  "next_steps": ["next step 1"]` + "\n"
	case "other":
		return `  "proposed_structure": {"Heading A": ["bullet 1"], "Heading B": ["bullet 1"]}` + "\n"
	default:
		return `  "ideas": ["idea 1"]` + "\n"
	}
}

func parseExtractionResult(result string) (*Extracted, error) {
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var extracted Extracted
	if err := json.Unmarshal([]byte(result), &extracted); err != nil {
		return &Extracted{
			Summary: fmt.Sprintf("Failed to parse extraction: %s\nRaw: %.200s", err, result),
		}, nil
	}
	return &extracted, nil
}
