package prototypes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/vault"
)

// Synthesizer runs Claude to fill a chosen Format from source documents.
type Synthesizer struct {
	workingDir string
	model      string
	formats    *Registry
	persona    *persona.Loader
}

// NewSynthesizer wires a synthesizer with the default format registry.
// The persona loader is optional; pass nil to skip persona context injection.
func NewSynthesizer(workingDir, model string, p *persona.Loader) *Synthesizer {
	return &Synthesizer{workingDir: workingDir, model: model, formats: NewRegistry(), persona: p}
}

// Registry returns the format registry so handlers can list supported names.
func (s *Synthesizer) Registry() *Registry { return s.formats }

// Synthesize reads the requested sources, picks the format, builds a Claude
// prompt, and returns a ready-to-store Prototype.
func (s *Synthesizer) Synthesize(ctx context.Context, v *vault.Vault, req CreateRequest) (*Prototype, error) {
	format := s.formats.Pick(req.Format)

	// Concatenate source contents.
	var sources strings.Builder
	for _, path := range req.SourcePaths {
		body, err := v.ReadFile(path)
		if err != nil {
			sources.WriteString(fmt.Sprintf("\n\n[could not read %s: %s]\n\n", path, err))
			continue
		}
		sources.WriteString(fmt.Sprintf("\n\n### %s\n\n", path))
		if len(body) > 15000 {
			sources.WriteString(body[:15000])
			sources.WriteString("\n\n[...truncated]")
		} else {
			sources.WriteString(body)
		}
	}

	prompt := s.persona.BuildContextPrompt() + buildSynthesisPrompt(format, sources.String(), req.Hints)

	ag := &agent.Agent{
		Model:      s.model,
		MaxTurns:   1,
		WorkingDir: s.workingDir,
	}
	raw, err := ag.RunSimple(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("synthesize: %w", err)
	}

	filled := parseJSONResponse(raw)
	if filled == nil {
		return nil, fmt.Errorf("synthesize: could not parse Claude response; raw: %.300s", raw)
	}

	spec := format.Render(filled)

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
	}, nil
}

func buildSynthesisPrompt(format Format, sources, hints string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("You are a senior product designer. Turn the source documents below into a **%s** design spec.\n\n", format.Name()))
	b.WriteString(fmt.Sprintf("## Format: %s\n%s\n\n", format.Name(), format.Description()))

	if hints != "" {
		b.WriteString("## User hints\n")
		b.WriteString(hints)
		b.WriteString("\n\n")
	}

	b.WriteString("## Source documents\n")
	b.WriteString(sources)
	b.WriteString("\n\n")

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
