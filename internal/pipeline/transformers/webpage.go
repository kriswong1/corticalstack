package transformers

import (
	"fmt"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// WebPageTransformer fetches a URL and strips HTML to readable text.
// Runs after YouTube/LinkedIn so those hosts get their specialized handling.
type WebPageTransformer struct{}

func (t *WebPageTransformer) Name() string { return "webpage" }

func (t *WebPageTransformer) CanHandle(input *pipeline.RawInput) bool {
	u := input.URL
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func (t *WebPageTransformer) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, error) {
	body, err := httpGet(input.URL)
	if err != nil {
		return nil, fmt.Errorf("fetching url: %w", err)
	}

	title := extractHTMLTitle(body)
	if title == "" {
		title = input.URL
	}

	content := stripHTML(body)
	if content == "" {
		return nil, fmt.Errorf("no readable content extracted from %s", input.URL)
	}

	return &pipeline.TextDocument{
		ID:      input.URL,
		Source:  "webpage",
		Title:   title,
		URL:     input.URL,
		Date:    time.Now(),
		Content: content,
		Metadata: mergeMeta(input.Metadata, map[string]string{
			"url": input.URL,
		}),
	}, nil
}
