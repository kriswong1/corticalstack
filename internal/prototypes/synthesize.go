package prototypes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/questions"
	"github.com/kriswong/corticalstack/internal/vault"
)

// Synthesizer runs Claude to fill a chosen Format from source documents.
type Synthesizer struct {
	workingDir string
	model      string
	formats    *Registry
	persona    *persona.Loader
	asker      *questions.Asker
}

// NewSynthesizer wires a synthesizer with the default format registry.
// The persona loader is optional; pass nil to skip persona context injection.
func NewSynthesizer(workingDir, model string, p *persona.Loader) *Synthesizer {
	return &Synthesizer{
		workingDir: workingDir,
		model:      model,
		formats:    NewRegistry(),
		persona:    p,
		asker:      questions.NewAsker(workingDir, model),
	}
}

// Questions asks Claude what a user should clarify before synthesis. It
// reads the requested source paths so Claude can base questions on the
// actual content, not just the title.
func (s *Synthesizer) Questions(ctx context.Context, v *vault.Vault, req QuestionsRequest) ([]questions.Question, error) {
	format := s.formats.Pick(req.Format)

	goal := fmt.Sprintf(
		"Synthesize a %q prototype titled %q. Ask clarifying questions that will materially change the generated output — layout preferences, target audience, specific scenarios to include, design tone, out-of-scope elements.",
		format.Name(), req.Title,
	)

	blocks := []questions.ContextBlock{
		{Heading: "Format description", Body: format.Description()},
	}
	if req.Hints != "" {
		blocks = append(blocks, questions.ContextBlock{Heading: "User hints", Body: req.Hints})
	}
	for _, path := range req.SourcePaths {
		body, err := v.ReadFile(path)
		if err != nil {
			continue
		}
		if len(body) > 4000 {
			body = body[:4000] + "\n\n[...truncated]"
		}
		blocks = append(blocks, questions.ContextBlock{
			Heading: fmt.Sprintf("Source: %s", path),
			Body:    body,
		})
	}

	return s.asker.Ask(ctx, goal, blocks)
}

// Registry returns the format registry so handlers can list supported names.
func (s *Synthesizer) Registry() *Registry { return s.formats }

// InvalidSourcePathsError is returned when none of the requested source_paths
// can be read from the vault. Callers (e.g. HTTP handlers) can type-assert on
// this to surface a 400 with an actionable message instead of a generic 500.
type InvalidSourcePathsError struct {
	Failures map[string]string // path -> read error
}

func (e *InvalidSourcePathsError) Error() string {
	var b strings.Builder
	b.WriteString("source_paths: no readable files in the vault. Paths must be relative to the vault root (e.g. 'product/pitch/foo.md'), not prefixed with 'vault/'. Failures:")
	for p, reason := range e.Failures {
		b.WriteString("\n  - ")
		b.WriteString(p)
		b.WriteString(": ")
		b.WriteString(reason)
	}
	return b.String()
}

// Synthesize reads the requested sources, picks the format, builds a Claude
// prompt, and returns a ready-to-store Prototype.
func (s *Synthesizer) Synthesize(ctx context.Context, v *vault.Vault, req CreateRequest) (*Prototype, error) {
	format := s.formats.Pick(req.Format)

	// Concatenate source contents. Track read failures so we can fail fast
	// with a clear 400 when *none* of the paths resolved — otherwise Claude
	// would be handed an empty prompt and return garbage we'd surface as 500.
	var sources strings.Builder
	failures := map[string]string{}
	sourcesRead := 0
	for _, path := range req.SourcePaths {
		body, err := v.ReadFile(path)
		if err != nil {
			failures[path] = err.Error()
			sources.WriteString(fmt.Sprintf("\n\n[could not read %s: %s]\n\n", path, err))
			continue
		}
		sourcesRead++
		sources.WriteString(fmt.Sprintf("\n\n### %s\n\n", path))
		if len(body) > 15000 {
			sources.WriteString(body[:15000])
			sources.WriteString("\n\n[...truncated]")
		} else {
			sources.WriteString(body)
		}
	}
	if sourcesRead == 0 {
		return nil, &InvalidSourcePathsError{Failures: failures}
	}

	answerBlock := questions.FormatAnswers(req.Questions, req.Answers)
	prompt := s.persona.BuildContextPrompt() + buildSynthesisPrompt(format, sources.String(), req.Hints, answerBlock)

	ag := &agent.Agent{
		Model:      s.model,
		MaxTurns:   1,
		WorkingDir: s.workingDir,
	}
	raw, err := ag.RunSimple(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("synthesize: %w", err)
	}

	var spec, htmlBody string
	if rawFmt, ok := format.(RawFormat); ok && rawFmt.IsRaw() {
		spec, htmlBody = rawFmt.RenderRaw(raw)
		if strings.TrimSpace(htmlBody) == "" && strings.TrimSpace(spec) == "" {
			return nil, fmt.Errorf("synthesize: empty response from Claude")
		}
	} else {
		filled := parseJSONResponse(raw)
		if filled == nil {
			return nil, fmt.Errorf("synthesize: could not parse Claude response; raw: %.300s", raw)
		}
		spec = format.Render(filled)
	}

	title := req.Title
	if title == "" {
		title = fmt.Sprintf("Prototype · %s", req.Format)
	}

	return &Prototype{
		Title:        title,
		Format:       req.Format,
		SourceRefs:   req.SourcePaths,
		SourceThread: req.SourceThread,
		Projects:     req.ProjectIDs,
		Spec:         spec,
		HTMLBody:     htmlBody,
		HasHTML:      strings.TrimSpace(htmlBody) != "",
	}, nil
}

func buildSynthesisPrompt(format Format, sources, hints, answerBlock string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("You are a senior product designer. Turn the source documents below into a **%s** design spec.\n\n", format.Name()))
	b.WriteString(fmt.Sprintf("## Format: %s\n%s\n\n", format.Name(), format.Description()))

	if answerBlock != "" {
		b.WriteString(answerBlock)
	}

	if hints != "" {
		b.WriteString("## User hints\n")
		b.WriteString(hints)
		b.WriteString("\n\n")
	}

	b.WriteString("## Source documents\n")
	b.WriteString(sources)
	b.WriteString("\n\n")

	// Raw formats (interactive-html) get a different output block — no JSON.
	if rawFmt, ok := format.(RawFormat); ok && rawFmt.IsRaw() {
		b.WriteString("## Output\n\n")
		b.WriteString(format.SchemaHint())
		b.WriteString("\n\nRespond with ONLY the raw output described above. No prose before or after, no code fences.\n")
		return b.String()
	}

	b.WriteString("## Output\n\nRespond with ONLY a JSON object (no fences, no prose) matching this schema:\n\n")
	b.WriteString(format.SchemaHint())
	b.WriteString("\n\n")
	b.WriteString("Fill every field with concrete, specific content grounded in the source documents. Do not invent features the sources don't mention. Respond with ONLY the JSON.\n")

	return b.String()
}

func parseJSONResponse(raw string) map[string]interface{} {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
