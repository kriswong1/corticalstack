package stage

import "testing"

func TestAllStages(t *testing.T) {
	cases := map[EntityType]int{
		EntityProduct:   5,
		EntityMeeting:   3,
		EntityDocument:  2,
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
		{EntityMeeting, "audio"},
		{EntityMeeting, "transcript"},
		{EntityMeeting, "note"},
		{EntityDocument, "input"},
		{EntityDocument, "note"},
		{EntityPrototype, "breadboard"},
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
		{EntityMeeting, "audio", StageAudio},
		{EntityDocument, "need", StageInput},
		{EntityDocument, "in_progress", StageDocNote},
		{EntityDocument, "final", StageDocNote},
		{EntityPrototype, "need", StageProtoBreadboard},
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
		{EntityDocument, "", StageInput},
		{EntityDocument, "wip", StageInput},
		{EntityPrototype, "", StageProtoBreadboard},
	}
	for _, c := range cases {
		if got := Normalize(c.entity, c.raw); got != c.want {
			t.Errorf("Normalize(%q, %q) = %q, want %q", c.entity, c.raw, got, c.want)
		}
	}
}

func TestNormalizeCanonical(t *testing.T) {
	for _, et := range AllEntityTypes() {
		for _, s := range AllStages(et) {
			if got := Normalize(et, string(s)); got != s {
				t.Errorf("Normalize(%q, %q) = %q, want %q", et, s, got, s)
			}
		}
	}
}

func TestParseStrict(t *testing.T) {
	if _, err := Parse(EntityProduct, ""); err == nil {
		t.Error("Parse with empty stage should error")
	}
	if _, err := Parse("bogus", "idea"); err == nil {
		t.Error("Parse with unknown entity should error")
	}
	if _, err := Parse(EntityDocument, "wip"); err == nil {
		t.Error("Parse with unknown stage should error")
	}
	got, err := Parse(EntityProduct, "frame")
	if err != nil || got != StageFrame {
		t.Errorf("Parse(product, frame) = (%q, %v)", got, err)
	}
	got, err = Parse(EntityPrototype, "draft")
	if err != nil || got != StageInProgress {
		t.Errorf("Parse(prototype, draft) = (%q, %v)", got, err)
	}
}
