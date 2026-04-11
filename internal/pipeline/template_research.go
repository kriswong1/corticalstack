package pipeline

import (
	"fmt"
	"strings"
)

// ResearchRenderer produces Summary · Findings · Relevance · Sources · Next Steps.
type ResearchRenderer struct{}

func (r *ResearchRenderer) Name() string { return "research" }

func (r *ResearchRenderer) Render(doc *TextDocument, extracted *Extracted) (map[string]interface{}, string) {
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
		if len(extracted.Findings) > 0 {
			section(&b, "Findings")
			bulletList(&b, extracted.Findings)
		}
		if len(extracted.KeyPoints) > 0 {
			section(&b, "Key Points")
			bulletList(&b, extracted.KeyPoints)
		}
		if extracted.Relevance != "" {
			section(&b, "Relevance to Projects")
			b.WriteString(extracted.Relevance)
			b.WriteString("\n\n")
		}
		if len(extracted.Sources) > 0 {
			section(&b, "Sources")
			bulletList(&b, extracted.Sources)
		}
		if len(extracted.NextSteps) > 0 {
			section(&b, "Next Steps")
			bulletList(&b, extracted.NextSteps)
		}
		if len(extracted.Actions) > 0 {
			section(&b, "Action Items")
			writeActionLines(&b, extracted.Actions)
		}
	}

	b.WriteString(foldContent(doc))
	return fm, b.String()
}
