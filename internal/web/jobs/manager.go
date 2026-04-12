// Package jobs manages async pipeline runs. Each submission gets a Job
// with a goroutine, progress events via the SSE bus, and a terminal
// status (completed or failed).
//
// v2 jobs are two-phase: Submit() runs Transform + Classify and pauses at
// awaiting_confirmation. The handler calls Confirm() when the user accepts
// or edits Claude's proposed intention/projects, which resumes extraction
// and routing to completion.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/kriswong/corticalstack/internal/intent"
	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/web/sse"
)

// ErrShuttingDown is returned by Submit and Confirm when the manager
// has begun shutdown and is no longer accepting new work.
var ErrShuttingDown = errors.New("jobs: manager shutting down")

// pipelineRunner is the subset of *pipeline.Pipeline that Manager needs.
// Exists as an interface so tests can substitute a stub.
type pipelineRunner interface {
	Transform(input *pipeline.RawInput) (*pipeline.TextDocument, string, error)
	ExtractAndRoute(ctx context.Context, doc *pipeline.TextDocument, cfg pipeline.ExtractionConfig, transformerName string) *pipeline.ProcessResult
}

// docClassifier is the subset of *intent.ClaudeClassifier that Manager needs.
type docClassifier interface {
	Classify(ctx context.Context, doc *pipeline.TextDocument, activeProjects []*projects.Project) (*intent.PreviewResult, error)
}

// projectLister is the subset of *projects.Store that Manager needs.
type projectLister interface {
	List() []*projects.Project
}

// Status is a job's terminal-or-running state.
type Status string

const (
	StatusPending               Status = "pending"
	StatusTransforming          Status = "transforming"
	StatusClassifying           Status = "classifying"
	StatusAwaitingConfirmation  Status = "awaiting_confirmation"
	StatusExtracting            Status = "extracting"
	StatusRouting               Status = "routing"
	StatusCompleted             Status = "completed"
	StatusFailed                Status = "failed"
)

// Job is one pipeline invocation tracked by the manager.
type Job struct {
	ID          string                 `json:"id"`
	Label       string                 `json:"label"`
	Status      Status                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   time.Time              `json:"started_at,omitempty"`
	EndedAt     time.Time              `json:"ended_at,omitempty"`
	Error       string                 `json:"error,omitempty"`
	NotePath    string                 `json:"note_path,omitempty"`
	Messages    []string               `json:"messages,omitempty"`
	Preview     *intent.PreviewResult  `json:"preview,omitempty"`
	Transformer string                 `json:"transformer,omitempty"`

	// Internal state (not exposed in JSON except where needed)
	doc *pipeline.TextDocument
}

// Event describes a progress update for a single job.
type Event struct {
	JobID   string `json:"job_id"`
	Status  Status `json:"status"`
	Message string `json:"message"`
}

// ConfirmPayload is what the dashboard sends on POST /api/jobs/{id}/confirm.
type ConfirmPayload struct {
	Intention  string   `json:"intention"`
	ProjectIDs []string `json:"project_ids"`
	Why        string   `json:"why"`
	Title      string   `json:"title"`
}

// Manager runs pipeline jobs asynchronously.
type Manager struct {
	pipe       pipelineRunner
	bus        *sse.EventBus
	classifier docClassifier
	projects   projectLister

	// rootCtx is derived from main's shutdown context. Per-job contexts
	// are children of this, so canceling rootCtx cancels every job.
	rootCtx      context.Context
	wg           sync.WaitGroup
	shuttingDown atomic.Bool

	mu   sync.RWMutex
	jobs map[string]*Job
}

// New creates a manager bound to a pipeline, classifier, projects store, and bus.
// ctx is the root context whose cancellation propagates to every running job.
func New(
	ctx context.Context,
	pipe *pipeline.Pipeline,
	bus *sse.EventBus,
	classifier *intent.ClaudeClassifier,
	ps *projects.Store,
) *Manager {
	return &Manager{
		pipe:       pipe,
		bus:        bus,
		classifier: classifier,
		projects:   ps,
		rootCtx:    ctx,
		jobs:       make(map[string]*Job),
	}
}

// Submit creates a new job and kicks off Transform + Classify in a goroutine.
// The job pauses at awaiting_confirmation until Confirm() is called.
// Returns (nil, ErrShuttingDown) if the manager is no longer accepting work.
func (m *Manager) Submit(label string, input *pipeline.RawInput) (*Job, error) {
	if m.shuttingDown.Load() {
		return nil, ErrShuttingDown
	}
	job := &Job{
		ID:        uuid.NewString(),
		Label:     label,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	jobCtx, cancel := context.WithCancel(m.rootCtx)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		m.runPreview(jobCtx, job, input)
	}()
	return job, nil
}

