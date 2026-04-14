package transformers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

func TestVTTName(t *testing.T) {
	tr := &VTTTransformer{}
	if got := tr.Name(); got != "vtt" {
		t.Errorf("Name() = %q, want %q", got, "vtt")
	}
}

func TestVTTCanHandle(t *testing.T) {
	tr := &VTTTransformer{}
	tests := []struct {
		name  string
		input pipeline.RawInput
		want  bool
	}{
		{"filename .vtt", pipeline.RawInput{Kind: pipeline.InputFile, Filename: "meeting.vtt"}, true},
		{"filename .VTT upper", pipeline.RawInput{Kind: pipeline.InputFile, Filename: "MEETING.VTT"}, true},
		{"path .vtt", pipeline.RawInput{Kind: pipeline.InputFile, Path: "/tmp/call.vtt"}, true},
		{"txt rejected", pipeline.RawInput{Kind: pipeline.InputFile, Filename: "notes.txt"}, false},
		{"empty rejected", pipeline.RawInput{Kind: pipeline.InputText}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tr.CanHandle(&tt.input); got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}

const sampleVTT = `WEBVTT
Kind: captions
Language: en

NOTE
This is a header comment that should be ignored.

1
00:00:00.000 --> 00:00:04.500
<v Alice>Good morning everyone, let's start the planning meeting.</v>

2
00:00:04.500 --> 00:00:09.000
<v Bob>Thanks Alice. I wanted to raise the onboarding flow idea.</v>

3
00:00:09.000 --> 00:00:14.750
<v Alice>Great. Let's also discuss the new dashboard widget concept.</v>
`

func TestParseVTTBasic(t *testing.T) {
	res := parseVTT(sampleVTT)

	if res.Text == "" {
		t.Fatal("expected non-empty text")
	}
	if !strings.Contains(res.Text, "Alice: Good morning") {
		t.Errorf("expected Alice prefix in text; got: %s", res.Text)
	}
	if !strings.Contains(res.Text, "Bob: Thanks Alice") {
		t.Errorf("expected Bob prefix; got: %s", res.Text)
	}
	if strings.Contains(res.Text, "WEBVTT") {
		t.Error("WEBVTT header should be stripped")
	}
	if strings.Contains(res.Text, "-->") {
		t.Error("timestamp lines should be stripped")
	}
	if strings.Contains(res.Text, "header comment") {
		t.Error("NOTE blocks should be stripped")
	}
	if strings.Contains(res.Text, "<v") {
		t.Error("voice tags should be stripped")
	}

	if len(res.Speakers) != 2 {
		t.Errorf("expected 2 speakers, got %d: %v", len(res.Speakers), res.Speakers)
	}
	if res.Speakers[0] != "Alice" || res.Speakers[1] != "Bob" {
		t.Errorf("speakers = %v, want [Alice Bob]", res.Speakers)
	}

	if res.Duration != "00:00:14.750" {
		t.Errorf("duration = %q, want %q", res.Duration, "00:00:14.750")
	}
}

func TestParseVTTPlainSpeakerPrefix(t *testing.T) {
	// Teams / Otter style — no <v> tag, just `Speaker: text` after the timestamp.
	input := `WEBVTT

00:00.000 --> 00:05.000
Alice: This is a product idea worth exploring.

00:05.000 --> 00:10.000
Bob: Agreed, add it to the queue.
`
	res := parseVTT(input)
	if !strings.Contains(res.Text, "Alice: This is a product idea") {
		t.Errorf("expected Alice prefix preserved; got: %s", res.Text)
	}
	if !strings.Contains(res.Text, "Bob: Agreed") {
		t.Errorf("expected Bob prefix preserved; got: %s", res.Text)
	}
	if len(res.Speakers) != 2 {
		t.Errorf("expected 2 speakers, got %v", res.Speakers)
	}
}

func TestParseVTTCueIdentifierStripped(t *testing.T) {
	input := `WEBVTT

intro-cue
00:00.000 --> 00:05.000
First line of spoken content.

next-cue
00:05.000 --> 00:10.000
Second line of spoken content.
`
	res := parseVTT(input)
	if strings.Contains(res.Text, "intro-cue") || strings.Contains(res.Text, "next-cue") {
		t.Errorf("cue identifiers should be stripped; got: %s", res.Text)
	}
	if !strings.Contains(res.Text, "First line of spoken content") {
		t.Errorf("cue text missing; got: %s", res.Text)
	}
}

func TestParseVTTMultiLineCue(t *testing.T) {
	input := `WEBVTT

00:00.000 --> 00:10.000
<v Alice>First sentence from Alice.
Second sentence continues from the same speaker.</v>
`
	res := parseVTT(input)
	// Both lines should appear; the first is prefixed, the second isn't.
	if !strings.Contains(res.Text, "Alice: First sentence from Alice") {
		t.Errorf("first line missing prefix; got: %s", res.Text)
	}
	if !strings.Contains(res.Text, "Second sentence continues") {
		t.Errorf("continuation line missing; got: %s", res.Text)
	}
	if strings.Count(res.Text, "Alice:") != 1 {
		t.Errorf("Alice should be prefixed exactly once, got: %s", res.Text)
	}
}

func TestParseVTTStyleTagsStripped(t *testing.T) {
	input := `WEBVTT

00:00.000 --> 00:05.000
<v Alice>We should build <c.feature>a new dashboard</c> tomorrow.</v>
`
	res := parseVTT(input)
	if strings.Contains(res.Text, "<c") || strings.Contains(res.Text, "</c>") {
		t.Errorf("style tags should be stripped; got: %s", res.Text)
	}
	if !strings.Contains(res.Text, "a new dashboard") {
		t.Errorf("inner text should survive; got: %s", res.Text)
	}
}

func TestParseVTTHeaderOnlyReturnsEmpty(t *testing.T) {
	input := `WEBVTT
Kind: captions
`
	res := parseVTT(input)
	if res.Text != "" {
		t.Errorf("expected empty text for header-only input; got: %s", res.Text)
	}
}

func TestVTTTransformFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "planning.vtt")
	if err := os.WriteFile(path, []byte(sampleVTT), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	tr := &VTTTransformer{}
	doc, err := tr.Transform(&pipeline.RawInput{Kind: pipeline.InputFile, Path: path, Filename: "planning.vtt"})
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if doc.Source != "vtt" {
		t.Errorf("Source = %q, want vtt", doc.Source)
	}
	if doc.Title != "planning" {
		t.Errorf("Title = %q, want planning", doc.Title)
	}
	if len(doc.Authors) != 2 {
		t.Errorf("Authors = %v, want 2", doc.Authors)
	}
	if doc.Metadata["duration"] == "" {
		t.Error("duration metadata missing")
	}
	if doc.Metadata["input_file"] != path {
		t.Errorf("input_file metadata = %q, want %q", doc.Metadata["input_file"], path)
	}
	if !strings.Contains(doc.Content, "Alice:") {
		t.Errorf("content missing speaker prefix; got: %s", doc.Content)
	}
}

func TestVTTTransformEmptyErrors(t *testing.T) {
	tr := &VTTTransformer{}
	_, err := tr.Transform(&pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("")})
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestVTTTransformHeaderOnlyErrors(t *testing.T) {
	tr := &VTTTransformer{}
	_, err := tr.Transform(&pipeline.RawInput{
		Kind:     pipeline.InputFile,
		Filename: "empty.vtt",
		Content:  []byte("WEBVTT\n\nNOTE only a comment\n"),
	})
	if err == nil {
		t.Error("expected error for header-only input")
	}
}

// TestParseVTTCueStartingWithNOTE — MD-01 regression.
// A cue whose voice-span body starts with the literal word "NOTE" must be
// preserved as cue text, not silently swallowed as a comment block.
func TestParseVTTCueStartingWithNOTE(t *testing.T) {
	input := `WEBVTT

00:00.000 --> 00:05.000
<v Alice>NOTE this is important</v>
`
	res := parseVTT(input)
	if !strings.Contains(res.Text, "Alice: NOTE this is important") {
		t.Errorf("expected cue text preserved; got: %q", res.Text)
	}
	if len(res.Speakers) != 1 || res.Speakers[0] != "Alice" {
		t.Errorf("expected [Alice] speaker, got %v", res.Speakers)
	}
}

// TestParseVTTCueContainingNOTE — MD-01 regression.
// A multi-line cue whose second line starts with "NOTE" must keep both
// lines as cue text; the NOTE-block guard only applies between cues.
func TestParseVTTCueContainingNOTE(t *testing.T) {
	input := `WEBVTT

00:00.000 --> 00:10.000
<v Alice>First sentence from Alice.
NOTE the second point is critical.</v>
`
	res := parseVTT(input)
	if !strings.Contains(res.Text, "Alice: First sentence from Alice") {
		t.Errorf("first cue line missing; got: %q", res.Text)
	}
	if !strings.Contains(res.Text, "NOTE the second point is critical") {
		t.Errorf("second cue line swallowed by NOTE guard; got: %q", res.Text)
	}
}

// TestParseVTTBoundaryNotes — MD-01 regression.
// Words like "NOTES:", "NOTEBOOK", "NOTE-" share the "NOTE" prefix but are
// not comment markers per the spec. They must pass through the cue-text
// extraction path.
func TestParseVTTBoundaryNotes(t *testing.T) {
	input := `WEBVTT

00:00.000 --> 00:05.000
<v Alice>NOTES: meeting starts now.</v>

00:05.000 --> 00:10.000
<v Bob>NOTEBOOK is open on page three.</v>

00:10.000 --> 00:15.000
<v Carol>NOTE-taking is allowed.</v>
`
	res := parseVTT(input)
	for _, want := range []string{
		"NOTES: meeting starts now",
		"NOTEBOOK is open on page three",
		"NOTE-taking is allowed",
	} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("expected %q preserved; got: %q", want, res.Text)
		}
	}
}

