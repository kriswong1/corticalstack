package shapeup

import "sync"

// AdvanceProgress tracks the real-time state of an in-flight advance
// call. The pipeline UI polls this via GET /api/shapeup/threads/{id}/progress.
type AdvanceProgress struct {
	Turn     int    `json:"turn"`
	MaxTurns int    `json:"max_turns"`
	Status   string `json:"status"` // "generating" | "idle" | "done" | "error"
	Stage    string `json:"stage"`  // target stage being generated
}

// ProgressTracker is a thread-safe map of thread ID → current advance
// progress. Entries are created when an advance starts and removed
// (or set to "done"/"error") when it finishes.
type ProgressTracker struct {
	mu sync.RWMutex
	m  map[string]*AdvanceProgress
}

// NewProgressTracker creates an empty tracker.
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{m: make(map[string]*AdvanceProgress)}
}

// Start registers a new in-flight advance for a thread.
func (t *ProgressTracker) Start(threadID, stage string, maxTurns int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[threadID] = &AdvanceProgress{
		Turn:     0,
		MaxTurns: maxTurns,
		Status:   "generating",
		Stage:    stage,
	}
}

// SetTurn updates the current turn count for a thread's advance.
func (t *ProgressTracker) SetTurn(threadID string, turn int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.m[threadID]; ok {
		p.Turn = turn
	}
}

// Finish marks an advance as done or error and keeps the entry
// briefly so the next poll picks up the final state.
func (t *ProgressTracker) Finish(threadID, status string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.m[threadID]; ok {
		p.Status = status
	}
}

// Clear removes the progress entry for a thread. Called after the
// frontend has seen the "done" or "error" state.
func (t *ProgressTracker) Clear(threadID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, threadID)
}

// Get returns the current progress for a thread, or nil if no
// advance is in flight.
func (t *ProgressTracker) Get(threadID string) *AdvanceProgress {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if p, ok := t.m[threadID]; ok {
		cp := *p
		return &cp
	}
	return nil
}

// DefaultTracker is the process-wide progress tracker, wired once
// in main and shared between the advance handler and the progress
// polling endpoint.
var DefaultTracker = NewProgressTracker()
