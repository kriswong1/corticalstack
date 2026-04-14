// Package pipeline implements the three-stage document processing pipeline:
//
//	Stage 1: TRANSFORM  — any raw input → TextDocument
//	Stage 2: EXTRACT    — TextDocument → structured artifacts via Claude CLI
//	Stage 3: ROUTE      — artifacts → vault destinations
package pipeline

import (
	"context"
	"time"

	"github.com/kriswong/corticalstack/internal/vault"
)

// --- Stage 1: Transform Types ---

// InputKind classifies how a raw input entered the system.
type InputKind string

const (
	InputText InputKind = "text"
	InputFile InputKind = "file"
	InputURL  InputKind = "url"
)

// RawInput is anything that enters the system before transformation.
type RawInput struct {
	Kind     InputKind         // text | file | url
	Path     string            // File path (if file-based)
	Content  []byte            // Raw bytes (if pasted/uploaded inline)
	URL      string            // Source URL (for web content)
	MIMEType string            // Detected or provided MIME type
	Filename string            // Original filename for uploads
	Title    string            // User-provided title (optional)
	Source   string            // Origin tag; filled by transformer if blank
	Metadata map[string]string // Source-specific context
}

// TextDocument is the common format after Stage 1.
type TextDocument struct {
	ID       string            // Unique identifier
	Source   string            // Transformer that produced this
	Title    string            // Inferred or provided title
	URL      string            // Original source URL (if applicable)
	Date     time.Time         // When content was created
	Authors  []string          // Speakers/participants
	Content  string            // Full text content
	Metadata map[string]string // Carried metadata
	// Projects is the typed list of project IDs this document is
	// associated with. Destinations should prefer this field over
	// reading Metadata["projects"] because the metadata map is
	// `map[string]string` and a CSV round-trip loses any project name
	// that contains a comma (LO-06). The CSV form remains populated
	// for backward compatibility with legacy destinations, but new
	// code should read Projects directly.
	Projects []string
}

// --- Stage 2: Extract Types ---

// ExtractionConfig controls what to pull out of a document.
type ExtractionConfig struct {
	ExtractSummary   bool
	ExtractDecisions bool
	ExtractActions   bool
	ExtractIdeas     bool
	ExtractLearnings bool

	// Intention is the confirmed intention (learning / information / research /
	// project-application / other). It adjusts the prompt so extraction fields
	// line up with the template that will render the note body.
	Intention string

	// Projects is the list of project IDs this document is associated with.
	Projects []string

	// Why is the optional free-text reason the user gave for saving this.
	Why string
}

// DefaultExtractionConfig returns a config that extracts everything.
func DefaultExtractionConfig() ExtractionConfig {
	return ExtractionConfig{
		ExtractSummary:   true,
		ExtractDecisions: true,
		ExtractActions:   true,
		ExtractIdeas:     true,
		ExtractLearnings: true,
	}
}

// LightExtractionConfig only pulls summary + actions.
func LightExtractionConfig() ExtractionConfig {
	return ExtractionConfig{
		ExtractSummary: true,
		ExtractActions: true,
	}
}

// ActionItem aliases vault.ActionItem so pipeline types stay self-contained.
type ActionItem = vault.ActionItem

// Extracted holds structured artifacts pulled from a document.
// Fields are optional — templates ignore anything they don't need.
type Extracted struct {
	Summary    string       `json:"summary,omitempty"`
	KeyPoints  []string     `json:"key_points,omitempty"`
	Decisions  []string     `json:"decisions,omitempty"`
	Actions    []ActionItem `json:"actions,omitempty"`
	Ideas      []string     `json:"ideas,omitempty"`
	Learnings  []string     `json:"learnings,omitempty"`
	Tags       []string     `json:"tags,omitempty"`
	Domain     string       `json:"domain,omitempty"`
	SourceURL  string       `json:"source_url,omitempty"`
	Triggers   []string     `json:"triggers,omitempty"`

	// Intention-specific fields. Each intention template reads a subset.
	Facts          []string `json:"facts,omitempty"`           // information
	Claims         []string `json:"claims,omitempty"`          // information
	Definitions    []string `json:"definitions,omitempty"`     // information
	Findings       []string `json:"findings,omitempty"`        // research
	Sources        []string `json:"sources,omitempty"`         // research
	Relevance      string   `json:"relevance,omitempty"`       // research / project-application
	NextSteps      []string `json:"next_steps,omitempty"`      // research / project-application
	Impact         string   `json:"impact,omitempty"`          // project-application
	Integration    string   `json:"integration_notes,omitempty"` // project-application
	OpenQuestions  []string `json:"open_questions,omitempty"`  // learning
	HowThisApplies string   `json:"how_this_applies,omitempty"` // learning
	ProposedStructure map[string][]string `json:"proposed_structure,omitempty"` // other

	// The intention used for this extraction (passed through from config).
	Intention string `json:"intention,omitempty"`
}

// --- Interfaces ---

// Transformer converts raw input of a specific modality to text.
type Transformer interface {
	Name() string
	CanHandle(input *RawInput) bool
	Transform(input *RawInput) (*TextDocument, error)
}

// Extractor pulls structured artifacts from a TextDocument.
type Extractor interface {
	Extract(ctx context.Context, doc *TextDocument, config ExtractionConfig) (*Extracted, error)
}

// Destination receives processed documents and extracted artifacts.
type Destination interface {
	Name() string
	Accept(doc *TextDocument, extracted *Extracted) (string, error)
}