// TestParseVTTValidNOTEBetweenCues — MD-01 regression.
// A legit WebVTT NOTE block between two cues (at the top level) must still
// be stripped. This proves the fix didn't break the intended comment path.
func TestParseVTTValidNOTEBetweenCues(t *testing.T) {
	input := `WEBVTT

00:00.000 --> 00:05.000
<v Alice>First cue.</v>

NOTE this is a real comment between cues

00:05.000 --> 00:10.000
<v Bob>Second cue.</v>
`
	res := parseVTT(input)
	if strings.Contains(res.Text, "real comment between cues") {
		t.Errorf("NOTE block between cues should be stripped; got: %q", res.Text)
	}
	if !strings.Contains(res.Text, "Alice: First cue") {
		t.Errorf("first cue missing; got: %q", res.Text)
	}
	if !strings.Contains(res.Text, "Bob: Second cue") {
		t.Errorf("second cue missing; got: %q", res.Text)
	}
}

// TestParseVTTBareNOTEBetweenCues — MD-01 regression.
// The spec allows the bare word "NOTE" on a line by itself to introduce
// a multi-line comment body that runs until the next blank line.
func TestParseVTTBareNOTEBetweenCues(t *testing.T) {
	input := `WEBVTT

00:00.000 --> 00:05.000
<v Alice>First cue.</v>

NOTE
multi-line
comment body

00:05.000 --> 00:10.000
<v Bob>Second cue.</v>
`
	res := parseVTT(input)
	if strings.Contains(res.Text, "multi-line") || strings.Contains(res.Text, "comment body") {
		t.Errorf("bare-NOTE block body should be stripped; got: %q", res.Text)
	}
	if !strings.Contains(res.Text, "Alice: First cue") || !strings.Contains(res.Text, "Bob: Second cue") {
		t.Errorf("surrounding cues missing; got: %q", res.Text)
	}
}
