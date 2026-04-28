package intent

import (
	"strings"
	"testing"

	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/projects"
)

func TestParsePreviewResult(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantIntention Intention
		wantConf      float64
		checkFn       func(*testing.T, *PreviewResult)
	}{
		{
			name: "clean json with all fields",
			input: `{
				"intention": "learning",
				"confidence": 0.85,
				"summary": "A learning resource",
				"suggested_title": "Learn Go",
				"suggested_project_ids": ["proj-a"],
				"suggested_tags": ["go", "programming"],
				"reasoning": "covers Go concepts"
			}`,
			wantIntention: Learning,
			wantConf:      0.85,
		},
		{
			name:          "json with markdown fence",
			input:         "```json\n{\"intention\":\"information\",\"confidence\":0.9,\"summary\":\"Facts\"}\n```",
			wantIntention: Information,
			wantConf:      0.9,
		},
		{
			name:          "json with plain fence",
			input:         "```\n{\"intention\":\"research\",\"confidence\":0.7,\"summary\":\"Research\"}\n```",
			wantIntention: Research,
			wantConf:      0.7,
		},
		{
			name:  "malformed json falls back to information + error summary",
			input: `{not json`,
			checkFn: func(t *testing.T, got *PreviewResult) {
				if got.Intention != Information {
					t.Errorf("expected fallback to Information, got %q", got.Intention)
				}
				if !strings.Contains(got.Summary, "Classifier failed to parse") {
					t.Errorf("expected fallback summary, got %q", got.Summary)
				}
			},
		},
		{
			name:  "invalid intention maps to Other",
			input: `{"intention":"bogus","confidence":0.5,"summary":"x"}`,
			checkFn: func(t *testing.T, got *PreviewResult) {
				if got.Intention != Other {
					t.Errorf("expected fallback to Other, got %q", got.Intention)
				}
			},
		},
		{
			name:          "project-application intention",
			input:         `{"intention":"project-application","confidence":0.95,"summary":"Actionable"}`,
			wantIntention: ProjectApplication,
			wantConf:      0.95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePreviewResult(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantIntention != "" && got.Intention != tt.wantIntention {
				t.Errorf("intention: got %q, want %q", got.Intention, tt.wantIntention)
			}
			if tt.wantConf != 0 && got.Confidence != tt.wantConf {
				t.Errorf("confidence: got %v, want %v", got.Confidence, tt.wantConf)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, got)
			}
		})
	}
}

func TestBuildClassifyPrompt(t *testing.T) {
	doc := &pipeline.TextDocument{
		Source:  "webpage",
		Title:   "The Go Programming Language",
		URL:     "https://go.dev",
		Authors: []string{"Rob Pike"},
		Content: "Go is a language...",
	}
	activeProjects := []*projects.Project{
		{UUID: "uuid-a", Slug: "proj-a", Name: "Project A", Description: "First project", Status: projects.StatusActive},
		{UUID: "uuid-b", Slug: "proj-b", Name: "Project B", Status: projects.StatusArchived}, // should be filtered out
	}

	got := buildClassifyPrompt(doc, activeProjects)

	wantHas := []string{
		"intent classifier",
		"learning",
		"information",
		"research",
		"project-application",
		"other",
		"proj-a",
		"First project",
		"The Go Programming Language",
		"https://go.dev",
		"Rob Pike",
		"Go is a language",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("prompt missing %q", sub)
		}
	}

	// Archived project should not appear.
	if strings.Contains(got, "proj-b") {
		t.Errorf("prompt should not contain archived project proj-b")
	}
}

func TestBuildClassifyPromptTruncates(t *testing.T) {
	doc := &pipeline.TextDocument{
		Source:  "webpage",
		Content: strings.Repeat("x", 10000),
	}
	got := buildClassifyPrompt(doc, nil)
	if !strings.Contains(got, "[...truncated]") {
		t.Errorf("expected truncation marker in prompt")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"first wins", []string{"a", "b", "c"}, "a"},
		{"skips empty", []string{"", "b"}, "b"},
		{"skips whitespace-only", []string{"   ", "real"}, "real"},
		{"all empty", []string{"", "   ", ""}, ""},
		{"no args", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstNonEmpty(tt.values...)
			if got != tt.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

func TestIsValid(t *testing.T) {
	valid := []string{"learning", "information", "research", "project-application", "other"}
	for _, v := range valid {
		if !IsValid(v) {
			t.Errorf("IsValid(%q) = false, want true", v)
		}
	}
	invalid := []string{"", "LEARNING", "bogus", "project_application", "learn"}
	for _, v := range invalid {
		if IsValid(v) {
			t.Errorf("IsValid(%q) = true, want false", v)
		}
	}
}

func TestAll(t *testing.T) {
	got := All()
	if len(got) != 5 {
		t.Errorf("All() returned %d intentions, want 5", len(got))
	}
	wantOrder := []Intention{Learning, Information, Research, ProjectApplication, Other}
	for i, want := range wantOrder {
		if got[i] != want {
			t.Errorf("All()[%d] = %q, want %q", i, got[i], want)
		}
	}
}
