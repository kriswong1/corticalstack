package linear

import "testing"

const samplePRDBody = `# Some Project — shaped PRD

## Pitch

### Problem

Users can't see workflow state changes until they refresh.

### Appetite

2 weeks.

## §S4.5 Behavior Examples

Scenario: connected mode
  Given the websocket is open
  Then state changes appear within 1s

Scenario: disconnected
  Given the connection drops
  Then we fall back to a 30s poll

## §S4.7 Slice Plan

| Size | Slice                    | Target     |
|------|--------------------------|------------|
| L    | Realtime websocket layer | 2026-05-15 |
| S    | Polling fallback         |            |
| XL   | Multi-tenant fan-out     | 2026-06-01 |

## §S4.9 Acceptance Criteria

- [ ] State changes propagate within 1s when connected
- [x] Fallback to 30s poll on disconnect
- [ ] Multi-tenant fan-out preserves per-tenant ordering
- [ ]    Whitespace before checkbox text is trimmed

## §S5 Risks

(unrelated section, parser must skip)
- [ ] Not a criterion (out of S4.9)
`

func TestParseAcceptanceCriteria(t *testing.T) {
	got := ParseAcceptanceCriteria(samplePRDBody)
	if len(got) != 4 {
		t.Fatalf("want 4 criteria, got %d: %+v", len(got), got)
	}
	if got[0].Done {
		t.Errorf("first criterion should be open")
	}
	if !got[1].Done {
		t.Errorf("second criterion should be done")
	}
	for i, c := range got {
		if c.Hash == "" {
			t.Errorf("criterion %d has empty hash", i)
		}
	}
	// Confirm that a parser run picks up criteria from S4.9 only —
	// the "(out of S4.9)" line in §S5 must not leak in.
	for _, c := range got {
		if c.Text == "Not a criterion (out of S4.9)" {
			t.Errorf("S5 leaked into S4.9 result")
		}
	}
}

func TestParseSlicePlan(t *testing.T) {
	got := ParseSlicePlan(samplePRDBody)
	// Only L and XL rows count; the S row is filtered.
	if len(got) != 2 {
		t.Fatalf("want 2 L/XL slices, got %d: %+v", len(got), got)
	}
	if got[0].Size != "L" {
		t.Errorf("first slice size: want L, got %q", got[0].Size)
	}
	if got[0].Name != "Realtime websocket layer" {
		t.Errorf("first slice name: want %q, got %q", "Realtime websocket layer", got[0].Name)
	}
	if got[0].TargetDate != "2026-05-15" {
		t.Errorf("first slice target: want 2026-05-15, got %q", got[0].TargetDate)
	}
	if got[1].Size != "XL" {
		t.Errorf("second slice size: want XL, got %q", got[1].Size)
	}
}

func TestParseBehaviorExamples(t *testing.T) {
	got := ParseBehaviorExamples(samplePRDBody)
	if got == "" {
		t.Fatal("expected non-empty examples block")
	}
	if !contains(got, "Scenario: connected mode") {
		t.Errorf("expected 'Scenario: connected mode' in output, got: %q", got)
	}
	if contains(got, "Multi-tenant fan-out") {
		t.Errorf("S4.5 result leaked S4.7 content")
	}
}

func TestCriterionHashStability(t *testing.T) {
	a := criterionHash("State changes propagate within 1s when connected")
	b := criterionHash("  state CHANGES  propagate WITHIN 1s when CONNECTED  ")
	if a != b {
		t.Errorf("normalization mismatch: %q != %q", a, b)
	}
	c := criterionHash("State changes propagate within 2s when connected")
	if a == c {
		t.Errorf("different texts hashed identically")
	}
}

func contains(s, sub string) bool { return indexFold(s, sub) >= 0 }
