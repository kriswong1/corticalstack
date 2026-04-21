package persona

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kriswong/corticalstack/internal/agent"
)

// MaxChatTurns is the hard ceiling on user–Claude exchanges per session.
const MaxChatTurns = 10

// ChatMessage is a single turn in the persona setup conversation.
type ChatMessage struct {
	Role    string   `json:"role"`    // "assistant" or "user"
	Content string   `json:"content"` // display text
	Options []string `json:"options,omitempty"`
}

// ChatSession holds the state of a multi-turn persona setup conversation.
type ChatSession struct {
	ID          string        `json:"id"`
	PersonaName Name          `json:"persona_name"`
	Messages    []ChatMessage `json:"messages"`
	TurnCount   int           `json:"turn_count"` // count of user messages sent
	MaxTurns    int           `json:"max_turns"`
	Done        bool          `json:"done"`
	Result      string        `json:"result"` // generated persona markdown
}

// claudeResponse is the JSON structure Claude returns per turn.
type claudeResponse struct {
	Text    string   `json:"text"`
	Options []string `json:"options,omitempty"`
	Done    bool     `json:"done,omitempty"`
	Result  string   `json:"result,omitempty"`
}

// StartChat creates a new session and generates the first assistant message.
func StartChat(ctx context.Context, name Name, model, workingDir string) (*ChatSession, error) {
	session := &ChatSession{
		PersonaName: name,
		MaxTurns:    MaxChatTurns,
	}

	prompt := buildChatPrompt(name, session.Messages, "")
	raw, err := runChatTurn(ctx, model, workingDir, prompt)
	if err != nil {
		return nil, fmt.Errorf("persona chat start: %w", err)
	}

	resp := parseChatResponse(raw)
	session.Messages = append(session.Messages, ChatMessage{
		Role:    "assistant",
		Content: resp.Text,
		Options: resp.Options,
	})

	return session, nil
}

// ContinueChat sends the user's input and gets Claude's next response.
// Returns true if the conversation is done (either Claude said so or
// we've hit the turn limit).
func ContinueChat(ctx context.Context, session *ChatSession, userInput, model, workingDir string) (bool, error) {
	session.Messages = append(session.Messages, ChatMessage{
		Role:    "user",
		Content: userInput,
	})
	session.TurnCount++

	isFinalTurn := session.TurnCount >= session.MaxTurns
	prompt := buildChatPrompt(session.PersonaName, session.Messages, "")
	if isFinalTurn {
		prompt += "\n\nIMPORTANT: This is the final turn. Generate the complete persona file NOW. Set done=true and put the full markdown in the result field.\n"
	}

	raw, err := runChatTurn(ctx, model, workingDir, prompt)
	if err != nil {
		return false, fmt.Errorf("persona chat continue: %w", err)
	}

	resp := parseChatResponse(raw)
	session.Messages = append(session.Messages, ChatMessage{
		Role:    "assistant",
		Content: resp.Text,
		Options: resp.Options,
	})

	if resp.Done || isFinalTurn {
		session.Done = true
		session.Result = resp.Result
	}

	return session.Done, nil
}

// FinishChat forces generation of the persona file from whatever
// context has been collected so far (the "Done early" flow).
func FinishChat(ctx context.Context, session *ChatSession, model, workingDir string) error {
	prompt := buildChatPrompt(session.PersonaName, session.Messages, "")
	prompt += "\n\nThe user clicked 'Done early'. Generate the complete persona file NOW from everything collected so far. Set done=true and put the full markdown in the result field.\n"

	raw, err := runChatTurn(ctx, model, workingDir, prompt)
	if err != nil {
		return fmt.Errorf("persona chat finish: %w", err)
	}

	resp := parseChatResponse(raw)
	session.Done = true
	session.Result = resp.Result
	if session.Result == "" {
		// Fallback: use the text output as the result
		session.Result = resp.Text
	}

	return nil
}

func runChatTurn(ctx context.Context, model, workingDir, prompt string) (string, error) {
	ag := &agent.Agent{
		Model:      model,
		MaxTurns:   10,
		WorkingDir: workingDir,
		CallerHint: "persona.chat",
	}
	return ag.RunSimple(ctx, prompt)
}

