package questions

import (
	"strings"
	"testing"
)

func TestParseQuestions(t *testing.T) {
	t.Run("plain array", func(t *testing.T) {
		raw := `[{"id":"a","prompt":"first?","kind":"text"},{"id":"b","prompt":"pick","kind":"choice","choices":["x","y"]}]`
		qs, err := parseQuestions(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(qs) != 2 {
			t.Fatalf("want 2 questions, got %d", len(qs))
		}
		if qs[0].Kind != "text" || qs[1].Kind != "choice" {
			t.Errorf("kinds not preserved: %+v", qs)
		}
	})

	t.Run("code fences are stripped", func(t *testing.T) {
		raw := "```json\n[{\"id\":\"a\",\"prompt\":\"first?\",\"kind\":\"text\"}]\n```"
		qs, err := parseQuestions(raw)
		if err != nil || len(qs) != 1 {
			t.Fatalf("got %d questions err=%v", len(qs), err)
		}
	})

	t.Run("empty array means no questions needed", func(t *testing.T) {
		qs, err := parseQuestions("[]")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(qs) != 0 {
			t.Errorf("want 0 questions, got %d", len(qs))
		}
	})

	t.Run("choice without choices is downgraded to text", func(t *testing.T) {
		raw := `[{"id":"a","prompt":"pick","kind":"choice"}]`
		qs, _ := parseQuestions(raw)
		if len(qs) != 1 || qs[0].Kind != "text" {
			t.Errorf("expected downgrade to text, got %+v", qs)
		}
	})

	t.Run("blank prompts dropped", func(t *testing.T) {
		raw := `[{"id":"a","prompt":"","kind":"text"},{"id":"b","prompt":"ok","kind":"text"}]`
		qs, _ := parseQuestions(raw)
		if len(qs) != 1 || qs[0].ID != "b" {
			t.Errorf("want 1 (b), got %+v", qs)
		}
	})

	t.Run("cap at 5 questions", func(t *testing.T) {
		var items []string
		for i := 0; i < 8; i++ {
			items = append(items, `{"prompt":"q","kind":"text"}`)
		}
		raw := "[" + strings.Join(items, ",") + "]"
		qs, _ := parseQuestions(raw)
		if len(qs) != 5 {
			t.Errorf("want 5 (cap), got %d", len(qs))
		}
	})
}

func TestFormatAnswers(t *testing.T) {
	qs := []Question{
		{ID: "appetite", Prompt: "What's the appetite?"},
		{ID: "users", Prompt: "Who feels the pain?"},
	}
	ans := []Answer{
		{ID: "appetite", Value: "6 weeks"},
		{ID: "users", Value: "Power users"},
	}
	got := FormatAnswers(qs, ans)
	if !strings.Contains(got, "What's the appetite?") || !strings.Contains(got, "6 weeks") {
		t.Errorf("expected rendered Q&A, got: %s", got)
	}
	if !strings.Contains(got, "Power users") {
		t.Errorf("expected second answer, got: %s", got)
	}
}

func TestFormatAnswers_empty(t *testing.T) {
	if FormatAnswers(nil, nil) != "" {
		t.Error("expected empty string for no answers")
	}
}

func TestFormatAnswers_blankValueSkipped(t *testing.T) {
	qs := []Question{{ID: "x", Prompt: "X?"}}
	ans := []Answer{{ID: "x", Value: "   "}}
	got := FormatAnswers(qs, ans)
	if strings.Contains(got, "X?") {
		t.Errorf("blank value should be skipped, got: %s", got)
	}
}

func TestBuildAskPrompt_containsGoalAndBlocks(t *testing.T) {
	blocks := []ContextBlock{{Heading: "Pitch", Body: "this is a pitch"}}
	p := buildAskPrompt("draft the shape stage", blocks)
	for _, want := range []string{"draft the shape stage", "## Pitch", "this is a pitch", "JSON array"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
