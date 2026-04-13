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
	// Fast path: plain HTTP fetch (works for most server-rendered pages).
	body, err := httpGet(input.URL)
	if err != nil {
		// HTTP fetch failed entirely — try headless Chrome as fallback.
		body, err = chromedpGet(input.URL)
		if err != nil {
			return nil, fmt.Errorf("fetching url (http + chromedp both failed): %w", err)
		}
	}

	title := extractHTMLTitle(body)
	content := stripHTML(body)

	// If content is empty or too short, the page likely needs JS rendering.
	// Fall back to headless Chrome for the full rendered DOM.
	if contentLooksEmpty(content) {
		rendered, renderErr := chromedpGet(input.URL)
		if renderErr == nil {
			if t := extractHTMLTitle(rendered); t != "" {
				title = t
			}
			rendered = stripHTML(rendered)
			if len(rendered) > len(content) {
				content = rendered
			}
		}
	}

	if title == "" {
		title = input.URL
	}
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
