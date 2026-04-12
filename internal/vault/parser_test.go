package vault

import (
	"strings"
	"testing"
)

func TestParseNote(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFMKeys  []string
		wantBody    string
		wantErr     bool
	}{
		{
			name:       "frontmatter and body",
			input:      "---\ntitle: Hello\ntype: note\n---\n\nBody content here.\n",
			wantFMKeys: []string{"title", "type"},
			wantBody:   "Body content here.",
		},
		{
			name:       "body only, no frontmatter",
			input:      "Just a plain markdown body.\n",
			wantFMKeys: nil,
			wantBody:   "Just a plain markdown body.\n",
		},
		{
			name:       "frontmatter only, empty body",
			input:      "---\nkey: value\n---\n",
			wantFMKeys: []string{"key"},
			wantBody:   "",
		},
		{
			name:       "empty input",
			input:      "",
			wantFMKeys: nil,
			wantBody:   "",
		},
		{
			name:       "missing closing delimiter treats whole as body",
			input:      "---\ntitle: Hello\nbody continues forever",
			wantFMKeys: nil,
			wantBody:   "---\ntitle: Hello\nbody continues forever",
		},
		{
			name:       "unicode in frontmatter and body",
			input:      "---\ntitle: \"日本語\"\n---\n\némoji 🌍 body\n",
			wantFMKeys: []string{"title"},
			wantBody:   "émoji 🌍 body",
		},
		{
			name:       "CRLF line endings",
			input:      "---\r\ntitle: Hello\r\n---\r\n\r\nBody\r\n",
			wantFMKeys: []string{"title"},
			wantBody:   "Body",
		},
		{
			name:       "frontmatter delimiter appearing in body",
			input:      "---\ntitle: A\n---\n\nSome prose\n\n---\n\nMore prose\n",
			wantFMKeys: []string{"title"},
			wantBody:   "Some prose\n\n---\n\nMore prose",
		},
		{
			name:       "frontmatter with list and number",
			input:      "---\ntags: [a, b, c]\ncount: 42\n---\n\nbody\n",
			wantFMKeys: []string{"tags", "count"},
			wantBody:   "body",
		},
		{
			name:    "malformed yaml errors",
			input:   "---\ntitle: [unclosed\n---\n\nbody\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note, err := ParseNote([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if note.Body != tt.wantBody {
				t.Errorf("body: got %q, want %q", note.Body, tt.wantBody)
			}
			for _, k := range tt.wantFMKeys {
				if _, ok := note.Frontmatter[k]; !ok {
					t.Errorf("frontmatter missing key %q (have: %v)", k, note.Frontmatter)
				}
			}
		})
	}
}

func TestRenderNote(t *testing.T) {
	tests := []struct {
		name     string
		note     *Note
		contains []string
		exact    string
	}{
		{
			name: "frontmatter and body",
			note: &Note{
				Frontmatter: map[string]interface{}{"title": "Hello", "type": "note"},
				Body:        "Body content",
			},
			contains: []string{"---\n", "title: Hello", "type: note", "---\n", "\nBody content\n"},
		},
		{
			name:     "body only, no frontmatter",
			note:     &Note{Body: "just body"},
			exact:    "just body\n",
		},
		{
			name:     "empty note",
			note:     &Note{},
			exact:    "",
		},
		{
			name: "body already ends with newline",
			note: &Note{Body: "already newlined\n"},
			exact: "already newlined\n",
		},
		{
			name: "frontmatter only, empty body",
			note: &Note{Frontmatter: map[string]interface{}{"k": "v"}},
			contains: []string{"---\n", "k: v"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := RenderNote(tt.note)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := string(out)
			if tt.exact != "" && got != tt.exact {
				t.Errorf("got %q, want %q", got, tt.exact)
			}
			for _, sub := range tt.contains {
				if !strings.Contains(got, sub) {
					t.Errorf("output missing %q\nfull output: %q", sub, got)
				}
			}
		})
	}
}

func TestParseRenderRoundtrip(t *testing.T) {
	inputs := []string{
		"---\ntitle: Hello\ntype: note\n---\n\nBody text.\n",
		"Plain body only.\n",
		"---\ntags:\n    - a\n    - b\n---\n\nBody with list tags.\n",
	}
	for _, in := range inputs {
		t.Run(in[:min(len(in), 20)], func(t *testing.T) {
			note1, err := ParseNote([]byte(in))
			if err != nil {
				t.Fatalf("first parse: %v", err)
			}
			rendered, err := RenderNote(note1)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			note2, err := ParseNote(rendered)
			if err != nil {
				t.Fatalf("second parse: %v", err)
			}
			if note1.Body != note2.Body {
				t.Errorf("body drift:\n  first:  %q\n  second: %q", note1.Body, note2.Body)
			}
			if len(note1.Frontmatter) != len(note2.Frontmatter) {
				t.Errorf("frontmatter key count drift: %d vs %d", len(note1.Frontmatter), len(note2.Frontmatter))
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
