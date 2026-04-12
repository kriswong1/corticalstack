package usecases

import (
	"strings"
	"testing"
)

func TestParseUseCases(t *testing.T) {
	projectIDs := []string{"proj-a"}
	sources := []SourceRef{{Type: "doc", Path: "notes/x.md"}}

	t.Run("json array", func(t *testing.T) {
		raw := `[
			{"title":"Sign In","actors":["User"],"main_flow":["Enter creds","Submit"]},
			{"title":"Sign Out","actors":["User"],"main_flow":["Click logout"]}
		]`
		got, err := parseUseCases(raw, projectIDs, sources)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].Title != "Sign In" {
			t.Errorf("first title = %q", got[0].Title)
		}
		if got[0].Projects[0] != "proj-a" {
			t.Errorf("projects not attached: %v", got[0].Projects)
		}
		if got[0].Sources[0].Path != "notes/x.md" {
			t.Errorf("sources not attached: %v", got[0].Sources)
		}
	})

	t.Run("single object fallback", func(t *testing.T) {
		raw := `{"title":"Solo","actors":["User"],"main_flow":["Do it"]}`
		got, err := parseUseCases(raw, projectIDs, sources)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
	})

	t.Run("markdown fenced json", func(t *testing.T) {
		raw := "```json\n[{\"title\":\"Fenced\",\"actors\":[\"U\"],\"main_flow\":[\"x\"]}]\n```"
		got, err := parseUseCases(raw, projectIDs, sources)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Title != "Fenced" {
			t.Errorf("got = %+v", got)
		}
	})

	t.Run("malformed json errors", func(t *testing.T) {
		_, err := parseUseCases(`{not valid`, projectIDs, sources)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestBuildFromDocPrompt(t *testing.T) {
	got := buildFromDocPrompt("notes/article.md", "This is an article body.", "focus on auth flows")
	wantHas := []string{
		"notes/article.md",
		"This is an article body",
		"focus on auth flows",
		"User hint",
		"JSON array",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("prompt missing %q", sub)
		}
	}
}

func TestBuildFromDocPromptTruncates(t *testing.T) {
	got := buildFromDocPrompt("src.md", strings.Repeat("x", 25000), "")
	if !strings.Contains(got, "[...truncated]") {
		t.Errorf("expected truncation marker")
	}
}

func TestBuildFromTextPrompt(t *testing.T) {
	got := buildFromTextPrompt("User wants to reset password", "user, admin")
	wantHas := []string{"User wants to reset password", "user, admin", "Actors hint", "JSON array"}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("prompt missing %q", sub)
		}
	}
}

func TestBuildFromTextPromptNoHint(t *testing.T) {
	got := buildFromTextPrompt("Something", "")
	if strings.Contains(got, "Actors hint") {
		t.Errorf("empty hint should not produce hint section")
	}
}

func TestCommonUseCasePromptTail(t *testing.T) {
	got := commonUseCasePromptTail()
	wantHas := []string{"Output format", "JSON array", "title", "main_flow", "alternative_flows", "business_rules", "non_functional"}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("tail missing %q", sub)
		}
	}
}
