package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/projects"
)

// ClaudeClassifier runs a single Claude CLI call to classify a document.
type ClaudeClassifier struct {
	workingDir string
	model      string
	persona    persona.ContextBuilder
}

// NewClaudeClassifier creates a classifier bound to a working directory.
// The persona loader is optional; nil is substituted with
// persona.NoopContextBuilder so call sites never dereference a nil.
func NewClaudeClassifier(workingDir, model string, p *persona.Loader) *ClaudeClassifier {
	return &ClaudeClassifier{workingDir: workingDir, model: model, persona: persona.ResolveContextBuilder(p)}
}

// Classify sends the document to Claude and returns a parsed preview.
// activeProjects lets Claude suggest associations when content mentions them.
func (c *ClaudeClassifier) Classify(ctx context.Context, doc *pipeline.TextDocument, activeProjects []*projects.Project) (*PreviewResult, error) {
	prompt := c.persona.BuildContextPrompt() + buildClassifyPrompt(doc, activeProjects)

	ag := &agent.Agent{
		Model:      c.model,
		MaxTurns:   10,
		WorkingDir: c.workingDir,
	}
	raw, err := ag.RunSimple(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("classifier claude call: %w", err)
	}
	return parsePreviewResult(raw)
}

func buildClassifyPrompt(doc *pipeline.TextDocument, activeProjects []*projects.Project) string {
	var b strings.Builder

	b.WriteString("You are an intent classifier for a personal knowledge system. Given a piece of content, decide *why* the user probably saved it, suggest a short title, pick up to 3 matching active projects, and write a one-paragraph gist.\n\n")

	b.WriteString("## Supported intentions\n\n")
	b.WriteString("- `learning` — something the user wants to absorb or understand\n")
	b.WriteString("- `information` — facts the user wants to reference later (docs, specs, prices, definitions)\n")
	b.WriteString("- `research` — info gathered in service of a project with a clear provenance trail\n")
	b.WriteString("- `project-application` — directly useful for an active project (feature ideas, bug details, integration notes)\n")
	b.WriteString("- `other` — none of the above; a Claude-proposed structure will be used\n\n")

	if len(activeProjects) > 0 {
		b.WriteString("## Active projects (id — description)\n\n")
		for _, p := range activeProjects {
			if p.Status == projects.StatusArchived {
				continue
			}
			b.WriteString(fmt.Sprintf("- `%s` — %s\n", p.Slug, firstNonEmpty(p.Description, p.Name)))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Document context\n")
	b.WriteString(fmt.Sprintf("- Source: %s\n", doc.Source))
	if doc.Title != "" {
		b.WriteString(fmt.Sprintf("- Title: %s\n", doc.Title))
	}
	if doc.URL != "" {
		b.WriteString(fmt.Sprintf("- URL: %s\n", doc.URL))
	}
	if len(doc.Authors) > 0 {
		b.WriteString(fmt.Sprintf("- Authors: %s\n", strings.Join(doc.Authors, ", ")))
	}
	maxChars := config.MaxClassifierChars()
	b.WriteString(fmt.Sprintf("\n## Document content (first %d chars)\n\n", maxChars))

	content := doc.Content
	if len(content) > maxChars {
		content = content[:maxChars] + "\n\n[...truncated]"
	}
	b.WriteString(content)
	b.WriteString("\n\n")

	b.WriteString("## Instructions\n\n")
	b.WriteString("Respond with ONLY a JSON object (no markdown fences, no explanation) containing:\n\n")
	b.WriteString("```\n{\n")
	b.WriteString(`  "intention": "learning|information|research|project-application|other",` + "\n")
	b.WriteString(`  "confidence": 0.0-1.0,` + "\n")
	b.WriteString(`  "summary": "one paragraph (2-4 sentences) describing what this is",` + "\n")
	b.WriteString(`  "suggested_title": "short human title (under 80 chars)",` + "\n")
	b.WriteString(`  "suggested_project_ids": ["existing-project-slug"],` + "\n")
	b.WriteString(`  "proposed_project_name": "New Project Name (only if no active project fits)",` + "\n")
	b.WriteString(`  "suggested_tags": ["topic", "tags"],` + "\n")
	b.WriteString(`  "reasoning": "one sentence on why this intention fits"` + "\n")
	b.WriteString("}\n```\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- suggested_project_ids MUST come from the active projects list above. Empty array if none fit.\n")
	b.WriteString("- DO NOT invent new project IDs. If the content clearly belongs to a *new* project that's not in the list, leave suggested_project_ids empty and put the proposed name in `proposed_project_name`. The user will confirm whether to create it.\n")
	b.WriteString("- Prefer `project-application` over `research` when the content is directly actionable for a listed project.\n")
	b.WriteString("- Use `information` for neutral reference material with no obvious action.\n")
	b.WriteString("- Respond with ONLY the JSON. No other text.\n")

	return b.String()
}

func parsePreviewResult(raw string) (*PreviewResult, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var result PreviewResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		// Fall back to a safe default so callers can still show something.
		return &PreviewResult{
			Intention:  Information,
			Confidence: 0.0,
			Summary:    fmt.Sprintf("Classifier failed to parse response. Defaulted to 'information'. Raw: %.200s", raw),
			Reasoning:  "classifier parse error: " + err.Error(),
		}, nil
	}
	if !IsValid(string(result.Intention)) {
		result.Intention = Other
	}
	return &result, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
