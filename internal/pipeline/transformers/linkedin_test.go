package transformers

import (
	"testing"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// rawInputWithURL is a shared test helper for constructing RawInput
// fixtures from a URL string.
func rawInputWithURL(u string) pipeline.RawInput {
	return pipeline.RawInput{Kind: pipeline.InputURL, URL: u}
}

func TestExtractLinkedInJSONLD(t *testing.T) {
	t.Run("articleBody with author", func(t *testing.T) {
		html := `<html><head><script type="application/ld+json">
{"@type":"Article","articleBody":"This is the body of the article.","author":{"@type":"Person","name":"Jane Doe"}}
</script></head></html>`
		content, author := extractLinkedInJSONLD(html)
		if content != "This is the body of the article." {
			t.Errorf("content = %q", content)
		}
		if author != "Jane Doe" {
			t.Errorf("author = %q", author)
		}
	})

	t.Run("articleBody without author", func(t *testing.T) {
		html := `<script type="application/ld+json">{"articleBody":"Body text only."}</script>`
		content, author := extractLinkedInJSONLD(html)
		if content != "Body text only." {
			t.Errorf("content = %q", content)
		}
		if author != "" {
			t.Errorf("author should be empty, got %q", author)
		}
	})

	t.Run("text field fallback", func(t *testing.T) {
		html := `<script type="application/ld+json">{"text":"Fallback body via text field"}</script>`
		content, _ := extractLinkedInJSONLD(html)
		if content != "Fallback body via text field" {
			t.Errorf("content = %q", content)
		}
	})

	t.Run("no JSON-LD returns empty", func(t *testing.T) {
		html := `<html><body>nothing here</body></html>`
		content, author := extractLinkedInJSONLD(html)
		if content != "" || author != "" {
			t.Errorf("expected empty, got content=%q author=%q", content, author)
		}
	})

	t.Run("multiple blocks picks first viable", func(t *testing.T) {
		html := `<script type="application/ld+json">{"@type":"WebPage"}</script>
<script type="application/ld+json">{"articleBody":"Second block wins"}</script>`
		content, _ := extractLinkedInJSONLD(html)
		if content != "Second block wins" {
			t.Errorf("content = %q", content)
		}
	})

	t.Run("malformed JSON is skipped", func(t *testing.T) {
		html := `<script type="application/ld+json">{not json</script>
<script type="application/ld+json">{"articleBody":"Valid after bad"}</script>`
		content, _ := extractLinkedInJSONLD(html)
		if content != "Valid after bad" {
			t.Errorf("content = %q", content)
		}
	})

	t.Run("no articleBody or text returns empty", func(t *testing.T) {
		html := `<script type="application/ld+json">{"@type":"Person","name":"Someone"}</script>`
		content, author := extractLinkedInJSONLD(html)
		if content != "" || author != "" {
			t.Errorf("expected empty, got content=%q author=%q", content, author)
		}
	})
}

func TestLinkedInCanHandle(t *testing.T) {
	tr := &LinkedInTransformer{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.linkedin.com/posts/jane-doe_activity-123", true},
		{"https://linkedin.com/pulse/article-slug-jane-doe", true},
		{"https://example.com/linkedin", false},
		{"https://twitter.com/foo", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			in := rawInputWithURL(tt.url)
			got := tr.CanHandle(&in)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
