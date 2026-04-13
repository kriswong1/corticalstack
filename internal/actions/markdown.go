package actions

import (
	"fmt"
	"regexp"
	"strings"
)

// Canonical action line format (GTD Hybrid):
//
//   - [ ] [Owner] Description *(due: 2026-04-18)* #status/next #p/1 #effort/s #ctx/quick <!-- id:abc123 -->
//
// The checkbox state tracks Obsidian-native done/undone, the `#status/*` tag
// carries nuanced state, priority/effort/context are optional Obsidian tags,
// and the HTML comment holds the stable ID.

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

	var tags strings.Builder
	tags.WriteString(fmt.Sprintf("#status/%s", a.Status))
	if a.Priority != "" {
		tags.WriteString(fmt.Sprintf(" #p/%s", strings.TrimPrefix(string(a.Priority), "p")))
	}
	if a.Effort != "" {
		tags.WriteString(fmt.Sprintf(" #effort/%s", a.Effort))
	}
	if a.Context != "" {
		tags.WriteString(fmt.Sprintf(" #ctx/%s", a.Context))
	}

	desc := a.Description
	if a.Title != "" {
		desc = fmt.Sprintf("**%s** — %s", a.Title, a.Description)
	}

	return fmt.Sprintf("- %s [%s] %s%s %s <!-- id:%s -->",
		checkbox, owner, desc, deadline, tags.String(), a.ID)
}

// Parsed holds the fields recovered from a canonical action line.
type Parsed struct {
	ID          string
	Owner       string
	Description string
	Deadline    string
	Status      Status
	Priority    Priority
	Effort      Effort
	Context     string
	Checked     bool // the [x] vs [ ] state
}

// actionLineRe matches the canonical format with generous whitespace handling.
// Captures: check, owner, desc, due (optional), then all tags as a single group, then id.
var actionLineRe = regexp.MustCompile(
	`(?i)^\s*-\s+\[(?P<check>[ xX])\]\s+\[(?P<owner>[^\]]*)\]\s+(?P<desc>.*?)(?:\s+\*\(due:\s*(?P<due>[^)]+)\)\*)?\s+(?P<tags>#\S+(?:\s+#\S+)*)\s+<!--\s*id:(?P<id>[a-f0-9\-]+)\s*-->\s*$`)

// Tag extraction patterns.
var (
	statusTagRe  = regexp.MustCompile(`#status/(\w+)`)
	priorityTagRe = regexp.MustCompile(`#p/(\d)`)
	effortTagRe  = regexp.MustCompile(`#effort/(\w+)`)
	contextTagRe = regexp.MustCompile(`#ctx/(\w[\w-]*)`)
)

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
	}
	ch := strings.TrimSpace(get("check"))
	p.Checked = ch == "x" || ch == "X"

	// Parse tags block.
	tags := get("tags")
	if m := statusTagRe.FindStringSubmatch(tags); m != nil {
		p.Status = MigrateStatus(Status(strings.ToLower(m[1])))
	}
	if !IsValid(string(p.Status)) {
		p.Status = StatusInbox
	}
	if m := priorityTagRe.FindStringSubmatch(tags); m != nil {
		p.Priority = Priority("p" + m[1])
	}
	if m := effortTagRe.FindStringSubmatch(tags); m != nil {
		p.Effort = Effort(m[1])
	}
	if m := contextTagRe.FindStringSubmatch(tags); m != nil {
		p.Context = m[1]
	}

	return p
}

// LineCarriesID reports whether a line contains an action ID marker.
// Used by the reconciler to find candidate lines quickly.
func LineCarriesID(line string) bool {
	return strings.Contains(line, "<!-- id:")
}
