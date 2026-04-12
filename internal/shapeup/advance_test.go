package shapeup

import (
	"strings"
	"testing"
	"time"
)

func TestStageGuidance(t *testing.T) {
	stages := []Stage{StageFrame, StageShape, StageBreadboard, StagePitch}
	seen := make(map[string]bool)
	for _, s := range stages {
		got := stageGuidance(s)
		if got == "" {
			t.Errorf("stageGuidance(%q) returned empty", s)
		}
		if seen[got] {
			t.Errorf("stageGuidance(%q) returned same content as another stage", s)
		}
		seen[got] = true
	}
	// Unknown stage falls back to freeform.
	if got := stageGuidance("unknown"); !strings.Contains(got, "Freeform markdown") {
		t.Errorf("unknown stage should fall back, got %q", got)
	}
	if got := stageGuidance(StageRaw); !strings.Contains(got, "Freeform markdown") {
		t.Errorf("raw stage should use freeform, got %q", got)
	}
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"```markdown\nhello\n```", "hello"},
		{"```md\nhello\n```", "hello"},
		{"```\nhello\n```", "hello"},
		{"no fences here", "no fences here"},
		{"  \n\n  padded  \n\n  ", "padded"},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripCodeFences(tt.in)
		if got != tt.want {
			t.Errorf("stripCodeFences(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUnmarshalStructured(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		got := UnmarshalStructured(`{"key":"value","count":42}`)
		if got == nil {
			t.Fatal("expected map, got nil")
		}
		if got["key"] != "value" {
			t.Errorf("key = %v", got["key"])
		}
	})

	t.Run("plain fenced json", func(t *testing.T) {
		got := UnmarshalStructured("```\n{\"a\":1}\n```")
		if got == nil {
			t.Fatal("expected map, got nil")
		}
	})

	t.Run("plain markdown returns nil", func(t *testing.T) {
		got := UnmarshalStructured("# Heading\n\nJust markdown.")
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("malformed json returns nil", func(t *testing.T) {
		got := UnmarshalStructured(`{not valid`)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestBuildAdvancePrompt(t *testing.T) {
	thread := &Thread{
		ID:       "thread-1",
		Title:    "Interesting Idea",
		Projects: []string{"proj-a"},
		Artifacts: []*Artifact{
			{
				Stage:   StageRaw,
				Title:   "Raw Idea",
				Body:    "Initial thoughts about the idea",
				Created: time.Now(),
			},
			{
				Stage:   StageFrame,
				Title:   "Framed",
				Body:    "After framing the problem",
				Created: time.Now(),
			},
		},
	}

	got := buildAdvancePrompt(thread, StageShape, "focus on UX")

	wantHas := []string{
		"Ryan Singer's Shape Up",
		"shape", // target stage
		"focus on UX",
		"proj-a",
		"Raw Idea",
		"Initial thoughts",
		"Framed",
		"After framing",
		"Shape structure",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("prompt missing %q", sub)
		}
	}
}

func TestBuildAdvancePromptTruncatesLongBody(t *testing.T) {
	thread := &Thread{
		Artifacts: []*Artifact{
			{Stage: StageRaw, Title: "Huge", Body: strings.Repeat("x", 9000)},
		},
	}
	got := buildAdvancePrompt(thread, StageFrame, "")
	if !strings.Contains(got, "[...truncated]") {
		t.Errorf("expected truncation marker for >8000 char body")
	}
}

func TestBuildAdvancePromptEmptyHints(t *testing.T) {
	thread := &Thread{
		Artifacts: []*Artifact{{Stage: StageRaw, Body: "x"}},
	}
	got := buildAdvancePrompt(thread, StageFrame, "")
	if strings.Contains(got, "Hints from the user") {
		t.Errorf("empty hints should not produce hints section")
	}
}