// Confirm resumes a job paused at awaiting_confirmation with the user's choices.
// Returns an error if the job is missing, not in the expected state, or the
// manager is shutting down.
func (m *Manager) Confirm(jobID string, payload ConfirmPayload) error {
	if m.shuttingDown.Load() {
		return ErrShuttingDown
	}
	m.mu.Lock()
	job, ok := m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("job not found: %s", jobID)
	}
	if job.Status != StatusAwaitingConfirmation {
		m.mu.Unlock()
		return fmt.Errorf("job %s is in state %q, expected awaiting_confirmation", jobID, job.Status)
	}
	doc := job.doc
	m.mu.Unlock()

	jobCtx, cancel := context.WithCancel(m.rootCtx)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		m.runConfirm(jobCtx, job, doc, payload)
	}()
	return nil
}

// Shutdown prevents new jobs from being accepted and waits for all
// in-flight jobs to finish or for ctx to expire. Returns ctx.Err() on
// timeout, nil if all jobs drained cleanly.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.shuttingDown.Store(true)
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Get returns a job by ID, or nil if not found.
func (m *Manager) Get(id string) *Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs[id]
}

// List returns a snapshot of all jobs, newest first.
func (m *Manager) List() []*Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func (m *Manager) runPreview(ctx context.Context, job *Job, input *pipeline.RawInput) {
	job.StartedAt = time.Now()
	m.setStatus(job, StatusTransforming, "Transforming input")

	doc, transformerName, err := m.pipe.Transform(input)
	if err != nil {
		m.fail(job, err.Error())
		return
	}
	m.mu.Lock()
	job.doc = doc
	job.Transformer = transformerName
	m.mu.Unlock()
	m.publishProgress(job, fmt.Sprintf("Transformed via %s (%d chars)", transformerName, len(doc.Content)))

	m.setStatus(job, StatusClassifying, "Classifying intention")
	preview, err := m.classifier.Classify(ctx, doc, m.projects.List())
	if err != nil {
		m.fail(job, "classification failed: "+err.Error())
		return
	}
	m.mu.Lock()
	job.Preview = preview
	m.mu.Unlock()

	m.setStatus(job, StatusAwaitingConfirmation, fmt.Sprintf("Proposed intention: %s", preview.Intention))
	m.bus.Publish(sse.Event{
		Type: "job_preview",
		Data: map[string]interface{}{
			"job_id":  job.ID,
			"preview": preview,
		},
	})
}

func (m *Manager) runConfirm(ctx context.Context, job *Job, doc *pipeline.TextDocument, payload ConfirmPayload) {
	m.setStatus(job, StatusExtracting, "Extracting structured data")

	cfg := pipeline.DefaultConfigForSource(doc.Source)
	cfg.Intention = payload.Intention
	cfg.Projects = payload.ProjectIDs
	cfg.Why = payload.Why
	if payload.Title != "" {
		doc.Title = payload.Title
	}

	m.setStatus(job, StatusRouting, "Writing notes")
	result := m.pipe.ExtractAndRoute(ctx, doc, cfg, job.Transformer)

	for _, msg := range result.Errors {
		m.publishProgress(job, "warning: "+msg)
	}
	if path := result.Outputs["vault-note"]; path != "" {
		m.mu.Lock()
		job.NotePath = path
		m.mu.Unlock()
	}

	m.publishProgress(job, fmt.Sprintf("Wrote %d output(s)", len(result.Outputs)))
	m.complete(job, fmt.Sprintf("Processed via %s as %s", result.Transformer, payload.Intention))
}

func (m *Manager) setStatus(job *Job, status Status, message string) {
	m.mu.Lock()
	job.Status = status
	if message != "" {
		job.Messages = append(job.Messages, message)
	}
	m.mu.Unlock()

	m.bus.Publish(sse.Event{
		Type: "job_status",
		Data: Event{JobID: job.ID, Status: status, Message: message},
	})
}

func (m *Manager) publishProgress(job *Job, message string) {
	m.mu.Lock()
	job.Messages = append(job.Messages, message)
	m.mu.Unlock()

	m.bus.Publish(sse.Event{
		Type: "job_progress",
		Data: Event{JobID: job.ID, Status: job.Status, Message: message},
	})
}

func (m *Manager) complete(job *Job, message string) {
	m.mu.Lock()
	job.Status = StatusCompleted
	job.EndedAt = time.Now()
	job.Messages = append(job.Messages, message)
	m.mu.Unlock()

	m.bus.Publish(sse.Event{
		Type: "job_complete",
		Data: Event{JobID: job.ID, Status: StatusCompleted, Message: message},
	})
}

func (m *Manager) fail(job *Job, message string) {
	m.mu.Lock()
	job.Status = StatusFailed
	job.Error = message
	job.EndedAt = time.Now()
	job.Messages = append(job.Messages, "error: "+message)
	m.mu.Unlock()

	m.bus.Publish(sse.Event{
		Type: "job_failed",
		Data: Event{JobID: job.ID, Status: StatusFailed, Message: message},
	})
}
