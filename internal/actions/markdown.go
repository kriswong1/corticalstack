package actions

import (
	"fmt"
	"regexp"
	"strings"
)

// Canonical action line format:
//
//   - [ ] [Owner] Description *(due: deadline)* #status/pending <!-- id:abc123 -->
//
// The checkbox state tracks Obsidian-native done/undone, the `#status/*` tag
// carries nuanced state, and the HTML comment holds the stable ID.

// FormatLine returns the canonical markdown line for an action.
func FormatLine(a *Action) string {
	owner := a.Owner
	if owner == "" {
		owner = "TBD"
	}
	checkbox := "[ ]"
	if a.Status == StatusDone {
		checkbox = "[x]"
	}
	deadline := ""
	if a.Deadline != "" {
		deadline = fmt.Sprintf(" *(due: %s)*", a.Deadline)
	}
	return fmt.Sprintf("- %s [%s] %s%s #status/%s <!-- id:%s -->",
		checkbox, owner, a.Description, deadline, a.Status, a.ID)
}

// Parsed holds the fields recovered from a canonical action line.
type Parsed struct {
	ID          string
	Owner       string
	Description string
	Deadline    string
	Status      Status
	Checked     bool // the [x] vs [ ] state
}

// actionLineRe matches our canonical format with generous whitespace handling.
var actionLineRe = regexp.MustCompile(
	`(?i)^\s*-\s+\[(?P<check>[ xX])\]\s+\[(?P<owner>[^\]]*)\]\s+(?P<desc>.*?)(?:\s+\*\(due:\s*(?P<due>[^)]+)\)\*)?\s+#status/(?P<status>[a-z]+)\s+<!--\s*id:(?P<id>[a-f0-9\-]+)\s*-->\s*$`)

// ParseLine tries to decode one line of markdown. Returns nil if the line
// isn't a CorticalStack action line.
func ParseLine(line string) *Parsed {
	match := actionLineRe.FindStringSubmatch(line)
	if match == nil {
		return nil
	}
	names := actionLineRe.SubexpNames()
	get := func(name string) string {
		for i, n := range names {
			if n == name {
				return match[i]
			}
		}
		return ""
	}

	p := &Parsed{
		ID:          strings.TrimSpace(get("id")),
		Owner:       strings.TrimSpace(get("owner")),
		Description: strings.TrimSpace(get("desc")),
		Deadline:    strings.TrimSpace(get("due")),
		Status:      Status(strings.ToLower(strings.TrimSpace(get("status")))),
	}
	ch := strings.TrimSpace(get("check"))
	p.Checked = ch == "x" || ch == "X"
	if !IsValid(string(p.Status)) {
		p.Status = StatusPending
	}
	return p
}

// LineCarriesID reports whether a line contains an action ID marker.
// Used by the reconciler to find candidate lines quickly.
func LineCarriesID(line string) bool {
	return strings.Contains(line, "<!-- id:")
}
