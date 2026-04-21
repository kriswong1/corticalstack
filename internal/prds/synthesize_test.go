package prds

import (
	"strings"
	"testing"
)

func TestParseSynthesis(t *testing.T) {
	t.Run("valid full payload", func(t *testing.T) {
		raw := `{
			"title": "Sample PRD",
			"problem": "Users can't find things",
			"goals": ["Improve search", "Reduce time-to-result"],
			"functional_requirements": ["FR1", "FR2"],
			"open_questions": ["How to rank results?"]
		}`
		got, err := parseSynthesis(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "Sample PRD" {
			t.Errorf("title = %q", got.Title)
		}
		if len(got.Goals) != 2 {
			t.Errorf("goals len = %d", len(got.Goals))
		}
		if len(got.OpenQuestions) != 1 {
			t.Errorf("open_questions len = %d", len(got.OpenQuestions))
		}
	})

	t.Run("markdown fenced", func(t *testing.T) {
		raw := "```json\n{\"title\":\"X\",\"problem\":\"p\",\"goals\":[\"g\"],\"functional_requirements\":[\"fr\"]}\n```"
		got, err := parseSynthesis(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "X" {
			t.Errorf("title = %q", got.Title)
		}
	})

	t.Run("missing title errors", func(t *testing.T) {
		raw := `{"problem":"no title"}`
		_, err := parseSynthesis(raw)
		if err == nil {
			t.Fatal("expected error for missing title")
		}
	})

	t.Run("malformed json errors", func(t *testing.T) {
		_, err := parseSynthesis("{not json")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRenderPRDBody(t *testing.T) {
	s := &Synthesis{
		Title:                     "Sample PRD",
		Problem:                   "The problem framing",
		Goals:                     []string{"Goal one", "Goal two"},
		NonGoals:                  []string{"Don't rewrite the world"},
		UserStories:               []string{"As a user, I want X so that Y"},
		FunctionalReqs:            []string{"FR1", "FR2"},
		NonFunctionalReqs:         []string{"Handle 1000 rps"},
		DesignConsiderations:      []string{"Match the dashboard style"},
		EngineeringConsiderations: []string{"Use existing auth middleware"},
		RolloutPlan:               []string{"Phase 1", "Phase 2"},
		SuccessMetrics:            []string{"Reduced bounce rate"},
		OpenQuestions:             []string{"How to measure?"},
		References:                []string{"product/pitches/foo.md", "design/system.md"},
	}
	got := renderPRDBody(s)

	wantHas := []string{
		"# Sample PRD",
		"## Problem",
		"The problem framing",
		"## Goals",
		"- Goal one",
		"## Non-goals",
		"## User Stories",
		"## Functional Requirements",
		"## Non-functional Requirements",
		"## Design Considerations",
		"## Engineering Considerations",
		"## Rollout Plan",
		"## Success Metrics",
		"## Open Questions",
		"## References",
		"[[product/pitches/foo]]",
		"[[design/system]]",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("body missing %q", sub)
		}
	}
}

func TestRenderPRDBodyMinimal(t *testing.T) {
	s := &Synthesis{Title: "Minimal"}
	got := renderPRDBody(s)
	if !strings.Contains(got, "# Minimal") {
		t.Errorf("missing title heading: %q", got)
	}
	// Empty sections should not be rendered.
	if strings.Contains(got, "## Goals") {
		t.Errorf("should not render empty Goals section")
	}
}

func TestWriteList(t *testing.T) {
	t.Run("writes items under heading", func(t *testing.T) {
		var b strings.Builder
		writeList(&b, "Things", []string{"a", "b", "c"})
		got := b.String()
		if !strings.Contains(got, "## Things") {
			t.Errorf("missing heading: %q", got)
		}
		if !strings.Contains(got, "- a") || !strings.Contains(got, "- b") || !strings.Contains(got, "- c") {
			t.Errorf("missing items: %q", got)
		}
	})

	t.Run("empty list renders nothing", func(t *testing.T) {
		var b strings.Builder
		writeList(&b, "Empty", nil)
		if b.Len() != 0 {
			t.Errorf("expected no output, got %q", b.String())
		}
	})

	t.Run("skips whitespace items", func(t *testing.T) {
		var b strings.Builder
		writeList(&b, "Mixed", []string{"real", "   ", "", "also real"})
		got := b.String()
		if strings.Count(got, "- ") != 2 {
			t.Errorf("expected 2 bullet lines, got %q", got)
		}
	})
}

func TestBuildPRDPrompt(t *testing.T) {
	context := []RetrievedNote{
		{Bucket: "design", Title: "Design System", Path: "design/system.md", Body: "Design body"},
		{Bucket: "engineering", Title: "Auth Middleware", Path: "eng/auth.md", Body: "Eng body"},
	}
	got := buildPRDPrompt("pitches/foo.md", "The pitch body", context, "", "", "", false)

	wantHas := []string{
		"senior product manager",
		"pitches/foo.md",
		"The pitch body",
		"design/system.md",
		"Design System",
		"Design body",
		"eng/auth.md",
		"Auth Middleware",
		"functional_requirements",
		"open_questions",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("prompt missing %q", sub)
		}
	}
}

func TestBuildPRDPromptTruncatesPitch(t *testing.T) {
	got := buildPRDPrompt("x.md", strings.Repeat("p", 20000), nil, "", "", "", false)
	if !strings.Contains(got, "[...truncated]") {
		t.Errorf("expected truncation for long pitch")
	}
}

func TestBuildPRDPromptTruncatesContextBody(t *testing.T) {
	context := []RetrievedNote{
		{Bucket: "design", Title: "Long", Path: "x.md", Body: strings.Repeat("p", 5000)},
	}
	got := buildPRDPrompt("x.md", "short", context, "", "", "", false)
	if !strings.Contains(got, "[...truncated]") {
		t.Errorf("expected truncation for long context body")
	}
}

func TestBuildPRDPromptRefineIncludesPrevious(t *testing.T) {
	got := buildPRDPrompt("pitches/foo.md", "pitch", nil, "", "reduce scope for v2", "PREVIOUS_BODY", true)
	for _, sub := range []string{
		"Revise the existing PRD",
		"Current PRD (revise this)",
		"PREVIOUS_BODY",
		"reduce scope for v2",
	} {
		if !strings.Contains(got, sub) {
			t.Errorf("refine prompt missing %q", sub)
		}
	}
}

func TestBuildPRDPromptNonRefineOmitsPrevious(t *testing.T) {
	got := buildPRDPrompt("pitches/foo.md", "pitch", nil, "", "", "PREVIOUS_BODY", false)
	if strings.Contains(got, "Current PRD") {
		t.Errorf("non-refine prompt should not include previous-version block")
	}
	if strings.Contains(got, "PREVIOUS_BODY") {
		t.Errorf("non-refine prompt should not leak previous body")
	}
}
