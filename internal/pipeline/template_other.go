package pipeline

import (
	"fmt"
	"strings"
)

// OtherRenderer uses Claude's proposed_structure map to lay out section
// headings dynamically. Falls back to a summary + key points dump if
// Claude didn't supply a structure.
type OtherRenderer struct{}

func (r *OtherRenderer) Name() string { return "other" }

func (r *OtherRenderer) Render(doc *TextDocument, extracted *Extracted) (map[string]interface{}, string) {
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

		if len(extracted.ProposedStructure) > 0 {
			for _, heading := range stableKeys(extracted.ProposedStructure) {
				section(&b, heading)
				bulletList(&b, extracted.ProposedStructure[heading])
			}
		} else {
			if len(extracted.KeyPoints) > 0 {
				section(&b, "Key Points")
				bulletList(&b, extracted.KeyPoints)
			}
			if len(extracted.Ideas) > 0 {
				section(&b, "Ideas")
				bulletList(&b, extracted.Ideas)
			}
		}

		if len(extracted.Actions) > 0 {
			section(&b, "Action Items")
			writeActionLines(&b, extracted.Actions)
		}
	}

	b.WriteString(foldContent(doc))
	return fm, b.String()
}
