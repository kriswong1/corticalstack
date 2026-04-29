package linear

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Criterion is a parsed acceptance-criteria checkbox from a shaped-prd
// body (§S4.9). Hash is SHA256 of the normalized text — used as the
// sidecar map key per Q8 (additive-only re-runs).
type Criterion struct {
	Hash string // SHA256 hex of strings.ToLower(strings.Join(strings.Fields(text), " "))
	Text string
	Done bool
}

// SlicePlanRow is one row of the §S4.7 / "Slice Plan" section. Only
// L/XL rows produce Project Milestones — smaller features ride along
// inside whichever Issue's criterion they're attached to.
type SlicePlanRow struct {
	Name string
	Size string // "S" / "M" / "L" / "XL"
	// TargetDate is optional. Captured when present; "" otherwise.
	TargetDate string
}

// findSection locates a markdown section by section-token (e.g. "S4.9")
// or by case-insensitive name fragment ("Slice Plan", "Acceptance"). It
// returns the slice of body lines from the line *after* the heading
// through the line *before* the next "## ..." (or higher) heading.
//
// Returns nil if no matching heading is found. Permissive about
// heading depth (## or ### or ####) and about extra prefix tokens
// before the section name.
func findSection(body string, tokens ...string) []string {
	if body == "" || len(tokens) == 0 {
		return nil
	}
	lines := strings.Split(body, "\n")
	headingPrefix := regexp.MustCompile(`^\s{0,3}#{2,6}\s+`)
	tokenLowers := make([]string, len(tokens))
	for i, t := range tokens {
		tokenLowers[i] = strings.ToLower(t)
	}

	startIdx := -1
	for i, line := range lines {
		if !headingPrefix.MatchString(line) {
			continue
		}
		head := strings.ToLower(line)
		matched := false
		for _, t := range tokenLowers {
			if strings.Contains(head, t) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		startIdx = i + 1
		break
	}
	if startIdx < 0 {
		return nil
	}
	endIdx := len(lines)
	for i := startIdx; i < len(lines); i++ {
		if headingPrefix.MatchString(lines[i]) {
			endIdx = i
			break
		}
	}
	return lines[startIdx:endIdx]
}

// checkboxPattern matches markdown task list items at any indentation:
//   - [ ] text     (open)
//   - [x] text     (done)
//   * [X] text     (done, alt bullet/case)
var checkboxPattern = regexp.MustCompile(`(?i)^\s*[-*+]\s*\[(\s|x)\]\s*(.+?)\s*$`)

// ParseAcceptanceCriteria walks the §S4.9 / "Acceptance Criteria"
// section and returns one Criterion per checkbox. Order matches the
// document. Empty result when the section is missing or has no
// checkboxes (which is fine — the L6 generator just creates 0 Issues).
func ParseAcceptanceCriteria(body string) []Criterion {
	section := findSection(body, "S4.9", "Acceptance Criteria")
	if section == nil {
		return nil
	}
	var out []Criterion
	for _, line := range section {
		m := checkboxPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := strings.TrimSpace(m[2])
		if text == "" {
			continue
		}
		out = append(out, Criterion{
			Hash: criterionHash(text),
			Text: text,
			Done: strings.EqualFold(strings.TrimSpace(m[1]), "x"),
		})
	}
	return out
}

// ParseBehaviorExamples returns the full text of the §S4.5 / "Behavior
// Examples" section as a single string. The shapedPRD output is loose
// enough — Gherkin blocks, prose, mixed — that the safest carry-over
// is verbatim text. The L6 generator appends this verbatim to every
// generated Issue's description so the QA-relevant scenarios travel
// with the work item.
//
// Returns "" when the section is absent.
func ParseBehaviorExamples(body string) string {
	section := findSection(body, "S4.5", "Behavior Examples")
	if section == nil {
		return ""
	}
	text := strings.TrimSpace(strings.Join(section, "\n"))
	return text
}

// slicePlanRowPattern matches a "Slice Plan" entry in either of the
// two common shapedPRD shapes:
//
//	- L: Feature Name (target: 2026-05-15)
//	- XL — Feature Name
//	| L | Feature Name | 2026-05-15 |
//
// Permissive about size token (case-insensitive S/M/L/XL).
var slicePlanRowPattern = regexp.MustCompile(`(?i)\b(XS|S|M|L|XL)\b\s*[:|—–-]\s*([^|]+?)(?:\s*[|—–-]\s*(\d{4}-\d{2}-\d{2}))?\s*\|?\s*$`)

// ParseSlicePlan walks the "Slice Plan" / §S4.7 section and returns
// every row whose size token is L or XL. Smaller slices are
// intentionally dropped — Project Milestones in Linear are heavyweight
// enough that creating one per S/M slice creates more clutter than
// signal.
func ParseSlicePlan(body string) []SlicePlanRow {
	section := findSection(body, "Slice Plan", "S4.7")
	if section == nil {
		return nil
	}
	var out []SlicePlanRow
	seen := map[string]bool{}
	for _, line := range section {
		// Skip empty lines and the markdown table header/separator.
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "|---") {
			continue
		}
		// Strip leading bullet/table chars before matching.
		stripped := strings.TrimLeft(trimmed, "-*+| ")
		m := slicePlanRowPattern.FindStringSubmatch(stripped)
		if m == nil {
			continue
		}
		size := strings.ToUpper(m[1])
		if size != "L" && size != "XL" {
			continue
		}
		name := strings.TrimSpace(m[2])
		// Strip a trailing pipe if the row was a table cell.
		name = strings.TrimRight(name, "| ")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, SlicePlanRow{
			Name:       name,
			Size:       size,
			TargetDate: m[3],
		})
	}
	return out
}

// criterionHash computes the sidecar-map key for a criterion. Lowercase
// + collapse whitespace before hashing so trivial reformatting
// (extra spaces, capitalization tweaks) doesn't trigger an "orphan +
// new issue" round per Q8.
func criterionHash(text string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
