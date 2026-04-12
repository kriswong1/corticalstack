package shapeup

import "testing"

func TestAllStages(t *testing.T) {
	got := AllStages()
	want := []Stage{StageRaw, StageFrame, StageShape, StageBreadboard, StagePitch}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestNextStage(t *testing.T) {
	tests := []struct {
		in   Stage
		want Stage
	}{
		{StageRaw, StageFrame},
		{StageFrame, StageShape},
		{StageShape, StageBreadboard},
		{StageBreadboard, StagePitch},
		{StagePitch, ""}, // last stage returns empty
		{"unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := NextStage(tt.in)
		if got != tt.want {
			t.Errorf("NextStage(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsValidStage(t *testing.T) {
	valid := []string{"raw", "frame", "shape", "breadboard", "pitch"}
	for _, v := range valid {
		if !IsValidStage(v) {
			t.Errorf("IsValidStage(%q) = false, want true", v)
		}
	}
	invalid := []string{"", "RAW", "bogus", "frames", "draft"}
	for _, v := range invalid {
		if IsValidStage(v) {
			t.Errorf("IsValidStage(%q) = true, want false", v)
		}
	}
}
