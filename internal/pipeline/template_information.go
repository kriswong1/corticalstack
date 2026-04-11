package pipeline

import (
	"fmt"
	"strings"
)

// InformationRenderer produces Structured Facts · Claims · Definitions.
type InformationRenderer struct{}

func (r *InformationRenderer) Name() string { return "information" }

func (r *InformationRenderer) Render(doc *TextDocument, extracted *Extracted) (map[string]interface{}, string) {
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
		if len(extracted.Facts) > 0 {
			section(&b, "Facts")
			bulletList(&b, extracted.Facts)
		}
		if len(extracted.Claims) > 0 {
			section(&b, "Claims")
			bulletList(&b, extracted.Claims)
		}
		if len(extracted.Definitions) > 0 {
			section(&b, "Definitions")
			bulletList(&b, extracted.Definitions)
		}
		if len(extracted.KeyPoints) > 0 {
			section(&b, "Key Points")
			bulletList(&b, extracted.KeyPoints)
		}
		if len(extracted.Actions) > 0 {
			section(&b, "Action Items")
			writeActionLines(&b, extracted.Actions)
		}
	}

	b.WriteString(foldContent(doc))
	return fm, b.String()
}
