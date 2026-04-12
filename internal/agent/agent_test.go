package agent

import (
	"strings"
	"testing"
)

func TestParseStreamAssistantText(t *testing.T) {
	input := `{"type":"system","session_id":"sess-1"}
{"type":"assistant","content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}]}
`
	got := parseStream(strings.NewReader(input))
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
	got := parseStream(strings.NewReader(input))
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
	got := parseStream(strings.NewReader(input))
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
	got := parseStream(strings.NewReader(input))
	if got.Text != "fallback joined" {
		t.Errorf("text = %q", got.Text)
	}
}

func TestParseStreamSkipsBlankLines(t *testing.T) {
	input := `

{"type":"system","session_id":"sess-4"}

{"type":"result","result":"ok"}
`
	got := parseStream(strings.NewReader(input))
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
	got := parseStream(strings.NewReader(input))
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
	got := parseStream(strings.NewReader(input))
	if got.Text != "kept" {
		t.Errorf("text = %q", got.Text)
	}
}

func TestParseStreamEmptyInput(t *testing.T) {
	got := parseStream(strings.NewReader(""))
	if got.Text != "" {
		t.Errorf("text = %q, expected empty", got.Text)
	}
}

func TestParseStreamEmptyTextBlockSkipped(t *testing.T) {
	// Empty text blocks inside assistant content should be skipped.
	input := `{"type":"assistant","content":[{"type":"text","text":""},{"type":"text","text":"real"}]}
`
	got := parseStream(strings.NewReader(input))
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
