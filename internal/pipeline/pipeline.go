package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/vault"
)

// Pipeline orchestrates the three-stage document processing flow.
// It supports both a one-shot Process() and a two-phase Transform → Confirm flow.
type Pipeline struct {
	transformers []Transformer
	extractor    Extractor
	destinations []Destination
}

// ProcessResult tracks what happened during pipeline execution.
type ProcessResult struct {
	Document    *TextDocument
	Extracted   *Extracted
	Outputs     map[string]string // destination name → created path/ID
	Errors      []string
	Transformer string
}

// BuildFn is a constructor that returns the list of transformers for a
// pipeline. Accepting this lets main.go wire integrations without an
// import cycle between pipeline and transformers.
type BuildFn func(deepgram *integrations.DeepgramClient) []Transformer

// New creates a fully wired pipeline.
func New(
	v *vault.Vault,
	workingDir, claudeModel string,
	deepgram *integrations.DeepgramClient,
	buildTransformers BuildFn,
	actionStore *actions.Store,
	personaLoader *persona.Loader,
) *Pipeline {
	return &Pipeline{
		transformers: buildTransformers(deepgram),
		extractor:    NewClaudeExtractor(workingDir, claudeModel, personaLoader),
		destinations: []Destination{
			NewVaultNoteDestination(v),
			NewActionItemsDestination(v, actionStore),
			NewDailyLogDestination(v),
		},
	}
}

// Transform runs only Stage 1 and returns the TextDocument plus the
// transformer name that handled it. Callers use this when they want to
// run the classifier before committing to extract + route.
func (p *Pipeline) Transform(input *RawInput) (*TextDocument, string, error) {
	transformer := FindTransformer(p.transformers, input)
	if transformer == nil {
		return nil, "", fmt.Errorf("no transformer can handle this input (path=%q url=%q kind=%q)", input.Path, input.URL, input.Kind)
	}
	doc, err := transformer.Transform(input)
	if err != nil {
		return nil, transformer.Name(), fmt.Errorf("transform (%s) failed: %w", transformer.Name(), err)
	}
	if doc.Content == "" {
		return nil, transformer.Name(), fmt.Errorf("transformer %s produced empty content", transformer.Name())
	}
	return doc, transformer.Name(), nil
}

// ExtractAndRoute runs stages 2 and 3 on an already-transformed document.
// The intention/projects/why fields are embedded in the ExtractionConfig so
// the prompt and template both see them.
func (p *Pipeline) ExtractAndRoute(ctx context.Context, doc *TextDocument, cfg ExtractionConfig, transformerName string) *ProcessResult {
	result := &ProcessResult{
		Outputs:     make(map[string]string),
		Document:    doc,
		Transformer: transformerName,
	}

	extracted, err := p.extractor.Extract(ctx, doc, cfg)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("extraction: %v", err))
		extracted = &Extracted{}
	}
	// Carry intention + metadata into Extracted so destinations can read it.
	extracted.Intention = cfg.Intention
	// Stash projects + why in doc.Metadata for the destinations to consume.
	if doc.Metadata == nil {
		doc.Metadata = map[string]string{}
	}
	if len(cfg.Projects) > 0 {
		doc.Metadata["projects"] = strings.Join(cfg.Projects, ",")
	}
	if cfg.Why != "" {
		doc.Metadata["why"] = cfg.Why
	}
	// Pre-generate stable IDs for every action so templates embed the same
	// ID that the action store will later upsert.
	for i := range extracted.Actions {
		if extracted.Actions[i].ID == "" {
			extracted.Actions[i].ID = uuid.NewString()
		}
	}
	result.Extracted = extracted

	for _, dest := range p.destinations {
		output, err := dest.Accept(doc, extracted)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", dest.Name(), err))
			continue
		}
		if output != "" {
			result.Outputs[dest.Name()] = output
		}
	}
	return result
}

// Process runs all three stages in one shot using default config for the
// document's source. Kept for any non-interactive callers.
func (p *Pipeline) Process(ctx context.Context, input *RawInput) (*ProcessResult, error) {
	doc, transformerName, err := p.Transform(input)
	if err != nil {
		return nil, err
	}
	cfg := extractionConfigForSource(doc.Source)
	return p.ExtractAndRoute(ctx, doc, cfg, transformerName), nil
}

// ProcessText is a convenience for pasted text input.
func (p *Pipeline) ProcessText(ctx context.Context, text, title string) (*ProcessResult, error) {
	return p.Process(ctx, &RawInput{
		Kind:    InputText,
		Content: []byte(text),
		Title:   title,
	})
}

// ProcessFile is a convenience for a file path.
func (p *Pipeline) ProcessFile(ctx context.Context, path string) (*ProcessResult, error) {
	return p.Process(ctx, &RawInput{Kind: InputFile, Path: path})
}

// ProcessUpload is a convenience for an in-memory upload.
func (p *Pipeline) ProcessUpload(ctx context.Context, filename string, content []byte) (*ProcessResult, error) {
	return p.Process(ctx, &RawInput{Kind: InputFile, Filename: filename, Content: content})
}

// ProcessURL is a convenience for a URL input.
func (p *Pipeline) ProcessURL(ctx context.Context, url string) (*ProcessResult, error) {
	return p.Process(ctx, &RawInput{Kind: InputURL, URL: url})
}

// ListTransformers returns the names of available transformers.
func (p *Pipeline) ListTransformers() []string {
	var names []string
	for _, t := range p.transformers {
		names = append(names, t.Name())
	}
	return names
}

// ListDestinations returns the names of wired destinations.
func (p *Pipeline) ListDestinations() []string {
	var names []string
	for _, d := range p.destinations {
		names = append(names, d.Name())
	}
	return names
}

// EnsureFolders creates the standard vault folder layout.
func (p *Pipeline) EnsureFolders(v *vault.Vault) {
	EnsureVaultFolders(v)
}

// FormatResult returns a human-readable summary of the pipeline output.
func FormatResult(r *ProcessResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Pipeline processed: %s\n", r.Document.Title))
	b.WriteString(fmt.Sprintf("  Transformer: %s\n", r.Transformer))
	b.WriteString(fmt.Sprintf("  Content: %d chars\n", len(r.Document.Content)))
	if len(r.Outputs) > 0 {
		b.WriteString("  Outputs:\n")
		for dest, path := range r.Outputs {
			b.WriteString(fmt.Sprintf("    %s → %s\n", dest, path))
		}
	}
	if len(r.Errors) > 0 {
		b.WriteString("  Errors:\n")
		for _, e := range r.Errors {
			b.WriteString(fmt.Sprintf("    %s\n", e))
		}
	}
	return b.String()
}

// extractionConfigForSource picks the right extraction config per source.
func extractionConfigForSource(source string) ExtractionConfig {
	switch source {
	case "deepgram", "audio", "pdf", "docx", "youtube", "webpage", "linkedin", "html":
		return DefaultExtractionConfig()
	default:
		return LightExtractionConfig()
	}
}

// DefaultConfigForSource is the exported helper handlers use when building
// a config from an already-transformed document.
func DefaultConfigForSource(source string) ExtractionConfig {
	return extractionConfigForSource(source)
}
