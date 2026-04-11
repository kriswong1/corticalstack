package persona

import (
	"fmt"
	"strings"
)

// BuildContextPrompt returns a single markdown block to prepend to any
// Claude prompt. It includes every persona file with content, trimmed to
// its per-file budget, in a stable SOUL → USER → MEMORY order.
//
// Files that are missing or empty are silently skipped — Claude calls
// remain functional even if the user has never created the vault files.
func (l *Loader) BuildContextPrompt() string {
	if l == nil {
		return ""
	}

	var sections []string
	for _, name := range AllNames() {
		content := l.getTrimmed(name)
		if !hasBody(content) {
			continue
		}
		heading := fmt.Sprintf("## %s context\n\n%s", strings.ToUpper(string(name)), strings.TrimSpace(content))
		sections = append(sections, heading)
	}

	if len(sections) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Persona context\n\n")
	b.WriteString("*The following context files tailor Claude's output to the user. Respect them unless the user's request contradicts them.*\n\n")
	b.WriteString(strings.Join(sections, "\n\n---\n\n"))
	b.WriteString("\n\n---\n\n")
	return b.String()
}
