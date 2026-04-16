package itemusage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/kriswong/corticalstack/internal/agent"
)

// JSONLRecorder appends one Entry per line to item-usage.jsonl. It is
// a near-clone of telemetry.JSONLRecorder but written separately so
// the two indices can evolve independently — adding a field to Entry
// (e.g. ParentID for nested calls) shouldn't force a schema change on
// the global usage log.
//
// Safe for concurrent callers: the web/jobs manager spawns multiple
// agent.Run goroutines per ingest and any of them may pass through a
// recorder.Record() call.
type JSONLRecorder struct {
	path string
	mu   sync.Mutex
	f    *os.File
}

// NewJSONLRecorder opens (or creates) item-usage.jsonl. Returns an
// error only on initial open failures; per-record write errors are
// swallowed inside Record so item-usage telemetry can never break a
// CLI call.
func NewJSONLRecorder(path string) (*JSONLRecorder, error) {
	if path == "" {
		return nil, fmt.Errorf("itemusage: empty log path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("itemusage: mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("itemusage: open %s: %w", path, err)
	}
	return &JSONLRecorder{path: path, f: f}, nil
}

// Record marshals the entry, appends a newline, and writes the whole
// buffer in one Write call under the mutex. Single-Write keeps lines
// atomic on Windows where O_APPEND atomicity is best-effort.
//
// Drops entries with empty ItemType or ItemID — those are programming
// errors at the call site (caller knew the item but forgot to pass
// the context). Logging the drop at Warn level surfaces the bug
// without breaking the user-facing CLI call.
func (r *JSONLRecorder) Record(e Entry) {
	if e.ItemType == "" || e.ItemID == "" {
		slog.Warn("itemusage: dropping entry with empty type/id",
			"type", e.ItemType, "id", e.ItemID, "model", e.Model)
		return
	}
	buf, err := json.Marshal(e)
	if err != nil {
		slog.Warn("itemusage: marshal failed",
			"error", err, "type", e.ItemType, "id", e.ItemID)
		return
	}
	buf = append(buf, '\n')

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.f.Write(buf); err != nil {
		slog.Warn("itemusage: write failed",
			"error", err, "type", e.ItemType, "id", e.ItemID)
	}
}

// RecordItem adapts an agent.ItemEvent (the cross-package wire form)
// into a local Entry and persists it. Implements agent.ItemRecorder
// so main can wire `agent.DefaultItemRecorder = recorder` directly.
func (r *JSONLRecorder) RecordItem(e agent.ItemEvent) {
	r.Record(Entry{
		Timestamp:           e.Timestamp,
		ItemType:            e.ItemType,
		ItemID:              e.ItemID,
		Model:               e.Model,
		InputTokens:         e.InputTokens,
		OutputTokens:        e.OutputTokens,
		CacheCreationTokens: e.CacheCreationTokens,
		CacheReadTokens:     e.CacheReadTokens,
		CostUSD:             e.CostUSD,
		DurationMS:          e.DurationMS,
		CallerHint:          e.CallerHint,
		Error:               e.Error,
	})
}

// Close flushes and closes the file handle. Safe to call once at
// shutdown via defer in main.
func (r *JSONLRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// Path returns the on-disk log file path. Useful for tests and for
// the Reader, which reads the same file independently.
func (r *JSONLRecorder) Path() string {
	return r.path
}

// DefaultRecorder is the package-level sink. main wires this once at
// startup; if it stays nil, agent.Run's item-context branch silently
// skips recording. Mirrors the agent.DefaultRecorder convention.
var DefaultRecorder *JSONLRecorder
