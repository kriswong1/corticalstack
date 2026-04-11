package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const deepgramEndpoint = "https://api.deepgram.com/v1/listen"

// DeepgramClient wraps the Deepgram REST API. Configured from
// DEEPGRAM_API_KEY via config.DeepgramAPIKey().
type DeepgramClient struct {
	APIKey string
	Model  string        // defaults to "nova-3"
	Lang   string        // defaults to "en"
	HTTP   *http.Client
	Timeout time.Duration // per-request timeout; defaults to 10 min
}

// NewDeepgramClient builds a Deepgram client with sensible defaults.
func NewDeepgramClient(apiKey string) *DeepgramClient {
	return &DeepgramClient{
		APIKey:  apiKey,
		Model:   "nova-3",
		Lang:    "en",
		HTTP:    &http.Client{},
		Timeout: 10 * time.Minute,
	}
}

// --- Integration interface ---

func (c *DeepgramClient) ID() string   { return "deepgram" }
func (c *DeepgramClient) Name() string { return "Deepgram" }

func (c *DeepgramClient) Configured() bool {
	return strings.TrimSpace(c.APIKey) != ""
}

// HealthCheck hits the Deepgram root with the API key to verify credentials.
// Deepgram doesn't publish a dedicated health endpoint; a cheap GET to the
// projects endpoint returns 200 with a valid key.
func (c *DeepgramClient) HealthCheck() error {
	if !c.Configured() {
		return fmt.Errorf("deepgram: not configured")
	}
	req, _ := http.NewRequest("GET", "https://api.deepgram.com/v1/projects", nil)
	req.Header.Set("Authorization", "Token "+c.APIKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("deepgram health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("deepgram health check: http %d", resp.StatusCode)
	}
	return nil
}

// --- Transcription ---

// TranscriptionResult holds Deepgram's response, normalized.
type TranscriptionResult struct {
	Transcript string
	Duration   float64 // seconds
}

// DurationStr returns the duration as a human-readable HH:MM:SS string.
func (r *TranscriptionResult) DurationStr() string {
	total := int(r.Duration)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// deepgramResponse models the JSON returned by /v1/listen.
type deepgramResponse struct {
	Metadata struct {
		Duration float64 `json:"duration"`
	} `json:"metadata"`
	Results struct {
		Utterances []deepgramUtterance `json:"utterances"`
		Channels   []deepgramChannel   `json:"channels"`
	} `json:"results"`
}

type deepgramUtterance struct {
	Speaker    int     `json:"speaker"`
	Transcript string  `json:"transcript"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
}

type deepgramChannel struct {
	Alternatives []struct {
		Transcript string `json:"transcript"`
	} `json:"alternatives"`
}

// Transcribe sends audio bytes to Deepgram and returns a diarized transcript.
func (c *DeepgramClient) Transcribe(audio []byte, mime string) (*TranscriptionResult, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("deepgram: not configured (set DEEPGRAM_API_KEY)")
	}
	if mime == "" {
		mime = "audio/mpeg"
	}

	params := url.Values{}
	params.Set("model", c.modelOrDefault())
	params.Set("language", c.langOrDefault())
	params.Set("diarize", "true")
	params.Set("smart_format", "true")
	params.Set("punctuate", "true")
	params.Set("utterances", "true")

	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", deepgramEndpoint+"?"+params.Encode(), bytes.NewReader(audio))
	if err != nil {
		return nil, fmt.Errorf("deepgram: build request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.APIKey)
	req.Header.Set("Content-Type", mime)

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepgram api call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("deepgram read response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("deepgram api error (http %d): %s", resp.StatusCode, truncate(string(body), 500))
	}

	var parsed deepgramResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("deepgram parse response: %w", err)
	}

	transcript := formatDiarized(parsed.Results.Utterances)
	if transcript == "" && len(parsed.Results.Channels) > 0 && len(parsed.Results.Channels[0].Alternatives) > 0 {
		transcript = parsed.Results.Channels[0].Alternatives[0].Transcript
	}
	if transcript == "" {
		return nil, fmt.Errorf("deepgram returned no transcript")
	}

	return &TranscriptionResult{
		Transcript: transcript,
		Duration:   parsed.Metadata.Duration,
	}, nil
}

func (c *DeepgramClient) modelOrDefault() string {
	if c.Model != "" {
		return c.Model
	}
	return "nova-3"
}

func (c *DeepgramClient) langOrDefault() string {
	if c.Lang != "" {
		return c.Lang
	}
	return "en"
}

func formatDiarized(utterances []deepgramUtterance) string {
	if len(utterances) == 0 {
		return ""
	}
	var b strings.Builder
	for _, u := range utterances {
		b.WriteString(fmt.Sprintf("%s Speaker %d\n", formatTimestamp(u.Start), u.Speaker+1))
		b.WriteString(u.Transcript)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func formatTimestamp(seconds float64) string {
	total := int(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
