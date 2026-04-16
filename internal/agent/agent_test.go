package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseStreamAssistantText(t *testing.T) {
	input := `{"type":"system","session_id":"sess-1"}
{"type":"assistant","content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}]}
`
	got := parseStream(strings.NewReader(input), nil)
	if got.SessionID != "sess-1" {
		t.Errorf("session = %q", got.SessionID)
	}
	if got.Text != "Hello world" {
		t.Errorf("text = %q", got.Text)
	}
}

func TestParseStreamResultString(t *testing.T) {
	input := `{"type":"system","session_id":"sess-2"}
{"type":"assistant","content":[{"type":"text","text":"intermediate"}]}
{"type":"result","result":"final answer","total_cost_usd":0.0042}
`
	got := parseStream(strings.NewReader(input), nil)
	if got.Text != "final answer" {
		t.Errorf("text = %q, want 'final answer'", got.Text)
	}
	if got.CostUSD != 0.0042 {
		t.Errorf("cost = %v", got.CostUSD)
	}
	if got.SessionID != "sess-2" {
		t.Errorf("session = %q", got.SessionID)
	}
}

func TestParseStreamResultPayloadObject(t *testing.T) {
	input := `{"type":"result","result":{"text":"object form","session_id":"sess-3","cost_usd":0.01}}
`
	got := parseStream(strings.NewReader(input), nil)
	if got.Text != "object form" {
		t.Errorf("text = %q", got.Text)
	}
	if got.SessionID != "sess-3" {
		t.Errorf("session = %q", got.SessionID)
	}
	if got.CostUSD != 0.01 {
		t.Errorf("cost = %v", got.CostUSD)
	}
}

func TestParseStreamFallsBackToAssistantText(t *testing.T) {
	// No result event — should fall back to assistant text parts.
	input := `{"type":"assistant","content":[{"type":"text","text":"fallback "},{"type":"text","text":"joined"}]}
`
	got := parseStream(strings.NewReader(input), nil)
	if got.Text != "fallback joined" {
		t.Errorf("text = %q", got.Text)
	}
}

func TestParseStreamSkipsBlankLines(t *testing.T) {
	input := `

{"type":"system","session_id":"sess-4"}

{"type":"result","result":"ok"}
`
	got := parseStream(strings.NewReader(input), nil)
	if got.Text != "ok" {
		t.Errorf("text = %q", got.Text)
	}
}

func TestParseStreamSkipsMalformedJSON(t *testing.T) {
	input := `not json at all
{"type":"system","session_id":"sess-5"}
also garbage
{"type":"result","result":"survived"}
`
	got := parseStream(strings.NewReader(input), nil)
	if got.Text != "survived" {
		t.Errorf("text = %q", got.Text)
	}
	if got.SessionID != "sess-5" {
		t.Errorf("session = %q", got.SessionID)
	}
}

func TestParseStreamIgnoresNonTextBlocks(t *testing.T) {
	// Only text blocks should accumulate; tool_use or other types skipped.
	input := `{"type":"assistant","content":[{"type":"tool_use"},{"type":"text","text":"kept"}]}
`
	got := parseStream(strings.NewReader(input), nil)
	if got.Text != "kept" {
		t.Errorf("text = %q", got.Text)
	}
}

func TestParseStreamEmptyInput(t *testing.T) {
	got := parseStream(strings.NewReader(""), nil)
	if got.Text != "" {
		t.Errorf("text = %q, expected empty", got.Text)
	}
}

func TestParseStreamEmptyTextBlockSkipped(t *testing.T) {
	// Empty text blocks inside assistant content should be skipped.
	input := `{"type":"assistant","content":[{"type":"text","text":""},{"type":"text","text":"real"}]}
`
	got := parseStream(strings.NewReader(input), nil)
	if got.Text != "real" {
		t.Errorf("text = %q", got.Text)
	}
}

func TestIsInstalled(t *testing.T) {
	// Just verify IsInstalled returns a bool without panicking.
	// We don't assert true/false since claude may or may not be installed.
	got := IsInstalled()
	_ = got // use the value to avoid any linter complaint
}

// Real CLI shape: assistant content + usage are nested under .message.
// The earlier flat-shape tests are kept for the legacy fallback path.
func TestParseStreamCapturesUsageNested(t *testing.T) {
	input := `{"type":"system","subtype":"init","session_id":"abc-123","model":"claude-sonnet-4-5","cwd":"/tmp","tools":["Read"]}
{"type":"assistant","message":{"id":"msg_01XYZ","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}],"stop_reason":"end_turn","usage":{"input_tokens":12,"cache_creation_input_tokens":1024,"cache_read_input_tokens":2048,"output_tokens":7,"service_tier":"standard"}},"session_id":"abc-123"}
{"type":"result","subtype":"success","is_error":false,"duration_ms":1820,"duration_api_ms":1654,"num_turns":1,"result":"Hello world","session_id":"abc-123","total_cost_usd":0.00345,"usage":{"input_tokens":12,"cache_creation_input_tokens":1024,"cache_read_input_tokens":2048,"output_tokens":7,"server_tool_use":{"web_search_requests":0}}}
`
	got := parseStream(strings.NewReader(input), nil)

	if got.Model != "claude-sonnet-4-5" {
		t.Errorf("model = %q", got.Model)
	}
	if got.SessionID != "abc-123" {
		t.Errorf("session = %q", got.SessionID)
	}
	if got.Text != "Hello world" {
		t.Errorf("text = %q", got.Text)
	}
	if got.InputTokens != 12 {
		t.Errorf("input_tokens = %d, want 12", got.InputTokens)
	}
	if got.OutputTokens != 7 {
		t.Errorf("output_tokens = %d, want 7", got.OutputTokens)
	}
	if got.CacheCreationTokens != 1024 {
		t.Errorf("cache_creation_tokens = %d, want 1024", got.CacheCreationTokens)
	}
	if got.CacheReadTokens != 2048 {
		t.Errorf("cache_read_tokens = %d, want 2048", got.CacheReadTokens)
	}
	if got.CostUSD != 0.00345 {
		t.Errorf("cost_usd = %v, want 0.00345", got.CostUSD)
	}
	if got.DurationMS != 1820 {
		t.Errorf("duration_ms = %d, want 1820", got.DurationMS)
	}
	if got.DurationAPIMS != 1654 {
		t.Errorf("duration_api_ms = %d, want 1654", got.DurationAPIMS)
	}
	if got.NumTurns != 1 {
		t.Errorf("num_turns = %d, want 1", got.NumTurns)
	}
	if got.Subtype != "success" {
		t.Errorf("subtype = %q, want success", got.Subtype)
	}
	if got.IsError {
		t.Error("is_error should be false")
	}
}

