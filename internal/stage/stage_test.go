package stage

import "testing"

func TestAllStages(t *testing.T) {
	cases := map[EntityType]int{
		EntityProduct:   5,
		EntityMeeting:   3,
		EntityDocument:  3,
		EntityPrototype: 3,
	}
	for et, want := range cases {
		got := AllStages(et)
		if len(got) != want {
			t.Errorf("AllStages(%q) len = %d, want %d", et, len(got), want)
		}
	}
	if AllStages("bogus") != nil {
		t.Error("AllStages(bogus) should return nil")
	}
}

func TestValidateAcceptsCanonical(t *testing.T) {
	cases := []struct {
		entity EntityType
		stage  string
	}{
		{EntityProduct, "idea"},
		{EntityProduct, "frame"},
		{EntityProduct, "shape"},
		{EntityProduct, "breadboard"},
		{EntityProduct, "pitch"},
		{EntityMeeting, "transcript"},
		{EntityMeeting, "audio"},
		{EntityMeeting, "note"},
		{EntityDocument, "need"},
		{EntityDocument, "in_progress"},
		{EntityDocument, "final"},
		{EntityPrototype, "need"},
		{EntityPrototype, "in_progress"},
		{EntityPrototype, "final"},
	}
	for _, c := range cases {
		if !Validate(c.entity, c.stage) {
			t.Errorf("Validate(%q, %q) = false", c.entity, c.stage)
		}
	}
}

func TestValidateRejectsLegacyAndBogus(t *testing.T) {
	cases := []struct {
		entity EntityType
		stage  string
	}{
		// Legacy values intentionally fail Validate — they only pass
		// through Normalize. Validate is the strict gate.
		{EntityProduct, "raw"},
		{EntityMeeting, "summary"},
		{EntityPrototype, "draft"},
		{EntityPrototype, "exported"},
		{EntityProduct, ""},
		{EntityDocument, "wip"},
		{EntityProduct, "ship"},
	}
	for _, c := range cases {
		if Validate(c.entity, c.stage) {
			t.Errorf("Validate(%q, %q) = true", c.entity, c.stage)
		}
	}
}

func TestNormalizeLegacyAliases(t *testing.T) {
	cases := []struct {
		entity EntityType
		raw    string
		want   Stage
	}{
		{EntityProduct, "raw", StageIdea},
		{EntityProduct, "RAW", StageIdea},
		{EntityProduct, "  raw  ", StageIdea},
		{EntityMeeting, "summary", StageNote},
		{EntityPrototype, "draft", StageInProgress},
		{EntityPrototype, "exported", StageFinal},
		{EntityPrototype, "Exported", StageFinal},
	}
	for _, c := range cases {
		if got := Normalize(c.entity, c.raw); got != c.want {
			t.Errorf("Normalize(%q, %q) = %q, want %q", c.entity, c.raw, got, c.want)
		}
	}
}

func TestNormalizeFallback(t *testing.T) {
	cases := []struct {
		entity EntityType
		raw    string
		want   Stage
	}{
		{EntityProduct, "", StageIdea},
		{EntityProduct, "garbage", StageIdea},
		{EntityMeeting, "", StageTranscript},
		{EntityDocument, "", StageNeed},
		{EntityDocument, "wip", StageNeed},
		{EntityPrototype, "", StageNeed},
	}
	for _, c := range cases {
		if got := Normalize(c.entity, c.raw); got != c.want {
			t.Errorf("Normalize(%q, %q) = %q, want %q", c.entity, c.raw, got, c.want)
		}
	}
}

func TestNormalizeCanonical(t *testing.T) {
	// Canonical values should round-trip unchanged through Normalize.
	for _, et := range AllEntityTypes() {
		for _, s := range AllStages(et) {
			if got := Normalize(et, string(s)); got != s {
				t.Errorf("Normalize(%q, %q) = %q, want %q", et, s, got, s)
			}
		}
	}
}

func TestParseStrict(t *testing.T) {
	// Empty rejected.
	if _, err := Parse(EntityProduct, ""); err == nil {
		t.Error("Parse with empty stage should error")
	}
	// Unknown entity rejected.
	if _, err := Parse("bogus", "idea"); err == nil {
		t.Error("Parse with unknown entity should error")
	}
	// Bogus value rejected.
	if _, err := Parse(EntityDocument, "wip"); err == nil {
		t.Error("Parse with unknown stage should error")
	}
	// Canonical accepted.
	got, err := Parse(EntityProduct, "frame")
	if err != nil || got != StageFrame {
		t.Errorf("Parse(product, frame) = (%q, %v)", got, err)
	}
	// Legacy alias accepted.
	got, err = Parse(EntityPrototype, "draft")
	if err != nil || got != StageInProgress {
		t.Errorf("Parse(prototype, draft) = (%q, %v)", got, err)
	}
}
