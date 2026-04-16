// Package agent wraps the Claude CLI using the Paperclip pattern:
// shell out to `claude --print` which uses the Claude Max subscription
// at $0/call. No ANTHROPIC_API_KEY needed.
package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/platform"
)

// Agent configures a Claude CLI invocation.
type Agent struct {
	Model      string // e.g. "claude-sonnet-4-6"; empty = CLI default
	MaxTurns   int    // 0 = CLI default
	WorkingDir string
	CallerHint string // optional, opt-in tag for telemetry — e.g. "intent.classify"

	// Item is an optional dashboard-item tag. When both Type and ID
	// are non-empty, Run() additionally hands the call to
	// DefaultItemRecorder so the unified dashboard's per-card detail
	// page can compute aggregate usage for selected items. Leave
	// zero for calls that span many items or don't belong to one.
	Item ItemContext

	// OnTurn is an optional callback fired each time parseStream
	// detects a new assistant turn. The int is the 1-based turn
	// count so far. Used by the pipeline view to show real-time
	// progress ("Turn 2 of 10..."). Safe to leave nil.
	OnTurn func(turn int)
}

// Result holds the output of a Claude CLI invocation. Token and cost
// fields come from the stream-json `result` event when present, or
// from per-assistant accumulation as a fallback (Anthropic claude-code
// issue #1920: hung sessions sometimes never emit a result event).
type Result struct {
	Text                string
	Model               string
	SessionID           string
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	WebSearchRequests   int
	CostUSD             float64
	DurationMS          int64
	DurationAPIMS       int64
	NumTurns            int
	Subtype             string
	IsError             bool
}

// streamEvent is one line of `--output-format stream-json --verbose`.
// Real CLI output nests assistant content + usage under .message; the
// flat .Content field is kept only as a fallback for legacy fixtures.
type streamEvent struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Model     string          `json:"model,omitempty"`
	Message   json.RawMessage `json:"message,omitempty"`
	ResultRaw json.RawMessage `json:"result,omitempty"`

	// Result-event fields (top-level on the event itself, not nested).
	Usage         *usageBlock `json:"usage,omitempty"`
	TotalCost     float64     `json:"total_cost_usd,omitempty"`
	DurationMS    int64       `json:"duration_ms,omitempty"`
	DurationAPIMS int64       `json:"duration_api_ms,omitempty"`
	NumTurns      int         `json:"num_turns,omitempty"`
	IsError       bool        `json:"is_error,omitempty"`

	// Legacy / fallback: assistant content at the event root. Real CLI
	// output puts this under .message.content; tests still use the flat
	// shape, so we accept both.
	Content []contentBlock `json:"content,omitempty"`
}

// messageEnvelope is the shape of streamEvent.Message for assistant
// events: {id, model, content[], usage{...}}. Parsed lazily.
type messageEnvelope struct {
	ID      string         `json:"id,omitempty"`
	Model   string         `json:"model,omitempty"`
	Content []contentBlock `json:"content,omitempty"`
	Usage   *usageBlock    `json:"usage,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// usageBlock carries Anthropic's token accounting. Field names match
// the wire schema exactly; renaming happens at the Result boundary.
type usageBlock struct {
	InputTokens              int            `json:"input_tokens,omitempty"`
	OutputTokens             int            `json:"output_tokens,omitempty"`
	CacheCreationInputTokens int            `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int            `json:"cache_read_input_tokens,omitempty"`
	ServerToolUse            *serverToolUse `json:"server_tool_use,omitempty"`
}

type serverToolUse struct {
	WebSearchRequests int `json:"web_search_requests,omitempty"`
}

