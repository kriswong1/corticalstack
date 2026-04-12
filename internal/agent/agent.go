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
)

// Agent configures a Claude CLI invocation.
type Agent struct {
	Model      string // e.g. "claude-sonnet-4-6"; empty = CLI default
	MaxTurns   int    // 0 = CLI default
	WorkingDir string
}

// Result holds the output of a Claude CLI invocation.
type Result struct {
	Text      string
	SessionID string
	CostUSD   float64
}

type streamEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	Message   json.RawMessage `json:"message,omitempty"`
	ResultRaw json.RawMessage `json:"result,omitempty"`
	Content   []contentBlock  `json:"content,omitempty"`
	TotalCost float64         `json:"total_cost_usd,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type resultPayload struct {
	Text      string  `json:"text,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
}

// Run sends a prompt to Claude via stdin and returns the full result.
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

	result := parseStream(stdout)

	if err := cmd.Wait(); err != nil {
		if result.Text == "" {
			return nil, fmt.Errorf("claude exited with error: %w", err)
		}
	}
	slog.Info("claude cli done", "duration", time.Since(start), "cost_usd", result.CostUSD, "result_len", len(result.Text))
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

func parseStream(r io.Reader) *Result {
	result := &Result{}
	var textParts []string

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
		case "assistant":
			for _, block := range event.Content {
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			}
		case "result":
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
						result.CostUSD = rp.CostUSD
					}
				}
			}
			if event.SessionID != "" {
				result.SessionID = event.SessionID
			}
			if event.TotalCost > 0 {
				result.CostUSD = event.TotalCost
			}
		}
	}

	if result.Text == "" && len(textParts) > 0 {
		result.Text = strings.Join(textParts, "")
	}
	return result
}

func claudeBin() (string, error) {
	if p, err := exec.LookPath("claude"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		filepath.Join(home, ".claude", "local", "claude.exe"),
		filepath.Join(home, "AppData", "Local", "Programs", "claude-code", "claude.exe"),
		filepath.Join(home, "AppData", "Roaming", "npm", "claude.cmd"),
	} {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("claude CLI not found. Install from https://claude.ai/download and run `claude login`")
}

// IsInstalled reports whether the Claude CLI is available.
func IsInstalled() bool {
	_, err := claudeBin()
	return err == nil
}
