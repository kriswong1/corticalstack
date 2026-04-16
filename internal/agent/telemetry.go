package agent

import "time"

// ItemContext optionally tags a Claude CLI call with the dashboard
// item it was made for. Empty Type or ID means "no item context" —
// the call still flows through DefaultRecorder (the global usage log)
// but is skipped by ItemRecorder (the per-item index).
//
// Callers populate this only when they know which Product idea /
// Meeting / Document / Prototype the call belongs to. Synthesizers
// that span many items, intent classification, and the ingest
// pipeline leave it zero — those calls would have a wrong or
// arbitrary item if forced.
type ItemContext struct {
	Type string // "product" | "meeting" | "document" | "prototype"
	ID   string
}

// ItemRecorder is the sink for item-tagged invocations. Implemented
// by internal/itemusage.JSONLRecorder; the interface lives in agent
// to avoid an import cycle (agent → itemusage would be wrong, since
// itemusage already needs to be importable elsewhere).
//
// The interface is intentionally minimal: just one method, with a
// concrete struct argument (ItemEvent) that carries every field the
// recorder might want. Extending ItemEvent later is non-breaking; a
// new field on the struct doesn't change the interface.
type ItemRecorder interface {
	RecordItem(ItemEvent)
}

// ItemEvent is the per-call payload handed to ItemRecorder.
// Mirrors a subset of Invocation plus the Item context. Field names
// match the itemusage.Entry JSON tags so the recorder can marshal it
// directly.
type ItemEvent struct {
	Timestamp           time.Time
	ItemType            string
	ItemID              string
	Model               string
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	CostUSD             float64
	DurationMS          int64
	CallerHint          string
	Error               string
}

// DefaultItemRecorder is the package-level sink for item-tagged
// invocations. main wires this once at startup; if it stays nil,
// Run() simply skips the item recording branch. Mirrors the
// DefaultRecorder pattern so tests don't have to thread a recorder
// through every Agent constructor.
var DefaultItemRecorder ItemRecorder

// Invocation is the structured record of a single Claude CLI call.
// It is captured by Run() after every invocation (success or failure)
// and handed to the package-level DefaultRecorder if one is set.
//
// Field names track the JSON tags exactly so a JSONL file written by
// the recorder can be read back into this same struct without remapping.
type Invocation struct {
	Timestamp           time.Time `json:"timestamp"`
	Model               string    `json:"model,omitempty"`
	SessionID           string    `json:"session_id,omitempty"`
	InputTokens         int       `json:"input_tokens"`
	OutputTokens        int       `json:"output_tokens"`
	CacheCreationTokens int       `json:"cache_creation_tokens"`
	CacheReadTokens     int       `json:"cache_read_tokens"`
	WebSearchRequests   int       `json:"web_search_requests,omitempty"`
	CostUSD             float64   `json:"cost_usd"`
	DurationMS          int64     `json:"duration_ms"`
	DurationAPIMS       int64     `json:"duration_api_ms,omitempty"`
	NumTurns            int       `json:"num_turns,omitempty"`
	Subtype             string    `json:"subtype,omitempty"`
	WorkingDir          string    `json:"working_dir,omitempty"`
	MaxTurns            int       `json:"max_turns,omitempty"`
	CallerHint          string    `json:"caller_hint,omitempty"`
	PromptLen           int       `json:"prompt_len"`
	ResultLen           int       `json:"result_len"`
	Error               string    `json:"error,omitempty"`
}

// Recorder is the sink for captured Invocations. Implementations live
// outside this package (see internal/telemetry) to avoid an import
// cycle — agent owns the type so any package can implement it.
//
// Record must be safe for concurrent use; the agent calls it from
// arbitrary goroutines (the web/jobs manager spawns one per ingest).
// Implementations should never panic and should swallow recoverable
// errors (e.g. disk full) — telemetry must not break the caller.
type Recorder interface {
	Record(Invocation)
}

// DefaultRecorder is the package-level sink. main wires this once at
// startup; if it stays nil, Run() simply skips recording. Global state
// is acceptable here because every call site already constructs Agent
// as a one-shot literal — threading a recorder field through 11 call
// sites and 8 constructors would be pure plumbing for one process-wide
// observability concern with one lifecycle.
var DefaultRecorder Recorder
