package transformers

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// VTTTransformer parses WebVTT meeting transcripts (Zoom, Teams, Google Meet,
// manual exports) into clean plaintext with speaker prefixes preserved.
// It strips the WEBVTT header, NOTE blocks, cue identifiers, timestamp lines,
// and inline styling markup so the downstream Claude extractor sees only the
// spoken content.
type VTTTransformer struct{}

func (t *VTTTransformer) Name() string { return "vtt" }

func (t *VTTTransformer) CanHandle(input *pipeline.RawInput) bool {
	ext := strings.ToLower(filepath.Ext(input.Path))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(input.Filename))
	}
	return ext == ".vtt"
}

// maxVTTRawBytes caps the raw input size of a single VTT transform so a
// pathological 200MB upload (the current MaxUploadBytes default) doesn't
// trigger 3x peak memory via ReplaceAll + Split allocations. 16MB is
// ~6000 hours of single-speaker dialogue at 40 chars/sec — far beyond any
// legitimate transcript — so the cap is effectively "you have a bug" not
// "you have a long meeting". LO-01.
const maxVTTRawBytes = 16 * 1024 * 1024

func (t *VTTTransformer) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, error) {
	raw, err := readInputBytes(input)
	if err != nil {
		return nil, fmt.Errorf("vtt: %w", err)
	}
	if raw == "" {
		return nil, fmt.Errorf("vtt: empty input")
	}
	if len(raw) > maxVTTRawBytes {
		return nil, fmt.Errorf("vtt: input exceeds %d byte cap (got %d); stream-split large transcripts before ingest",
			maxVTTRawBytes, len(raw))
	}

	parsed := parseVTT(raw)
	if parsed.Text == "" {
		return nil, fmt.Errorf("vtt: no spoken content found (only headers / comments?)")
	}

	title := input.Title
	if title == "" {
		name := input.Filename
		if name == "" {
			name = filepath.Base(input.Path)
		}
		title = strings.TrimSuffix(name, filepath.Ext(name))
	}

	meta := map[string]string{
		"input_kind": string(input.Kind),
	}
	if input.Path != "" {
		meta["input_file"] = input.Path
	}
	if parsed.Duration != "" {
		meta["duration"] = parsed.Duration
	}

	return &pipeline.TextDocument{
		ID:       identifierFor(input),
		Source:   "vtt",
		Title:    title,
		Date:     fileModTime(input.Path),
		Authors:  parsed.Speakers,
		Content:  parsed.Text,
		Metadata: mergeMeta(input.Metadata, meta),
	}, nil
}

// vttParseResult bundles the text, speaker list, and duration parsed
// from a WebVTT file.
type vttParseResult struct {
	Text     string
	Speakers []string
	Duration string
}

var (
	// vttTimestampRe matches a cue timing line, anchored at start of line.
	// Format: HH:MM:SS.mmm --> HH:MM:SS.mmm [settings]. The hours component
	// is optional per spec (MM:SS.mmm --> MM:SS.mmm is also valid).
	vttTimestampRe = regexp.MustCompile(`^\s*(?:\d+:)?\d{2}:\d{2}\.\d{3}\s*-->\s*(?:\d+:)?\d{2}:\d{2}\.\d{3}`)

	// vttEndTimeRe captures the end timestamp of a cue so we can report the
	// last cue's end as the transcript duration.
	vttEndTimeRe = regexp.MustCompile(`-->\s*((?:\d+:)?\d{2}:\d{2}\.\d{3})`)

	// vttVoiceOpenRe matches an opening voice span like `<v Speaker Name>`
	// or `<v.class Speaker>`. Captures the speaker name.
	vttVoiceOpenRe = regexp.MustCompile(`<v(?:\.[^>\s]+)?\s+([^>]+)>`)

	// vttAnyTagRe matches any remaining VTT markup tag (styling, class, etc.)
	// after the voice-tag substitution has run.
	vttAnyTagRe = regexp.MustCompile(`<[^>]*>`)

	// vttPlainSpeakerRe matches `Speaker Name: text` at the start of a line.
	// Tools like Teams and Otter emit this style instead of <v> tags.
	vttPlainSpeakerRe = regexp.MustCompile(`^([A-Z][A-Za-z0-9 .'\-]{0,40}):\s`)
)

