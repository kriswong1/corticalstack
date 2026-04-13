package pipeline

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Renderer produces the frontmatter + body for a single note. Each
// intention has its own implementation.
type Renderer interface {
	Name() string
	Render(doc *TextDocument, extracted *Extracted) (map[string]interface{}, string)
}

// TemplateRegistry holds all renderers keyed by intention name.
type TemplateRegistry struct {
	items    map[string]Renderer
	fallback Renderer
}

// NewTemplateRegistry wires the default set of intention renderers.
func NewTemplateRegistry() *TemplateRegistry {
	r := &TemplateRegistry{items: make(map[string]Renderer)}
	r.Register(&LearningRenderer{})
	r.Register(&InformationRenderer{})
	r.Register(&ResearchRenderer{})
	r.Register(&ProjectRenderer{})
	r.Register(&OtherRenderer{})
	r.fallback = &InformationRenderer{}
	return r
}

// Register adds a renderer.
func (r *TemplateRegistry) Register(rend Renderer) {
	r.items[rend.Name()] = rend
}

// Pick returns the renderer for the given intention, or the fallback.
func (r *TemplateRegistry) Pick(intention string) Renderer {
	if rend, ok := r.items[intention]; ok {
		return rend
	}
	return r.fallback
}

// --- Shared helpers for renderers ---

func buildFrontmatter(doc *TextDocument, extracted *Extracted, intention string) map[string]interface{} {
	fm := map[string]interface{}{
		"id":        uuid.NewString(),
		"date":      docDateOrNow(doc),
		"type":      "note",
		"intention": intention,
		"source":    doc.Source,
		"ingested":  time.Now().Format(time.RFC3339),
	}
	if doc.ID != "" {
		fm["document_id"] = doc.ID
	}
	if doc.URL != "" {
		fm["source_url"] = doc.URL
	} else if u := doc.Metadata["url"]; u != "" {
		fm["source_url"] = u
	}
	if inputFile := doc.Metadata["input_file"]; inputFile != "" {
		fm["input_file"] = inputFile
	}
	// Pass through video metadata from the youtube transformer so callers
	// can search / group by channel or reference duration without opening
	// the note. These keys are only set when the transformer provides them.
	if videoID := doc.Metadata["video_id"]; videoID != "" {
		fm["video_id"] = videoID
	}
	if duration := doc.Metadata["duration"]; duration != "" {
		fm["duration"] = duration
	}
	if channel := doc.Metadata["channel"]; channel != "" {
		fm["channel"] = channel
	}
	if len(doc.Authors) > 0 {
		fm["authors"] = doc.Authors
	}
	if why := doc.Metadata["why"]; why != "" {
		fm["why"] = why
	}
	if projects := doc.Metadata["projects"]; projects != "" {
		ids := splitAndTrim(projects, ",")
		if len(ids) > 0 {
			fm["projects"] = ids
		}
	}
	if extracted != nil {
		if extracted.Domain != "" {
			fm["domain"] = extracted.Domain
		}
		if extracted.SourceURL != "" && fm["source_url"] == nil {
			fm["source_url"] = extracted.SourceURL
		}
		if len(extracted.Triggers) > 0 {
			fm["triggers"] = extracted.Triggers
		}
	}

	tags := []string{"cortical", intention}
	if extracted != nil {
		tags = append(tags, extracted.Tags...)
	}
	fm["tags"] = dedupTags(tags)

	return fm
}

func sourceLine(doc *TextDocument) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("> Source: %s", doc.Source))
	if len(doc.Authors) > 0 {
		b.WriteString(fmt.Sprintf(" · %s", strings.Join(doc.Authors, ", ")))
	}
	if doc.URL != "" {
		b.WriteString(fmt.Sprintf(" · [Original](%s)", doc.URL))
	}
	b.WriteString("\n\n")
	return b.String()
}

func whyLine(doc *TextDocument) string {
	why := doc.Metadata["why"]
	if strings.TrimSpace(why) == "" {
		return ""
	}
	return fmt.Sprintf("> **Why saved:** %s\n\n", why)
}

func foldContent(doc *TextDocument) string {
	var b strings.Builder
	if len(doc.Content) > 500 {
		b.WriteString("## Original Content\n\n<details>\n<summary>Full content (click to expand)</summary>\n\n")
		b.WriteString(doc.Content)
		b.WriteString("\n\n</details>\n")
	} else {
		b.WriteString("## Content\n\n")
		b.WriteString(doc.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func section(b *strings.Builder, title string) {
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n\n")
}

func bulletList(b *strings.Builder, items []string) {
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func writeActionLines(b *strings.Builder, actions []ActionItem) {
	for _, a := range actions {
		owner := a.Owner
		if owner == "" {
			owner = "TBD"
		}
		deadline := ""
		if a.Deadline != "" {
			deadline = fmt.Sprintf(" *(due: %s)*", a.Deadline)
		}
		idMarker := ""
		if a.ID != "" {
			idMarker = fmt.Sprintf(" <!-- id:%s -->", a.ID)
		}
		b.WriteString(fmt.Sprintf("- [ ] [%s] %s%s #status/pending%s\n", owner, a.Description, deadline, idMarker))
	}
	b.WriteString("\n")
}

func docDateOrNow(doc *TextDocument) string {
	if doc.Date.IsZero() {
		return time.Now().Format("2006-01-02")
	}
	return doc.Date.Format("2006-01-02")
}

func dedupTags(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		key := strings.ToLower(strings.TrimSpace(v))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// stableKeys returns sorted keys of a string-slice map.
func stableKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