// Issue #1920 hang case: the CLI closes stdout before emitting a result
// event. Per-assistant accumulators must become the authoritative totals.
func TestParseStreamNoResultEvent(t *testing.T) {
	input := `{"type":"system","subtype":"init","session_id":"hung-1","model":"claude-sonnet-4-5"}
{"type":"assistant","message":{"id":"msg_a","model":"claude-sonnet-4-5","content":[{"type":"text","text":"partial"}],"usage":{"input_tokens":5,"output_tokens":3,"cache_creation_input_tokens":100,"cache_read_input_tokens":200}},"session_id":"hung-1"}
`
	got := parseStream(strings.NewReader(input), nil)

	if got.SessionID != "hung-1" {
		t.Errorf("session = %q", got.SessionID)
	}
	if got.Text != "partial" {
		t.Errorf("text = %q", got.Text)
	}
	if got.InputTokens != 5 {
		t.Errorf("input_tokens = %d, want 5 (assistant fallback)", got.InputTokens)
	}
	if got.OutputTokens != 3 {
		t.Errorf("output_tokens = %d, want 3 (assistant fallback)", got.OutputTokens)
	}
	if got.CacheCreationTokens != 100 {
		t.Errorf("cache_creation_tokens = %d, want 100 (assistant fallback)", got.CacheCreationTokens)
	}
	if got.CacheReadTokens != 200 {
		t.Errorf("cache_read_tokens = %d, want 200 (assistant fallback)", got.CacheReadTokens)
	}
}

// When both assistant and result events carry usage, result wins.
// The result event's totals already include all assistant turns, so
// trusting it is the only way to avoid double-counting.
func TestParseStreamResultPrecedence(t *testing.T) {
	input := `{"type":"assistant","message":{"model":"m","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":999,"output_tokens":999,"cache_creation_input_tokens":999,"cache_read_input_tokens":999}}}
{"type":"result","subtype":"success","result":"x","total_cost_usd":0.5,"usage":{"input_tokens":10,"output_tokens":20,"cache_creation_input_tokens":30,"cache_read_input_tokens":40}}
`
	got := parseStream(strings.NewReader(input), nil)

	if got.InputTokens != 10 {
		t.Errorf("input_tokens = %d, want 10 (result should override assistant)", got.InputTokens)
	}
	if got.OutputTokens != 20 {
		t.Errorf("output_tokens = %d, want 20", got.OutputTokens)
	}
	if got.CacheCreationTokens != 30 {
		t.Errorf("cache_creation_tokens = %d, want 30", got.CacheCreationTokens)
	}
	if got.CacheReadTokens != 40 {
		t.Errorf("cache_read_tokens = %d, want 40", got.CacheReadTokens)
	}
	if got.CostUSD != 0.5 {
		t.Errorf("cost = %v, want 0.5", got.CostUSD)
	}
}

// Web-search tool use surfaces under server_tool_use in the usage block.
// CorticalStack doesn't currently use tools, but the field is captured
// so it's there if someone adds a tool-using agent later.
func TestParseStreamWebSearchRequests(t *testing.T) {
	input := `{"type":"result","subtype":"success","result":"x","usage":{"input_tokens":1,"output_tokens":1,"server_tool_use":{"web_search_requests":3}}}
`
	got := parseStream(strings.NewReader(input), nil)
	if got.WebSearchRequests != 3 {
		t.Errorf("web_search_requests = %d, want 3", got.WebSearchRequests)
	}
}

func TestClaudeBinExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-claude")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho fake\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	t.Setenv("CLAUDE_BIN", fake)
	got, err := claudeBin()
	if err != nil {
		t.Fatalf("claudeBin() error = %v", err)
	}
	if got != fake {
		t.Errorf("claudeBin() = %q, want %q", got, fake)
	}
}

func TestClaudeBinOverrideMissingFile(t *testing.T) {
	t.Setenv("CLAUDE_BIN", "/definitely/not/a/real/path/claude")
	_, err := claudeBin()
	if err == nil {
		t.Fatal("expected error for missing CLAUDE_BIN path, got nil")
	}
	if !strings.Contains(err.Error(), "CLAUDE_BIN") {
		t.Errorf("error should mention CLAUDE_BIN: %v", err)
	}
}