// parseVTT is a line-oriented state machine that walks a WebVTT file and
// produces joined text with speaker prefixes preserved.
func parseVTT(raw string) vttParseResult {
	// Normalize CRLF.
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")

	var (
		out          strings.Builder
		speakers     []string
		seenSpeakers = map[string]bool{}
		lastSpeaker  string
		lastDuration string
		inHeader     = true // WEBVTT header block runs until the first blank line.
		inNote       bool
		inCue        bool
	)

	addSpeaker := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seenSpeakers[name] {
			return
		}
		seenSpeakers[name] = true
		speakers = append(speakers, name)
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// The WEBVTT header block is everything from the first line
		// (which must be `WEBVTT` per spec) up to the first blank line.
		// Optional header metadata like `Kind: captions` or `Language: en`
		// lives here and must not be parsed as cue content.
		if inHeader {
			if trimmed == "" {
				inHeader = false
				continue
			}
			if i == 0 || strings.HasPrefix(trimmed, "WEBVTT") ||
				strings.Contains(trimmed, ":") && !vttTimestampRe.MatchString(trimmed) {
				continue
			}
			// First non-header-looking line without a preceding blank —
			// treat as the start of the body.
			inHeader = false
		}

		// NOTE blocks run until the next blank line.
		if inNote {
			if trimmed == "" {
				inNote = false
			}
			continue
		}
		// Per the WebVTT spec, a NOTE block can only appear between cues,
		// never inside one. A NOTE line is either the bare word "NOTE" on a
		// line by itself (followed by a multi-line comment body terminated by
		// a blank line) or "NOTE" followed by whitespace and inline comment
		// text. Lines starting with "NOTES", "NOTEBOOK", "NOTE-" etc. are
		// not comment markers — they're ordinary cue text and must pass
		// through the cue-text extraction path below.
		if !inCue && isVTTNoteLine(trimmed) {
			inNote = true
			continue
		}

		// Blank line ends the current cue. We don't emit a newline here —
		// each cue gets its own joined line in the output.
		if trimmed == "" {
			inCue = false
			continue
		}

		// Timestamp line: record duration and start a new cue.
		if vttTimestampRe.MatchString(trimmed) {
			if m := vttEndTimeRe.FindStringSubmatch(trimmed); len(m) == 2 {
				lastDuration = m[1]
			}
			inCue = true
			continue
		}

		// Cue identifier: line immediately before a timestamp that isn't text.
		// We detect it heuristically — if we're not currently inside a cue
		// and the next non-empty line is a timestamp, this is an identifier
		// and can be skipped.
		if !inCue && isLikelyCueIdentifier(lines, i) {
			continue
		}

		// Cue text: extract speaker from <v> tag or "Speaker:" prefix,
		// strip remaining markup, and emit.
		text, speaker := extractVTTCueText(trimmed)
		if speaker != "" {
			addSpeaker(speaker)
			lastSpeaker = speaker
		}
		if text == "" {
			continue
		}

		if speaker != "" {
			out.WriteString(speaker)
			out.WriteString(": ")
			out.WriteString(text)
		} else if lastSpeaker != "" && inCue {
			// Continuation of the previous speaker's cue (multi-line).
			// Emit without re-prefixing to avoid duplicate labels.
			out.WriteString(text)
		} else {
			out.WriteString(text)
		}
		out.WriteString("\n")
	}

	return vttParseResult{
		Text:     strings.TrimSpace(out.String()),
		Speakers: speakers,
		Duration: lastDuration,
	}
}

// extractVTTCueText pulls the speaker name and plaintext out of a cue line.
// It handles `<v Speaker>...</v>` voice tags and `Speaker: text` prefixes.
func extractVTTCueText(line string) (text, speaker string) {
	// Voice tag form: <v Speaker Name>body</v>
	if m := vttVoiceOpenRe.FindStringSubmatch(line); len(m) == 2 {
		speaker = strings.TrimSpace(m[1])
		line = vttVoiceOpenRe.ReplaceAllString(line, "")
	}

	// Strip any remaining markup.
	line = vttAnyTagRe.ReplaceAllString(line, "")
	line = strings.TrimSpace(line)

	// Fall back to `Speaker: body` prefix if we didn't already find one.
	if speaker == "" {
		if m := vttPlainSpeakerRe.FindStringSubmatch(line); len(m) == 2 {
			speaker = strings.TrimSpace(m[1])
			line = strings.TrimSpace(strings.TrimPrefix(line, m[0]))
		}
	}
	return line, speaker
}

// isVTTNoteLine reports whether a line is the start of a WebVTT NOTE
// comment block. Per the spec (https://www.w3.org/TR/webvtt1/#webvtt-comment-block)
// a NOTE line is either the bare word "NOTE" on a line by itself (followed
// by a multi-line comment body up to the next blank line) or "NOTE"
// followed by whitespace and inline comment text. Any other prefix
// ("NOTES", "NOTEBOOK", "NOTE-", "NOTE:", etc.) is not a comment marker
// and must be treated as cue text by the caller.
//
// Callers must also gate on `!inCue` before calling this — the spec
// restricts NOTE blocks to the top level, between cues.
func isVTTNoteLine(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "NOTE") {
		return false
	}
	if len(trimmed) == 4 {
		// Bare "NOTE" on a line by itself begins a multi-line comment body.
		return true
	}
	// "NOTE " or "NOTE\t" begins an inline comment.
	next := trimmed[4]
	return next == ' ' || next == '\t'
}

// isLikelyCueIdentifier returns true if line i looks like a cue identifier:
// a single non-empty line whose following non-empty line is a timestamp.
func isLikelyCueIdentifier(lines []string, i int) bool {
	// Look ahead to the next non-empty line.
	for j := i + 1; j < len(lines); j++ {
		next := strings.TrimSpace(lines[j])
		if next == "" {
			continue
		}
		return vttTimestampRe.MatchString(next)
	}
	return false
}

