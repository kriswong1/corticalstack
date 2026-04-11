package transformers

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// HTMLTransformer handles local .html files or pasted HTML text.
type HTMLTransformer struct{}

func (t *HTMLTransformer) Name() string { return "html" }

func (t *HTMLTransformer) CanHandle(input *pipeline.RawInput) bool {
	ext := strings.ToLower(filepath.Ext(input.Path))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(input.Filename))
	}
	if ext == ".html" || ext == ".htm" {
		return true
	}
	if input.Kind == pipeline.InputText {
		content := string(input.Content)
		return strings.Contains(content, "<html") || strings.Contains(content, "<body")
	}
	return false
}

func (t *HTMLTransformer) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, error) {
	raw := readInputBytes(input)
	if raw == "" {
		return nil, fmt.Errorf("html: empty input")
	}

	title := input.Title
	if title == "" {
		title = extractHTMLTitle(raw)
	}
	if title == "" {
		name := input.Filename
		if name == "" {
			name = filepath.Base(input.Path)
		}
		title = strings.TrimSuffix(name, filepath.Ext(name))
	}
	if title == "" {
		title = "HTML Document"
	}

	text := stripHTML(raw)
	if text == "" {
		return nil, fmt.Errorf("html: no readable content after stripping")
	}

	return &pipeline.TextDocument{
		ID:      identifierFor(input),
		Source:  "html",
		Title:   title,
		Date:    htmlDate(input),
		Content: text,
		Metadata: mergeMeta(input.Metadata, map[string]string{
			"input_file": input.Path,
		}),
	}, nil
}

func htmlDate(input *pipeline.RawInput) time.Time {
	if input.Path != "" {
		return fileModTime(input.Path)
	}
	return time.Now()
}
