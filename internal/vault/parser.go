package vault

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

var frontmatterDelimiter = []byte("---")

// Note represents a markdown file with optional YAML frontmatter.
type Note struct {
	Frontmatter map[string]interface{} `yaml:",inline"`
	Body        string
	Path        string
}

// ParseNote splits a markdown file into frontmatter and body.
func ParseNote(content []byte) (*Note, error) {
	note := &Note{Frontmatter: make(map[string]interface{})}

	trimmed := bytes.TrimSpace(content)
	if !bytes.HasPrefix(trimmed, frontmatterDelimiter) {
		note.Body = string(content)
		return note, nil
	}

	rest := trimmed[len(frontmatterDelimiter):]
	rest = bytes.TrimLeft(rest, "\r\n")
	idx := bytes.Index(rest, frontmatterDelimiter)
	if idx < 0 {
		note.Body = string(content)
		return note, nil
	}

	fmBytes := rest[:idx]
	bodyBytes := rest[idx+len(frontmatterDelimiter):]

	if err := yaml.Unmarshal(fmBytes, &note.Frontmatter); err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}
	if note.Frontmatter == nil {
		note.Frontmatter = make(map[string]interface{})
	}

	note.Body = string(bytes.TrimLeft(bodyBytes, "\r\n"))
	return note, nil
}

// RenderNote serializes a Note back to markdown with YAML frontmatter.
func RenderNote(note *Note) ([]byte, error) {
	var buf bytes.Buffer

	if len(note.Frontmatter) > 0 {
		buf.WriteString("---\n")
		fmBytes, err := yaml.Marshal(note.Frontmatter)
		if err != nil {
			return nil, fmt.Errorf("marshaling frontmatter: %w", err)
		}
		buf.Write(fmBytes)
		buf.WriteString("---\n")
	}

	if note.Body != "" {
		if len(note.Frontmatter) > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(note.Body)
		if note.Body[len(note.Body)-1] != '\n' {
			buf.WriteString("\n")
		}
	}

	return buf.Bytes(), nil
}
