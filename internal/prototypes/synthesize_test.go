package prototypes

import (
	"strings"
	"testing"
)

func TestParseJSONResponse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantNil bool
		wantKey string
	}{
		{
			name:    "valid json",
			in:      `{"summary":"hello","count":3}`,
			wantKey: "summary",
		},
		{
			name:    "markdown fenced json",
			in:      "```json\n{\"a\":1}\n```",
			wantKey: "a",
		},
		{
			name:    "plain fenced json",
			in:      "```\n{\"b\":2}\n```",
			wantKey: "b",
		},
		{
			name:    "with whitespace padding",
			in:      "   \n{\"c\":3}\n   ",
			wantKey: "c",
		},
		{
			name:    "malformed json returns nil",
			in:      "{not json",
			wantNil: true,
		},
		{
			name:    "empty string returns nil",
			in:      "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseJSONResponse(tt.in)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected map, got nil")
			}
			if _, ok := got[tt.wantKey]; !ok {
				t.Errorf("expected key %q, have %v", tt.wantKey, got)
			}
		})
	}
}

func TestBuildSynthesisPrompt(t *testing.T) {
	format := &ScreenFlow{}
	got := buildSynthesisPrompt(format, "source document content", "make it mobile first")
	wantHas := []string{
		"senior product designer",
		"screen-flow",
		"source document content",
		"make it mobile first",
		"User hints",
		"SchemaHint",
	}
	// SchemaHint content leaks into prompt:
	wantHas = append(wantHas, "screens", "transitions")
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) && sub != "SchemaHint" {
			t.Errorf("prompt missing %q", sub)
		}
	}
}

func TestBuildSynthesisPromptNoHints(t *testing.T) {
	got := buildSynthesisPrompt(&ScreenFlow{}, "content", "")
	if strings.Contains(got, "User hints") {
		t.Errorf("empty hints should not produce hints section")
	}
}
