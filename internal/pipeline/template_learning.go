package pipeline

import (
	"fmt"
	"strings"
)

// LearningRenderer produces Summary · Key Points · How This Applies · Open Questions.
type LearningRenderer struct{}

func (r *LearningRenderer) Name() string { return "learning" }

func (r *LearningRenderer) Render(doc *TextDocument, extracted *Extracted) (map[string]interface{}, string) {
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
		if len(extracted.KeyPoints) > 0 {
			section(&b, "Key Points")
			bulletList(&b, extracted.KeyPoints)
		}
		if extracted.HowThisApplies != "" {
			section(&b, "How This Applies")
			b.WriteString(extracted.HowThisApplies)
			b.WriteString("\n\n")
		}
		if len(extracted.OpenQuestions) > 0 {
			section(&b, "Open Questions")
			bulletList(&b, extracted.OpenQuestions)
		}
		if len(extracted.Actions) > 0 {
			section(&b, "Action Items")
			writeActionLines(&b, extracted.Actions)
		}
	}

	b.WriteString(foldContent(doc))
	return fm, b.String()
}
