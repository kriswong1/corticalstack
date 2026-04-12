package pipeline

import (
	"strings"
	"testing"
	"time"
)

func TestParseExtractionResult(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSummary string
		wantTags    []string
		checkFn     func(*testing.T, *Extracted)
	}{
		{
			name:        "clean json",
			input:       `{"summary":"A brief summary","tags":["go","test"]}`,
			wantSummary: "A brief summary",
			wantTags:    []string{"go", "test"},
		},
		{
			name:        "json wrapped in markdown fences",
			input:       "```json\n{\"summary\":\"Wrapped\",\"tags\":[\"a\"]}\n```",
			wantSummary: "Wrapped",
			wantTags:    []string{"a"},
		},
		{
			name:        "json wrapped in plain fences",
			input:       "```\n{\"summary\":\"Plain fence\"}\n```",
			wantSummary: "Plain fence",
		},
		{
			name:  "malformed json falls back with error summary",
			input: `{not valid json`,
			checkFn: func(t *testing.T, got *Extracted) {
				if !strings.Contains(got.Summary, "Failed to parse extraction") {
					t.Errorf("expected parse failure summary, got %q", got.Summary)
				}
			},
		},
		{
			name:        "json with leading/trailing whitespace",
			input:       "   \n\n  {\"summary\":\"Padded\"}  \n\n",
			wantSummary: "Padded",
		},
		{
			name:  "json with all optional fields",
			input: `{"summary":"full","key_points":["a","b"],"decisions":["d1"],"ideas":["i1"],"learnings":["l1"],"domain":"engineering","intention":"learning"}`,
			checkFn: func(t *testing.T, got *Extracted) {
				if got.Summary != "full" {
					t.Errorf("summary = %q", got.Summary)
				}
				if len(got.KeyPoints) != 2 {
					t.Errorf("key_points len = %d", len(got.KeyPoints))
				}
				if got.Domain != "engineering" {
					t.Errorf("domain = %q", got.Domain)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExtractionResult(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSummary != "" && got.Summary != tt.wantSummary {
				t.Errorf("summary: got %q, want %q", got.Summary, tt.wantSummary)
			}
			if tt.wantTags != nil {
				if len(got.Tags) != len(tt.wantTags) {
					t.Errorf("tags len: got %d, want %d", len(got.Tags), len(tt.wantTags))
				}
			}
			if tt.checkFn != nil {
				tt.checkFn(t, got)
			}
		})
	}
}

func TestBuildExtractionPrompt(t *testing.T) {
	doc := &TextDocument{
		ID:      "doc-1",
		Source:  "webpage",
		Title:   "Sample Title",
		URL:     "https://example.com/article",
		Date:    time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC),
		Authors: []string{"Alice", "Bob"},
		Content: "Some document content here.",
	}

	tests := []struct {
		name     string
		cfg      ExtractionConfig
		wantHas  []string
	}{
		{
			name: "learning intention",
			cfg:  ExtractionConfig{Intention: "learning", Projects: []string{"proj-a"}, Why: "Interesting read"},
			wantHas: []string{
				"User's intention: learning",
				"Why the user saved this",
				"Interesting read",
				"proj-a",
				"Sample Title",
				"https://example.com/article",
				"2026-04-11",
				"Alice, Bob",
				"Some document content here",
				"how_this_applies",
				"open_questions",
			},
		},
		{
			name:    "information intention",
			cfg:     ExtractionConfig{Intention: "information"},
			wantHas: []string{"information", "facts", "claims", "definitions"},
		},
		{
			name:    "research intention",
			cfg:     ExtractionConfig{Intention: "research"},
			wantHas: []string{"research", "findings", "sources", "relevance", "next_steps"},
		},
		{
			name:    "project-application intention",
			cfg:     ExtractionConfig{Intention: "project-application"},
			wantHas: []string{"project-application", "impact", "integration_notes"},
		},
		{
			name:    "other intention",
			cfg:     ExtractionConfig{Intention: "other"},
			wantHas: []string{"proposed_structure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildExtractionPrompt(doc, tt.cfg)
			for _, sub := range tt.wantHas {
				if !strings.Contains(got, sub) {
					t.Errorf("prompt missing %q", sub)
				}
			}
		})
	}
}

func TestBuildExtractionPromptTruncates(t *testing.T) {
	doc := &TextDocument{
		Source:  "webpage",
		Content: strings.Repeat("x", 60000),
	}
	got := buildExtractionPrompt(doc, ExtractionConfig{})
	if !strings.Contains(got, "truncated at 50,000 characters") {
		t.Errorf("expected truncation notice in prompt")
	}
	if strings.Count(got, "x") > 50100 {
		t.Errorf("content should have been truncated but got %d x chars", strings.Count(got, "x"))
	}
}

func TestIntentionGuidance(t *testing.T) {
	intentions := []string{"learning", "information", "research", "project-application", "other"}
	seen := make(map[string]bool)
	for _, i := range intentions {
		got := intentionGuidance(i)
		if got == "" {
			t.Errorf("%s returned empty guidance", i)
		}
		if seen[got] {
			t.Errorf("%s returned same guidance as another intention", i)
		}
		seen[got] = true
	}
	// Unknown falls back to generic.
	generic := intentionGuidance("unknown")
	if generic == "" {
		t.Error("unknown intention returned empty")
	}
}

func TestIntentionFieldHints(t *testing.T) {
	intentions := []string{"learning", "information", "research", "project-application", "other"}
	for _, i := range intentions {
		if intentionFieldHints(i) == "" {
			t.Errorf("%s returned empty hints", i)
		}
	}
	// Default branch returns something (ideas).
	if intentionFieldHints("") == "" {
		t.Error("default branch empty")
	}
}
