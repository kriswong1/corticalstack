package pipeline

import (
	"fmt"
	"strings"
)

// ProjectRenderer produces Summary · Impact on Project · Action Items · Integration Notes.
type ProjectRenderer struct{}

func (r *ProjectRenderer) Name() string { return "project-application" }

func (r *ProjectRenderer) Render(doc *TextDocument, extracted *Extracted) (map[string]interface{}, string) {
	fm := buildFrontmatter(doc, extracted, r.Name())

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", doc.Title))
	b.WriteString(sourceLine(doc))
	b.WriteString(whyLine(doc))

	if extracted != nil {
		if extracted.Summary != "" {
			section(&b, "Summary")
			b.WriteString(extracted.Summary)
			b.WriteString("\n\n")
		}
		if extracted.Impact != "" {
			section(&b, "Impact on Project")
			b.WriteString(extracted.Impact)
			b.WriteString("\n\n")
		} else if extracted.Relevance != "" {
			section(&b, "Impact on Project")
			b.WriteString(extracted.Relevance)
			b.WriteString("\n\n")
		}
		if len(extracted.KeyPoints) > 0 {
			section(&b, "Key Points")
			bulletList(&b, extracted.KeyPoints)
		}
		if len(extracted.Actions) > 0 {
			section(&b, "Action Items")
			writeActionLines(&b, extracted.Actions)
		}
		if extracted.Integration != "" {
			section(&b, "Integration Notes")
			b.WriteString(extracted.Integration)
			b.WriteString("\n\n")
		}
		if len(extracted.NextSteps) > 0 {
			section(&b, "Next Steps")
			bulletList(&b, extracted.NextSteps)
		}
	}

	b.WriteString(foldContent(doc))
	return fm, b.String()
}
