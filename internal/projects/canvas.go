package projects

import (
	"strings"
)

// canvasHeader is the section heading the user-editable region opens
// with. The writer composes header + canvas + footer; everything between
// the canvas heading and the next deterministic section round-trips
// untouched.
const canvasHeader = "## Canvas"

// canvasFooterStart is the heading where the deterministic footer
// begins. Everything from this heading onward in the manifest body is
// regenerated on every write.
const canvasFooterStart = "## Notes"

// extractCanvas pulls the `## Canvas` section out of a manifest body.
// Returns the raw text (between the canvas heading and the next
// deterministic section), trimmed of leading/trailing blank lines but
// preserving internal formatting. Returns empty string when no canvas
// section exists.
//
// The parser is line-based and tolerant: a manifest written by an older
// build (no canvas section) returns "" so the writer can insert one on
// the next save without disturbing other content.
func extractCanvas(body string) string {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == canvasHeader {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		// Stop at the next deterministic section (## Notes / ## Action items)
		// or any other top-level heading the writer might add later.
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return strings.Trim(strings.Join(lines[start:end], "\n"), "\n ")
}

// composeBody renders the manifest body in split-mode:
//
//	# Name
//	[description]
//	## Canvas
//	[user content — round-tripped]
//	## Notes
//	[deterministic backlink prose]
//	## Action items
//	[deterministic backlink prose]
//
// canvas may be empty; the section heading is still emitted so users
// see where to type in Obsidian. The canvas region is the only place
// the Project's manifest body is user-editable through the app or by
// hand — everything else is regenerated on every write.
func composeBody(p *Project, canvas string) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(p.Name)
	b.WriteString("\n\n")
	if p.Description != "" {
		b.WriteString(p.Description)
		b.WriteString("\n\n")
	}

	b.WriteString(canvasHeader)
	b.WriteString("\n\n")
	canvas = strings.Trim(canvas, "\n ")
	if canvas != "" {
		b.WriteString(canvas)
		b.WriteString("\n\n")
	}

	b.WriteString("## Notes\n\n> CorticalStack notes tagged with this project will backlink here via frontmatter `projects:`.\n\n")
	b.WriteString("## Action items\n\n> See [[")
	b.WriteString("projects/" + p.Slug + "/ACTION-ITEMS")
	b.WriteString("]].\n")

	return b.String()
}
