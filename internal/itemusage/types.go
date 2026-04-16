// Package itemusage records and aggregates Claude CLI invocations
// linked to a specific dashboard item (a Product idea, a Meeting, a
// Document, or a Prototype). It exists as a sibling to internal
// /telemetry — the existing usage.jsonl stays unchanged and global,
// while this package writes a new item-usage.jsonl that the unified
// dashboard's per-card detail page reads to compute "tokens spent on
// the items currently selected in the table".
//
// Why not extend agent.Invocation directly?
//   - Existing usage.jsonl rows have no item context and would have
//     to be backfilled (impossible) or carry empty fields forever.
//   - Most call sites genuinely don't know which item they're
//     operating on — synthesizers span many items, intent
//     classification has no item, the ingest pipeline runs before any
//     item exists. Adding ItemID to every call site would be a lot of
//     plumbing for fields that stay empty 90% of the time.
//   - The new index is opt-in: only callers that know the item write
//     to it. Aggregation is then strictly correct: an Aggregate over
//     {item_id: X} contains exactly the calls made for item X.
package itemusage

import (
	"time"
)

// Entry is one Claude CLI invocation tagged with the dashboard item
// it was made for. Field names track the JSON tags so a JSONL line
// produced by Recorder roundtrips cleanly through Reader.
type Entry struct {
	Timestamp           time.Time `json:"timestamp"`
	ItemType            string    `json:"item_type"` // "product" | "meeting" | "document" | "prototype"
	ItemID              string    `json:"item_id"`
	Model               string    `json:"model,omitempty"`
	InputTokens         int       `json:"input_tokens"`
	OutputTokens        int       `json:"output_tokens"`
	CacheCreationTokens int       `json:"cache_creation_tokens"`
	CacheReadTokens     int       `json:"cache_read_tokens"`
	CostUSD             float64   `json:"cost_usd"`
	DurationMS          int64     `json:"duration_ms"`
	CallerHint          string    `json:"caller_hint,omitempty"`
	Error               string    `json:"error,omitempty"`
}

// Aggregate is the summed view returned by Reader. Mirrors the shape
// of telemetry.Summary so the cards handler can serialize one or the
// other without remapping. ByModel is keyed by model name.
type Aggregate struct {
	Calls               int                    `json:"calls"`
	CostUSD             float64                `json:"cost_usd"`
	InputTokens         int                    `json:"input_tokens"`
	OutputTokens        int                    `json:"output_tokens"`
	CacheCreationTokens int                    `json:"cache_creation_tokens"`
	CacheReadTokens     int                    `json:"cache_read_tokens"`
	ByModel             map[string]ModelTotals `json:"by_model"`
}

// ModelTotals is the per-model slice of an Aggregate.
type ModelTotals struct {
	Calls               int     `json:"calls"`
	CostUSD             float64 `json:"cost_usd"`
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
}

// Filter is the query against item-usage.jsonl. Empty fields mean
// "match anything for this dimension" so a zero-value Filter returns
// every entry — useful for computing a global aggregate across all
// items of all types.
type Filter struct {
	// Type, when non-empty, restricts results to entries with this
	// item_type. Use the canonical strings "product" / "meeting" /
	// "document" / "prototype".
	Type string

	// IDs, when non-nil and non-empty, restricts results to entries
	// whose ItemID is in the set. Empty IDs in the slice are ignored.
	IDs []string

	// Window, when non-zero, restricts results to entries whose
	// Timestamp is within [now-Window, now). Zero means "all time".
	Window time.Duration

	// Now is injected by tests so Window math is deterministic. The
	// production caller leaves this zero and Reader uses time.Now.
	Now time.Time
}
