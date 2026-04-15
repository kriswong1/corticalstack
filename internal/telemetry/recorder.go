// Package telemetry persists agent.Invocation records to disk and reads
// them back for the /usage dashboard. The Recorder writes append-only
// JSONL; the Reader scans the same file on demand.
//
// This package imports agent for the Invocation type but agent never
// imports telemetry — main wires the recorder into agent.DefaultRecorder
// at startup.
package telemetry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/kriswong/corticalstack/internal/agent"
)

// JSONLRecorder appends one JSON object per line to a file. Safe for
// concurrent callers: the web/jobs manager fires multiple Agent.Run
// goroutines per ingest, so Record is called from arbitrary goroutines.
//
// The file handle is opened once on construction and kept for the
// process lifetime to avoid open/close churn under load. Errors during
// Record are logged via slog and swallowed — telemetry must never break
// the caller.
type JSONLRecorder struct {
	path string
	mu   sync.Mutex
	f    *os.File
}

// NewJSONLRecorder creates the parent directory (mode 0o700) if needed
// and opens the log file for append. Returns an error only on initial
// open failures; per-record write errors are absorbed.
func NewJSONLRecorder(path string) (*JSONLRecorder, error) {
	if path == "" {
		return nil, fmt.Errorf("telemetry: empty log path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("telemetry: mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("telemetry: open %s: %w", path, err)
	}
	return &JSONLRecorder{path: path, f: f}, nil
}

// Record marshals the invocation, appends a newline, and writes the
// whole buffer in one Write call under the mutex. Single-Write keeps
// lines atomic on Windows where O_APPEND atomicity is best-effort.
//
// json.Encoder is intentionally not used here: it isn't safe for
// concurrent callers and buffers internally, which can interleave
// lines even under a mutex.
func (r *JSONLRecorder) Record(inv agent.Invocation) {
	buf, err := json.Marshal(inv)
	if err != nil {
		slog.Warn("telemetry: marshal failed",
			"error", err,
			"session_id", inv.SessionID,
			"model", inv.Model)
		return
	}
	buf = append(buf, '\n')

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.f.Write(buf); err != nil {
		slog.Warn("telemetry: write failed",
			"error", err,
			"session_id", inv.SessionID,
			"model", inv.Model)
	}
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
