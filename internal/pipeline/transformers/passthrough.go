package transformers

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// PassthroughTransformer handles plain text, .txt, and .md inputs.
// It is the registry fallback, so it accepts anything no other transformer claims.
type PassthroughTransformer struct{}

func (t *PassthroughTransformer) Name() string { return "passthrough" }

func (t *PassthroughTransformer) CanHandle(input *pipeline.RawInput) bool {
	if input.Kind == pipeline.InputText {
		return true
	}
	ext := strings.ToLower(filepath.Ext(input.Path))
	if input.Path == "" {
		ext = strings.ToLower(filepath.Ext(input.Filename))
	}
	switch ext {
	case "", ".txt", ".md", ".markdown":
		return true
	}
	return false
}

func (t *PassthroughTransformer) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, error) {
	content := readInputBytes(input)
	if content == "" {
		return nil, fmt.Errorf("passthrough: empty input")
	}

	title := input.Title
	if title == "" {
		if name := input.Filename; name != "" {
			title = strings.TrimSuffix(name, filepath.Ext(name))
		} else if input.Path != "" {
			title = strings.TrimSuffix(filepath.Base(input.Path), filepath.Ext(input.Path))
		} else {
			title = firstLineTitle(content)
		}
	}

	return &pipeline.TextDocument{
		ID:      identifierFor(input),
		Source:  "passthrough",
		Title:   title,
		Date:    time.Now(),
		Content: content,
		Metadata: mergeMeta(input.Metadata, map[string]string{
			"input_kind": string(input.Kind),
		}),
	}, nil
}

// firstLineTitle returns the first non-empty line, trimmed to 80 chars.
func firstLineTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 80 {
			line = line[:80]
		}
		return line
	}
	return "Untitled"
}
