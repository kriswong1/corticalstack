package pipeline

import (
	"strings"
	"testing"
	"time"
)

func TestDedupTags(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"dedupes case-insensitive", []string{"Go", "go", "GO"}, []string{"go"}},
		{"preserves order of first occurrence", []string{"b", "a", "b"}, []string{"b", "a"}},
		{"trims whitespace", []string{"  go  ", "rust"}, []string{"go", "rust"}},
		{"drops empty and whitespace-only", []string{"", "   ", "tag"}, []string{"tag"}},
		{"empty input", nil, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupTags(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, v := range tt.want {
				if got[i] != v {
					t.Errorf("[%d] got %q, want %q", i, got[i], v)
				}
			}
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		in   string
		sep  string
		want []string
	}{
		{"a, b, c", ",", []string{"a", "b", "c"}},
		{"  padded  ,  with , spaces  ", ",", []string{"padded", "with", "spaces"}},
		{"", ",", []string{}},
		{"no-sep-here", ",", []string{"no-sep-here"}},
		{",leading,trailing,", ",", []string{"leading", "trailing"}},
		{"a||b||c", "||", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		got := splitAndTrim(tt.in, tt.sep)
		if len(got) != len(tt.want) {
			t.Errorf("splitAndTrim(%q, %q) = %v, want %v", tt.in, tt.sep, got, tt.want)
			continue
		}
		for i, v := range tt.want {
			if got[i] != v {
				t.Errorf("[%d] got %q, want %q", i, got[i], v)
			}
		}
	}
}

func TestBulletList(t *testing.T) {
	t.Run("renders items", func(t *testing.T) {
		var b strings.Builder
		bulletList(&b, []string{"a", "b", "c"})
		got := b.String()
		for _, want := range []string{"- a", "- b", "- c"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in %q", want, got)
			}
		}
	})

	t.Run("skips empty and whitespace", func(t *testing.T) {
		var b strings.Builder
		bulletList(&b, []string{"real", "", "  ", "also"})
		got := b.String()
		if strings.Count(got, "- ") != 2 {
			t.Errorf("expected 2 bullet lines, got %q", got)
		}
	})
}

func TestSection(t *testing.T) {
	var b strings.Builder
	section(&b, "My Heading")
	if b.String() != "## My Heading\n\n" {
		t.Errorf("got %q", b.String())
	}
}

func TestSourceLine(t *testing.T) {
	tests := []struct {
		name    string
		doc     *TextDocument
		wantHas []string
	}{
		{
			name:    "source only",
			doc:     &TextDocument{Source: "webpage"},
			wantHas: []string{"> Source: webpage"},
		},
		{
			name:    "source + authors",
			doc:     &TextDocument{Source: "pdf", Authors: []string{"Alice", "Bob"}},
			wantHas: []string{"> Source: pdf", "Alice, Bob"},
		},
		{
			name:    "source + url",
			doc:     &TextDocument{Source: "webpage", URL: "https://example.com"},
			wantHas: []string{"> Source: webpage", "[Original](https://example.com)"},
		},
		{
			name:    "source + authors + url",
			doc:     &TextDocument{Source: "linkedin", Authors: []string{"Jane"}, URL: "https://l.in/foo"},
			wantHas: []string{"Source: linkedin", "Jane", "[Original](https://l.in/foo)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourceLine(tt.doc)
			for _, sub := range tt.wantHas {
				if !strings.Contains(got, sub) {
					t.Errorf("missing %q in %q", sub, got)
				}
			}
		})
	}
}

