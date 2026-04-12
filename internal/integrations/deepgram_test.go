package integrations

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeepgramConfigured(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"\t\n", false},
		{"abc123", true},
		{" padded-key ", true},
	}
	for _, tt := range tests {
		c := &DeepgramClient{APIKey: tt.key}
		if got := c.Configured(); got != tt.want {
			t.Errorf("Configured(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestDeepgramIDAndName(t *testing.T) {
	c := NewDeepgramClient("key")
	if c.ID() != "deepgram" {
		t.Errorf("ID = %q", c.ID())
	}
	if c.Name() != "Deepgram" {
		t.Errorf("Name = %q", c.Name())
	}
}

func TestDeepgramModelOrDefault(t *testing.T) {
	c := &DeepgramClient{}
	if got := c.modelOrDefault(); got != "nova-3" {
		t.Errorf("default model = %q", got)
	}
	c.Model = "nova-2"
	if got := c.modelOrDefault(); got != "nova-2" {
		t.Errorf("override model = %q", got)
	}
}

func TestDeepgramLangOrDefault(t *testing.T) {
	c := &DeepgramClient{}
	if got := c.langOrDefault(); got != "en" {
		t.Errorf("default lang = %q", got)
	}
	c.Lang = "ja"
	if got := c.langOrDefault(); got != "ja" {
		t.Errorf("override lang = %q", got)
	}
}

func TestTranscriptionResultDurationStr(t *testing.T) {
	tests := []struct {
		d    float64
		want string
	}{
		{0, "00:00:00"},
		{59, "00:00:59"},
		{60, "00:01:00"},
		{3599, "00:59:59"},
		{3600, "01:00:00"},
		{3661.5, "01:01:01"},
	}
	for _, tt := range tests {
		r := &TranscriptionResult{Duration: tt.d}
		if got := r.DurationStr(); got != tt.want {
			t.Errorf("DurationStr(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		s    float64
		want string
	}{
		{0, "00:00:00"},
		{125.7, "00:02:05"},
		{3661, "01:01:01"},
	}
	for _, tt := range tests {
		if got := formatTimestamp(tt.s); got != tt.want {
			t.Errorf("formatTimestamp(%v) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestFormatDiarized(t *testing.T) {
	t.Run("empty returns empty", func(t *testing.T) {
		if got := formatDiarized(nil); got != "" {
			t.Errorf("empty = %q", got)
		}
		if got := formatDiarized([]deepgramUtterance{}); got != "" {
			t.Errorf("empty slice = %q", got)
		}
	})

	t.Run("single utterance", func(t *testing.T) {
		got := formatDiarized([]deepgramUtterance{
			{Speaker: 0, Transcript: "Hello world", Start: 0},
		})
		if !strings.Contains(got, "Speaker 1") {
			t.Errorf("missing 'Speaker 1' (speaker+1): %q", got)
		}
		if !strings.Contains(got, "Hello world") {
			t.Errorf("missing transcript: %q", got)
		}
		if !strings.Contains(got, "00:00:00") {
			t.Errorf("missing timestamp: %q", got)
		}
	})

	t.Run("multiple speakers", func(t *testing.T) {
		got := formatDiarized([]deepgramUtterance{
			{Speaker: 0, Transcript: "Alice speaks", Start: 0},
			{Speaker: 1, Transcript: "Bob responds", Start: 10},
		})
		for _, sub := range []string{"Speaker 1", "Alice speaks", "Speaker 2", "Bob responds", "00:00:00", "00:00:10"} {
			if !strings.Contains(got, sub) {
				t.Errorf("missing %q in %q", sub, got)
			}
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly", 7, "exactly"},
		{"too long for the limit", 5, "too l..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		if got := truncate(tt.in, tt.n); got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

// --- Transcribe HTTP tests ---

func TestTranscribeNotConfigured(t *testing.T) {
	c := &DeepgramClient{APIKey: ""}
	_, err := c.Transcribe([]byte("audio"), "audio/mpeg")
	if err == nil {
		t.Fatal("expected error for unconfigured client")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error text = %q", err.Error())
	}
}

func TestTranscribeHappyPathUtterances(t *testing.T) {
	responseBody := `{
		"metadata": {"duration": 42.5},
		"results": {
			"utterances": [
				{"speaker": 0, "transcript": "Hello world", "start": 0, "end": 1.5},
				{"speaker": 1, "transcript": "How are you", "start": 2, "end": 4}
			]
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request shape.
		if r.Method != "POST" {
			t.Errorf("method = %q", r.Method)
		}
		if r.Header.Get("Authorization") != "Token test-key" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "audio/mp4" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "fake audio" {
			t.Errorf("body = %q", body)
		}
		w.WriteHeader(200)
		w.Write([]byte(responseBody))
	}))
	defer srv.Close()

	c := NewDeepgramClient("test-key")
	c.Endpoint = srv.URL

	result, err := c.Transcribe([]byte("fake audio"), "audio/mp4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Duration != 42.5 {
		t.Errorf("duration = %v", result.Duration)
	}
	for _, sub := range []string{"Speaker 1", "Hello world", "Speaker 2", "How are you"} {
		if !strings.Contains(result.Transcript, sub) {
			t.Errorf("transcript missing %q", sub)
		}
	}
}

func TestTranscribeFallsBackToChannel(t *testing.T) {
	responseBody := `{
		"metadata": {"duration": 3.2},
		"results": {
			"utterances": [],
			"channels": [{"alternatives": [{"transcript": "plain transcript"}]}]
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(responseBody))
	}))
	defer srv.Close()

	c := NewDeepgramClient("k")
	c.Endpoint = srv.URL

	result, err := c.Transcribe([]byte("x"), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Transcript != "plain transcript" {
		t.Errorf("transcript = %q", result.Transcript)
	}
}

func TestTranscribeNon200Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"err":"bad key"}`))
	}))
	defer srv.Close()

	c := NewDeepgramClient("bad")
	c.Endpoint = srv.URL

	_, err := c.Transcribe([]byte("x"), "audio/mpeg")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention 401: %v", err)
	}
}

func TestTranscribeMalformedResponseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	c := NewDeepgramClient("k")
	c.Endpoint = srv.URL

	_, err := c.Transcribe([]byte("x"), "audio/mpeg")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention parse: %v", err)
	}
}

func TestTranscribeEmptyTranscriptErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"metadata":{"duration":1},"results":{"utterances":[],"channels":[]}}`))
	}))
	defer srv.Close()

	c := NewDeepgramClient("k")
	c.Endpoint = srv.URL

	_, err := c.Transcribe([]byte("x"), "audio/mpeg")
	if err == nil {
		t.Fatal("expected error for empty transcript")
	}
	if !strings.Contains(err.Error(), "no transcript") {
		t.Errorf("error text = %v", err)
	}
}