func parseChatResponse(raw string) claudeResponse {
	raw = strings.TrimSpace(raw)
	// Strip code fences if Claude wrapped the JSON
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var resp claudeResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		// Claude didn't return valid JSON — degrade to text-only so the
		// UI still shows something, but log so the prompt failure is
		// visible in production logs instead of silently missing the
		// `done` signal.
		slog.Warn("persona.chat: non-JSON response", "error", err, "raw_prefix", truncatePrefix(raw, 200))
		return claudeResponse{Text: raw}
	}
	return resp
}

func truncatePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func buildChatPrompt(name Name, history []ChatMessage, extra string) string {
	var b strings.Builder

	b.WriteString(systemPromptFor(name))
	b.WriteString("\n\n")
	b.WriteString("IMPORTANT: Do NOT use any tools. Respond with ONLY a JSON object.\n\n")
	b.WriteString("## Response format\n\n")
	b.WriteString("Respond with a single JSON object (no code fences, no prose outside the JSON):\n")
	b.WriteString("```\n")
	b.WriteString(`{"text": "Your message to the user", "options": ["Option A", "Option B"], "done": false}`)
	b.WriteString("\n```\n")
	b.WriteString("- `text`: your message (markdown OK)\n")
	b.WriteString("- `options`: optional array of selectable choices for the user. Omit when a freeform answer is better.\n")
	b.WriteString("- `done`: set true ONLY when you have enough info to generate the file\n")
	b.WriteString("- `result`: when done=true, include the complete generated persona markdown file here\n\n")

	if len(history) > 0 {
		b.WriteString("## Conversation so far\n\n")
		for _, msg := range history {
			if msg.Role == "assistant" {
				b.WriteString("**Claude:** ")
			} else {
				b.WriteString("**User:** ")
			}
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		}
	}

	b.WriteString("## Your turn\n\nContinue the conversation. Ask the next question or, if you have enough information, generate the file.\n")

	if extra != "" {
		b.WriteString("\n")
		b.WriteString(extra)
	}

	return b.String()
}

func systemPromptFor(name Name) string {
	switch name {
	case NameSoul:
		return `You are helping a user configure their CorticalStack SOUL persona file. This file controls how Claude extracts structured data — tone, formatting conventions, domain focus, tagging rules, action-item formatting, and never-do constraints.

Ask focused questions one at a time. Mix freeform questions with multiple-choice options. Cover:
- What domain they work in (engineering, product, research, etc.)
- Preferred extraction tone (terse vs. detailed, formal vs. casual)
- How they want action items formatted (owner, deadlines, specificity)
- Tagging philosophy (flat tags vs. hierarchical, auto-tag vs. manual)
- Structure conventions (bullet vs. prose, section headings)
- Things Claude should NEVER do during extraction

Generate a SOUL.md file with this frontmatter:
---
type: persona
role: soul
purpose: Extraction style rules
---

Stay under 3,500 characters.`

	case NameUser:
		return `You are helping a user configure their CorticalStack USER persona file. This file tells Claude who the user is so every extraction is contextually relevant.

Ask focused questions one at a time. Mix freeform questions with multiple-choice options. Cover:
- Their name and role
- Timezone and working hours
- Platforms and tools they use daily
- What they use CorticalStack for (PKM goals)
- What "good output" looks like to them
- Current projects or focus areas

Generate a USER.md file with this frontmatter:
---
type: persona
role: user
purpose: User profile and identity context
---

Stay under 2,000 characters.`

	case NameMemory:
		return `You are helping a user seed their CorticalStack MEMORY persona file. Memory is different from Soul and User — it's accumulated context, not a personality profile. The chat creates a useful starting scaffold that will evolve over time.

Ask focused questions one at a time. Mix freeform questions with multiple-choice options. Cover:
- Active decisions they're currently tracking
- Important notes or documents that should always inform Claude
- Open questions they're mulling (from PRDs, research, etc.)
- Recurring themes in their work
- Things they keep forgetting and want Claude to remember

Generate a MEMORY.md file with this frontmatter:
---
type: persona
role: memory
purpose: Curated decisions and load-bearing notes
---

Stay under 2,500 characters.`

	default:
		return "You are helping configure a persona file."
	}
}