func TestWhyLine(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		doc := &TextDocument{Metadata: map[string]string{"why": "Interesting idea"}}
		got := whyLine(doc)
		if !strings.Contains(got, "Why saved") || !strings.Contains(got, "Interesting idea") {
			t.Errorf("got %q", got)
		}
	})
	t.Run("empty whitespace-only returns empty", func(t *testing.T) {
		doc := &TextDocument{Metadata: map[string]string{"why": "   "}}
		if got := whyLine(doc); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
	t.Run("missing key returns empty", func(t *testing.T) {
		doc := &TextDocument{Metadata: map[string]string{}}
		if got := whyLine(doc); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}

func TestFoldContent(t *testing.T) {
	t.Run("short content not folded", func(t *testing.T) {
		doc := &TextDocument{Content: "short body"}
		got := foldContent(doc)
		if !strings.Contains(got, "## Content") {
			t.Errorf("expected plain Content section, got %q", got)
		}
		if strings.Contains(got, "<details>") {
			t.Errorf("short content should not be folded")
		}
	})

	t.Run("long content folded", func(t *testing.T) {
		doc := &TextDocument{Content: strings.Repeat("x", 600)}
		got := foldContent(doc)
		if !strings.Contains(got, "<details>") {
			t.Errorf("expected <details> fold, got %q", got[:100])
		}
		if !strings.Contains(got, "## Original Content") {
			t.Errorf("expected 'Original Content' heading")
		}
	})
}

func TestDocDateOrNow(t *testing.T) {
	t.Run("with date", func(t *testing.T) {
		doc := &TextDocument{Date: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)}
		if got := docDateOrNow(doc); got != "2026-04-11" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("zero date falls back to now", func(t *testing.T) {
		doc := &TextDocument{}
		got := docDateOrNow(doc)
		if got != time.Now().Format("2006-01-02") {
			t.Errorf("got %q, expected today's date", got)
		}
	})
}

func TestWriteActionLines(t *testing.T) {
	var b strings.Builder
	writeActionLines(&b, []ActionItem{
		{ID: "act-1", Owner: "Alice", Description: "Review PR", Deadline: "Friday"},
		{Owner: "", Description: "Unowned task"},
	})
	got := b.String()
	wantHas := []string{
		"- [ ] [Alice] Review PR",
		"*(due: Friday)*",
		"<!-- id:act-1 -->",
		"#status/pending",
		"- [ ] [TBD] Unowned task",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("missing %q in %q", sub, got)
		}
	}
}

func TestStableKeys(t *testing.T) {
	m := map[string][]string{
		"zeta":  nil,
		"alpha": nil,
		"beta":  nil,
	}
	got := stableKeys(m)
	want := []string{"alpha", "beta", "zeta"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] got %q, want %q", i, got[i], w)
		}
	}
}

func TestBuildFrontmatter(t *testing.T) {
	doc := &TextDocument{
		ID:      "doc-1",
		Source:  "webpage",
		Title:   "A",
		URL:     "https://e.com",
		Authors: []string{"Jane"},
		Date:    time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC),
		Metadata: map[string]string{
			"why":      "Relevant",
			"projects": "p1, p2",
		},
	}
	ex := &Extracted{
		Domain:   "engineering",
		Triggers: []string{"when debugging"},
		Tags:     []string{"go", "debug"},
	}
	got := buildFrontmatter(doc, ex, "learning")

	if got["intention"] != "learning" {
		t.Errorf("intention = %v", got["intention"])
	}
	if got["source"] != "webpage" {
		t.Errorf("source = %v", got["source"])
	}
	if got["date"] != "2026-04-11" {
		t.Errorf("date = %v", got["date"])
	}
	if got["source_url"] != "https://e.com" {
		t.Errorf("source_url = %v", got["source_url"])
	}
	if got["domain"] != "engineering" {
		t.Errorf("domain = %v", got["domain"])
	}
	if got["why"] != "Relevant" {
		t.Errorf("why = %v", got["why"])
	}
	// projects from metadata should be parsed into slice
	projects, ok := got["projects"].([]string)
	if !ok || len(projects) != 2 {
		t.Errorf("projects = %v", got["projects"])
	}
	// tags should include "cortical", "learning", plus extracted tags, deduped+lowered
	tags, ok := got["tags"].([]string)
	if !ok {
		t.Fatalf("tags type = %T", got["tags"])
	}
	if !contains(tags, "cortical") || !contains(tags, "learning") || !contains(tags, "go") {
		t.Errorf("tags missing entries: %v", tags)
	}
}

func TestRenderersProduceOutput(t *testing.T) {
	doc := &TextDocument{
		Source:  "webpage",
		Title:   "Sample",
		URL:     "https://e.com",
		Content: "some body text",
	}
	ex := &Extracted{
		Summary:   "A summary",
		KeyPoints: []string{"Key 1"},
	}

	renderers := []Renderer{
		&LearningRenderer{},
		&InformationRenderer{},
		&ResearchRenderer{},
		&ProjectRenderer{},
		&OtherRenderer{},
	}
	for _, rend := range renderers {
		t.Run(rend.Name(), func(t *testing.T) {
			fm, body := rend.Render(doc, ex)
			if fm == nil {
				t.Error("frontmatter is nil")
			}
			if body == "" {
				t.Error("body is empty")
			}
			if !strings.Contains(body, "Sample") && !strings.Contains(body, "A summary") {
				t.Errorf("body missing core fields: %q", body[:min(len(body), 200)])
			}
		})
	}
}

func TestTemplateRegistryPick(t *testing.T) {
	r := NewTemplateRegistry()
	if r.Pick("learning").Name() != "learning" {
		t.Errorf("learning pick wrong")
	}
	if r.Pick("information").Name() != "information" {
		t.Errorf("information pick wrong")
	}
	// Unknown falls back to information.
	if r.Pick("bogus").Name() != "information" {
		t.Errorf("unknown should fall back to information")
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
