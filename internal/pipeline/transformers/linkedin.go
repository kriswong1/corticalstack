package transformers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// LinkedInTransformer fetches a LinkedIn post or article and extracts its content
// from embedded JSON-LD, falling back to HTML stripping if needed.
type LinkedInTransformer struct{}

func (t *LinkedInTransformer) Name() string { return "linkedin" }

func (t *LinkedInTransformer) CanHandle(input *pipeline.RawInput) bool {
	u := input.URL
	return strings.Contains(u, "linkedin.com/posts/") || strings.Contains(u, "linkedin.com/pulse/")
}

func (t *LinkedInTransformer) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, error) {
	body, err := httpGet(input.URL)
	if err != nil {
		return nil, fmt.Errorf("fetching linkedin: %w", err)
	}

	content, author := extractLinkedInJSONLD(body)
	if content == "" {
		content = stripHTML(body)
	}
	if content == "" {
		return nil, fmt.Errorf("could not extract linkedin content (may require auth)")
	}

	title := extractHTMLTitle(body)
	if title == "" {
		title = "LinkedIn Post"
	}

	authors := []string{}
	if author != "" {
		authors = append(authors, author)
	}

	return &pipeline.TextDocument{
		ID:      input.URL,
		Source:  "linkedin",
		Title:   title,
		URL:     input.URL,
		Date:    time.Now(),
		Authors: authors,
		Content: content,
		Metadata: mergeMeta(input.Metadata, map[string]string{
			"url": input.URL,
		}),
	}, nil
}

var jsonLDRe = regexp.MustCompile(`(?is)<script[^>]+type="application/ld\+json"[^>]*>([\s\S]*?)</script>`)

func extractLinkedInJSONLD(html string) (content, author string) {
	matches := jsonLDRe.FindAllStringSubmatch(html, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		var data map[string]interface{}
		if json.Unmarshal([]byte(match[1]), &data) != nil {
			continue
		}
		if text, ok := data["articleBody"].(string); ok && text != "" {
			if a, ok := data["author"].(map[string]interface{}); ok {
				if name, ok := a["name"].(string); ok {
					return text, name
				}
			}
			return text, ""
		}
		if text, ok := data["text"].(string); ok && text != "" {
			return text, ""
		}
	}
	return "", ""
}