// resultPayload is the legacy shape some older fixtures use, where
// `result.result` is an object instead of a bare string. Real CLI
// output puts cost on the event itself (event.TotalCost), so the
// CostUSD here is dead in production but kept for the existing test.
type resultPayload struct {
	Text      string  `json:"text,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
}

// Run sends a prompt to Claude via stdin and returns the full result.
// After the CLI exits (success or failure), an Invocation is handed
// to DefaultRecorder if one is set. Telemetry never blocks the caller
// and never fails a successful call.
func (a *Agent) Run(ctx context.Context, prompt string) (*Result, error) {
	bin, err := claudeBin()
	if err != nil {
		return nil, err
	}

	args := []string{"--print", "-", "--output-format", "stream-json", "--verbose"}
	if a.Model != "" {
		args = append(args, "--model", a.Model)
	}
	if a.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(a.MaxTurns))
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stderr = os.Stderr
	if a.WorkingDir != "" {
		cmd.Dir = a.WorkingDir
	}

	slog.Info("claude cli start", "model", a.Model, "prompt_len", len(prompt))
	start := time.Now()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting claude: %w", err)
	}

	result := parseStream(stdout, a.OnTurn)
	waitErr := cmd.Wait()
	duration := time.Since(start)

	// Telemetry runs unconditionally. Even a killed CLI emits whatever
	// the stream managed to produce before EOF — session_id, partial
	// usage, model — and we want all of that captured.
	if DefaultRecorder != nil {
		inv := Invocation{
			Timestamp:           start,
			Model:               result.Model,
			SessionID:           result.SessionID,
			InputTokens:         result.InputTokens,
			OutputTokens:        result.OutputTokens,
			CacheCreationTokens: result.CacheCreationTokens,
			CacheReadTokens:     result.CacheReadTokens,
			WebSearchRequests:   result.WebSearchRequests,
			CostUSD:             result.CostUSD,
			DurationMS:          duration.Milliseconds(),
			DurationAPIMS:       result.DurationAPIMS,
			NumTurns:            result.NumTurns,
			Subtype:             result.Subtype,
			WorkingDir:          a.WorkingDir,
			MaxTurns:            a.MaxTurns,
			CallerHint:          a.CallerHint,
			PromptLen:           len(prompt),
			ResultLen:           len(result.Text),
		}
		switch {
		case waitErr != nil:
			inv.Error = waitErr.Error()
		case result.IsError:
			inv.Error = "cli reported is_error=true subtype=" + result.Subtype
		}
		DefaultRecorder.Record(inv)
	}

	// Item-tagged recording: the second telemetry sink, opt-in via
	// Agent.Item. Skipped silently when the agent didn't carry an
	// item context (synthesizers, intent classification, ingest) or
	// when no DefaultItemRecorder is wired (tests, partial startup).
	if DefaultItemRecorder != nil && a.Item.Type != "" && a.Item.ID != "" {
		event := ItemEvent{
			Timestamp:           start,
			ItemType:            a.Item.Type,
			ItemID:              a.Item.ID,
			Model:               result.Model,
			InputTokens:         result.InputTokens,
			OutputTokens:        result.OutputTokens,
			CacheCreationTokens: result.CacheCreationTokens,
			CacheReadTokens:     result.CacheReadTokens,
			CostUSD:             result.CostUSD,
			DurationMS:          duration.Milliseconds(),
			CallerHint:          a.CallerHint,
		}
		switch {
		case waitErr != nil:
			event.Error = waitErr.Error()
		case result.IsError:
			event.Error = "cli reported is_error=true subtype=" + result.Subtype
		}
		DefaultItemRecorder.RecordItem(event)
	}

	slog.Info("claude cli done",
		"duration", duration,
		"model", result.Model,
		"num_turns", result.NumTurns,
		"max_turns", a.MaxTurns,
		"input_tokens", result.InputTokens,
		"output_tokens", result.OutputTokens,
		"cache_creation_tokens", result.CacheCreationTokens,
		"cache_read_tokens", result.CacheReadTokens,
		"cost_usd", result.CostUSD,
		"result_len", len(result.Text),
	)

	// Flag max-turns exhaustion so we can detect processes that need
	// a higher limit. Subtype "error_max_turns" is set by the CLI.
	if result.Subtype == "error_max_turns" {
		slog.Warn("claude cli hit max turns",
			"caller_hint", a.CallerHint,
			"max_turns", a.MaxTurns,
			"num_turns", result.NumTurns,
			"result_len", len(result.Text),
			"model", result.Model,
		)
	}

	if waitErr != nil {
		if result.Text == "" {
			return nil, fmt.Errorf("claude exited with error: %w", waitErr)
		}
	}
	return result, nil
}

// RunSimple returns only the assistant response text.
func (a *Agent) RunSimple(ctx context.Context, prompt string) (string, error) {
	result, err := a.Run(ctx, prompt)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

func parseStream(r io.Reader, onTurn func(int)) *Result {
	result := &Result{}
	var textParts []string

	// Per-assistant accumulators. Used as the authoritative source if
	// no result event arrives before EOF (issue #1920 hang case). When
	// a result event does arrive, its top-level usage block overwrites
	// these — that's the canonical total for the call.
	var (
		accInputTokens         int
		accOutputTokens        int
		accCacheCreationTokens int
		accCacheReadTokens     int
		accWebSearchRequests   int
		sawResult              bool
		turnCount              int
	)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var parseErrors int
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			parseErrors++
			if parseErrors <= 3 {
				slog.Warn("claude stream: malformed line", "error", err, "line_len", len(line))
			}
			continue
		}
		switch event.Type {
		case "system":
			if event.SessionID != "" {
				result.SessionID = event.SessionID
			}
			if event.Model != "" && result.Model == "" {
				result.Model = event.Model
			}
		case "assistant":
			// Count each assistant event as a turn and fire the
			// progress callback so the pipeline UI can show "Turn N".
			turnCount++
			if onTurn != nil {
				onTurn(turnCount)
			}
			// Real CLI shape: content + usage nested under .message.
			if len(event.Message) > 0 {
				var env messageEnvelope
				if err := json.Unmarshal(event.Message, &env); err == nil {
					if env.Model != "" {
						result.Model = env.Model
					}
					for _, block := range env.Content {
						if block.Type == "text" && block.Text != "" {
							textParts = append(textParts, block.Text)
						}
					}
					if env.Usage != nil {
						accInputTokens += env.Usage.InputTokens
						accOutputTokens += env.Usage.OutputTokens
						accCacheCreationTokens += env.Usage.CacheCreationInputTokens
						accCacheReadTokens += env.Usage.CacheReadInputTokens
						if env.Usage.ServerToolUse != nil {
							accWebSearchRequests += env.Usage.ServerToolUse.WebSearchRequests
						}
					}
				}
			}
			// Legacy / fallback shape: content at the event root. Some
			// existing tests use this; real CLI output never does.
			for _, block := range event.Content {
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			}
			if event.SessionID != "" {
				result.SessionID = event.SessionID
			}
		case "result":
			sawResult = true
			if len(event.ResultRaw) > 0 {
				var resultStr string
				if err := json.Unmarshal(event.ResultRaw, &resultStr); err == nil {
					if resultStr != "" {
						result.Text = resultStr
					}
				} else {
					var rp resultPayload
					if json.Unmarshal(event.ResultRaw, &rp) == nil {
						if rp.Text != "" {
							result.Text = rp.Text
						}
						if rp.SessionID != "" {
							result.SessionID = rp.SessionID
						}
						if rp.CostUSD > 0 {
							result.CostUSD = rp.CostUSD
						}
					}
				}
			}
			if event.SessionID != "" {
				result.SessionID = event.SessionID
			}
			if event.TotalCost > 0 {
				result.CostUSD = event.TotalCost
			}
			if event.Usage != nil {
				result.InputTokens = event.Usage.InputTokens
				result.OutputTokens = event.Usage.OutputTokens
				result.CacheCreationTokens = event.Usage.CacheCreationInputTokens
				result.CacheReadTokens = event.Usage.CacheReadInputTokens
				if event.Usage.ServerToolUse != nil {
					result.WebSearchRequests = event.Usage.ServerToolUse.WebSearchRequests
				}
			}
			if event.DurationMS > 0 {
				result.DurationMS = event.DurationMS
			}
			if event.DurationAPIMS > 0 {
				result.DurationAPIMS = event.DurationAPIMS
			}
			if event.NumTurns > 0 {
				result.NumTurns = event.NumTurns
			}
			if event.Subtype != "" {
				result.Subtype = event.Subtype
			}
			if event.IsError {
				result.IsError = true
			}
		}
	}

	if result.Text == "" && len(textParts) > 0 {
		result.Text = strings.Join(textParts, "")
	}

	// No result event — fall back to per-assistant accumulators so
	// hung sessions still produce telemetry instead of all-zero rows.
	if !sawResult {
		result.InputTokens = accInputTokens
		result.OutputTokens = accOutputTokens
		result.CacheCreationTokens = accCacheCreationTokens
		result.CacheReadTokens = accCacheReadTokens
		result.WebSearchRequests = accWebSearchRequests
	}

	return result
}

func claudeBin() (string, error) {
	// Explicit override wins. Cheapest escape hatch for weird setups —
	// e.g. WSL2 users who prefer to invoke a specific Windows-side binary,
	// or CI runners with a pinned path.
	if v := os.Getenv("CLAUDE_BIN"); v != "" {
		v = platform.MaybeTranslateForWSL(v)
		if _, err := os.Stat(v); err == nil {
			return v, nil
		}
		return "", fmt.Errorf("CLAUDE_BIN=%q but that file does not exist", v)
	}

	// PATH lookup — covers native installs on Linux, macOS, Windows, and
	// any WSL2 setup where claude was installed inside the WSL distro.
	if p, err := exec.LookPath("claude"); err == nil {
		return p, nil
	}

	// Native home-directory candidates (Windows layouts).
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, c := range []string{
			filepath.Join(home, ".claude", "local", "claude.exe"),
			filepath.Join(home, "AppData", "Local", "Programs", "claude-code", "claude.exe"),
			filepath.Join(home, "AppData", "Roaming", "npm", "claude.cmd"),
		} {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}

	// WSL2 fallback: the binary may live on the Windows side under
	// /mnt/c/Users/<winuser>/..., but os.UserHomeDir() in WSL returns
	// /home/<linuxuser> so the loop above never sees it. Glob every
	// Windows user directory for the known install layouts.
	if platform.IsWSL() {
		for _, pat := range []string{
			"/mnt/c/Users/*/.claude/local/claude.exe",
			"/mnt/c/Users/*/AppData/Local/Programs/claude-code/claude.exe",
			"/mnt/c/Users/*/AppData/Roaming/npm/claude.cmd",
		} {
			matches, _ := filepath.Glob(pat)
			for _, m := range matches {
				if _, err := os.Stat(m); err == nil {
					return m, nil
				}
			}
		}
		return "", fmt.Errorf("claude CLI not found in WSL2. Options: install inside your Linux distro (`npm i -g @anthropic-ai/claude-code`), or set CLAUDE_BIN to a Windows-side path like /mnt/c/Users/<you>/.claude/local/claude.exe")
	}

	return "", fmt.Errorf("claude CLI not found. Install from https://claude.ai/download and run `claude login`, or set CLAUDE_BIN to the binary path")
}

// IsInstalled reports whether the Claude CLI is available.
func IsInstalled() bool {
	_, err := claudeBin()
	return err == nil
}
