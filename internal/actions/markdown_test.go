package actions

import "testing"

func TestFormatParseRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   *Action
	}{
		{
			name: "simple pending",
			in: &Action{
				ID:          "abc12345-1111-2222-3333-444455556666",
				Owner:       "Kris",
				Description: "Review Deepgram pricing tiers",
				Status:      StatusPending,
			},
		},
		{
			name: "done with deadline",
			in: &Action{
				ID:          "def12345-1111-2222-3333-444455556666",
				Owner:       "Claude",
				Description: "Write unit tests",
				Deadline:    "2026-04-18",
				Status:      StatusDone,
			},
		},
		{
			name: "in progress, no deadline",
			in: &Action{
				ID:          "abcdef12-1111-2222-3333-444455556666",
				Owner:       "TBD",
				Description: "Ship the feature",
				Status:      StatusDoing,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			line := FormatLine(c.in)
			parsed := ParseLine(line)
			if parsed == nil {
				t.Fatalf("parse failed for line: %q", line)
			}
			if parsed.ID != c.in.ID {
				t.Errorf("id: got %q want %q", parsed.ID, c.in.ID)
			}
			if parsed.Owner != c.in.Owner {
				t.Errorf("owner: got %q want %q", parsed.Owner, c.in.Owner)
			}
			if parsed.Description != c.in.Description {
				t.Errorf("description: got %q want %q", parsed.Description, c.in.Description)
			}
			if parsed.Deadline != c.in.Deadline {
				t.Errorf("deadline: got %q want %q", parsed.Deadline, c.in.Deadline)
			}
			if parsed.Status != c.in.Status {
				t.Errorf("status: got %q want %q", parsed.Status, c.in.Status)
			}
			wantChecked := c.in.Status == StatusDone
			if parsed.Checked != wantChecked {
				t.Errorf("checked: got %v want %v", parsed.Checked, wantChecked)
			}
		})
	}
}

func TestParseLineIgnoresNonAction(t *testing.T) {
	for _, line := range []string{
		"- just a plain bullet",
		"## A heading",
		"",
		"random text with <!-- id:xxx --> but no checkbox",
	} {
		if ParseLine(line) != nil {
			t.Errorf("expected nil for %q", line)
		}
	}
}
