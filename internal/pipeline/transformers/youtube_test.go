package transformers

import (
	"strings"
	"testing"

	youtube "github.com/kkdai/youtube/v2"
)

func TestFormatHMS(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0, "00:00:00"},
		{1, "00:00:01"},
		{59.9, "00:00:59"},
		{60, "00:01:00"},
		{61.5, "00:01:01"},
		{3599, "00:59:59"},
		{3600, "01:00:00"},
		{3661, "01:01:01"},
		{86399, "23:59:59"},
		{90000, "25:00:00"},
	}
	for _, tt := range tests {
		got := formatHMS(tt.seconds)
		if got != tt.want {
			t.Errorf("formatHMS(%v) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestPickEnglishTrack(t *testing.T) {
	tests := []struct {
		name   string
		tracks []youtube.CaptionTrack
		wantLC string
	}{
		{
			name: "en present",
			tracks: []youtube.CaptionTrack{
				{LanguageCode: "fr"},
				{LanguageCode: "en"},
				{LanguageCode: "ja"},
			},
			wantLC: "en",
		},
		{
			name: "en-US preferred over en-GB",
			tracks: []youtube.CaptionTrack{
				{LanguageCode: "en-US"},
				{LanguageCode: "en-GB"},
			},
			wantLC: "en-US",
		},
		{
			name: "no english falls back to first",
			tracks: []youtube.CaptionTrack{
				{LanguageCode: "fr"},
				{LanguageCode: "ja"},
			},
			wantLC: "fr",
		},
		{
			name: "uppercase EN detected",
			tracks: []youtube.CaptionTrack{
				{LanguageCode: "fr"},
				{LanguageCode: "EN"},
			},
			wantLC: "EN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickEnglishTrack(tt.tracks)
			if got.LanguageCode != tt.wantLC {
				t.Errorf("got %q, want %q", got.LanguageCode, tt.wantLC)
			}
		})
	}
}

func TestParseTimedtextTranscript(t *testing.T) {
	t.Run("valid xml with timestamps", func(t *testing.T) {
		xml := `<?xml version="1.0" encoding="utf-8"?>
<transcript>
  <text start="0" dur="2">Hello world</text>
  <text start="3" dur="2">How are you</text>
  <text start="45" dur="2">Much later</text>
  <text start="60" dur="2">Even later</text>
</transcript>`
		got := parseTimedtextTranscript(xml)
		if !strings.Contains(got, "Hello world") {
			t.Errorf("missing 'Hello world' in output: %q", got)
		}
		if !strings.Contains(got, "How are you") {
			t.Errorf("missing 'How are you' in output: %q", got)
		}
		if !strings.Contains(got, "[00:00:45]") {
			t.Errorf("expected timestamp [00:00:45]: %q", got)
		}
	})

	t.Run("decodes entities", func(t *testing.T) {
		xml := `<transcript><text start="0">Tom &amp; Jerry</text></transcript>`
		got := parseTimedtextTranscript(xml)
		if !strings.Contains(got, "Tom & Jerry") {
			t.Errorf("entities not decoded: %q", got)
		}
	})

	t.Run("invalid xml returns empty", func(t *testing.T) {
		got := parseTimedtextTranscript("not xml at all")
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("no text lines returns empty", func(t *testing.T) {
		got := parseTimedtextTranscript(`<?xml version="1.0"?><transcript></transcript>`)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("skips empty text lines", func(t *testing.T) {
		xml := `<transcript><text start="0"></text><text start="1">real content</text></transcript>`
		got := parseTimedtextTranscript(xml)
		if !strings.Contains(got, "real content") {
			t.Errorf("missing real content: %q", got)
		}
	})
}

func TestYouTubeCanHandle(t *testing.T) {
	tr := &YouTubeTransformer{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/watch?v=abc123", true},
		{"https://youtu.be/abc123", true},
		{"https://youtube.com/shorts/abc123", true},
		{"https://example.com/video", false},
		{"https://vimeo.com/123", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			in := rawInputWithURL(tt.url)
			got := tr.CanHandle(&in)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
