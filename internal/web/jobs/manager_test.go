package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kriswong/corticalstack/internal/intent"
	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/web/sse"
)

// --- Stubs that satisfy the unexported interfaces ---

type stubPipeline struct {
	mu             sync.Mutex
	transformFn    func(input *pipeline.RawInput) (*pipeline.TextDocument, string, error)
	extractFn      func(ctx context.Context, doc *pipeline.TextDocument, cfg pipeline.ExtractionConfig, name string) *pipeline.ProcessResult
	extractDelay   time.Duration
	transformDelay time.Duration
}

func (s *stubPipeline) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, string, error) {
	if s.transformDelay > 0 {
		time.Sleep(s.transformDelay)
	}
	if s.transformFn != nil {
		return s.transformFn(input)
	}
	return &pipeline.TextDocument{
		Source:  "text",
		Title:   "stub",
		Content: "stub content",
	}, "stub-transformer", nil
}

func (s *stubPipeline) ExtractAndRoute(ctx context.Context, doc *pipeline.TextDocument, cfg pipeline.ExtractionConfig, name string) *pipeline.ProcessResult {
	if s.extractDelay > 0 {
		// Sleep respecting ctx cancellation.
		select {
		case <-time.After(s.extractDelay):
		case <-ctx.Done():
			return &pipeline.ProcessResult{
				Document:    doc,
				Transformer: name,
				Outputs:     map[string]string{},
				Errors:      []string{"canceled: " + ctx.Err().Error()},
			}
		}
	}
	if s.extractFn != nil {
		return s.extractFn(ctx, doc, cfg, name)
	}
	return &pipeline.ProcessResult{
		Document:    doc,
		Transformer: name,
		Outputs:     map[string]string{"vault-note": "notes/stub.md"},
	}
}

type stubClassifier struct {
	classifyFn func(ctx context.Context, doc *pipeline.TextDocument, activeProjects []*projects.Project) (*intent.PreviewResult, error)
}

func (s *stubClassifier) Classify(ctx context.Context, doc *pipeline.TextDocument, activeProjects []*projects.Project) (*intent.PreviewResult, error) {
	if s.classifyFn != nil {
		return s.classifyFn(ctx, doc, activeProjects)
	}
	return &intent.PreviewResult{
		Intention:  intent.Learning,
		Confidence: 0.9,
		Summary:    "stub preview",
	}, nil
}

type stubProjects struct {
	list []*projects.Project
}

func (s *stubProjects) List() []*projects.Project { return s.list }

// newTestManager wires Manager with stubs and returns it plus the
// stubs for direct manipulation.
func newTestManager(t *testing.T) (*Manager, *stubPipeline, *stubClassifier, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	pipe := &stubPipeline{}
	cls := &stubClassifier{}
	proj := &stubProjects{}
	bus := sse.NewEventBus()

	m := &Manager{
		pipe:       pipe,
		bus:        bus,
		classifier: cls,
		projects:   proj,
		rootCtx:    ctx,
		jobs:       make(map[string]*Job),
	}
	return m, pipe, cls, cancel
}

// waitForStatus polls the job until it reaches one of the given
// statuses or the deadline expires. Returns the final observed status.
func waitForStatus(t *testing.T, m *Manager, jobID string, want ...Status) Status {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job := m.Get(jobID)
		if job != nil {
			for _, w := range want {
				if job.Status == w {
					return job.Status
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	job := m.Get(jobID)
	if job == nil {
		t.Fatalf("job %s not found", jobID)
	}
	t.Fatalf("job %s reached %q, wanted one of %v", jobID, job.Status, want)
	return ""
}

// --- Submit path tests ---

func TestSubmitHappyPath(t *testing.T) {
	m, _, _, cancel := newTestManager(t)
	defer cancel()

	job, err := m.Submit("test", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("hi")})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if job.ID == "" {
		t.Error("job ID is empty")
	}

	waitForStatus(t, m, job.ID, StatusAwaitingConfirmation)
	got := m.Get(job.ID)
	if got.Preview == nil {
		t.Error("Preview not set")
	}
	if got.Transformer != "stub-transformer" {
		t.Errorf("Transformer = %q", got.Transformer)
	}
}

func TestSubmitTransformFail(t *testing.T) {
	m, pipe, _, cancel := newTestManager(t)
	defer cancel()

	pipe.transformFn = func(input *pipeline.RawInput) (*pipeline.TextDocument, string, error) {
		return nil, "", errors.New("no transformer for input")
	}

	job, err := m.Submit("bad", &pipeline.RawInput{})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitForStatus(t, m, job.ID, StatusFailed)
	got := m.Get(job.ID)
	if !contains(got.Error, "no transformer") {
		t.Errorf("Error = %q", got.Error)
	}
}

func TestSubmitClassifyFail(t *testing.T) {
	m, _, cls, cancel := newTestManager(t)
	defer cancel()

	cls.classifyFn = func(ctx context.Context, doc *pipeline.TextDocument, _ []*projects.Project) (*intent.PreviewResult, error) {
		return nil, errors.New("claude timeout")
	}

	job, _ := m.Submit("test", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("hi")})
	waitForStatus(t, m, job.ID, StatusFailed)
	got := m.Get(job.ID)
	if !contains(got.Error, "classification failed") {
		t.Errorf("Error = %q", got.Error)
	}
}

func TestSubmitAfterShutdownRejects(t *testing.T) {
	m, _, _, cancel := newTestManager(t)
	defer cancel()

	shutdownCtx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	_ = m.Shutdown(shutdownCtx)

	_, err := m.Submit("x", &pipeline.RawInput{})
	if !errors.Is(err, ErrShuttingDown) {
		t.Errorf("err = %v, want ErrShuttingDown", err)
	}
}

// --- Confirm path tests ---

func TestConfirmHappyPath(t *testing.T) {
	m, _, _, cancel := newTestManager(t)
	defer cancel()

	job, _ := m.Submit("x", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("hi")})
	waitForStatus(t, m, job.ID, StatusAwaitingConfirmation)

	if err := m.Confirm(job.ID, ConfirmPayload{Intention: "learning"}); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	waitForStatus(t, m, job.ID, StatusCompleted)
	got := m.Get(job.ID)
	if got.NotePath != "notes/stub.md" {
		t.Errorf("NotePath = %q", got.NotePath)
	}
}

func TestConfirmMissingJob(t *testing.T) {
	m, _, _, cancel := newTestManager(t)
	defer cancel()

	err := m.Confirm("nonexistent", ConfirmPayload{})
	if err == nil || !contains(err.Error(), "not found") {
		t.Errorf("err = %v", err)
	}
}

func TestConfirmWrongState(t *testing.T) {
	m, _, _, cancel := newTestManager(t)
	defer cancel()

	// Create a job directly in a non-awaiting state.
	m.mu.Lock()
	job := &Job{ID: "fake-1", Status: StatusTransforming}
	m.jobs[job.ID] = job
	m.mu.Unlock()

	err := m.Confirm(job.ID, ConfirmPayload{})
	if err == nil || !contains(err.Error(), "expected awaiting_confirmation") {
		t.Errorf("err = %v", err)
	}
}

func TestConfirmAfterShutdownRejects(t *testing.T) {
	m, _, _, cancel := newTestManager(t)
	defer cancel()

	job, _ := m.Submit("x", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("hi")})
	waitForStatus(t, m, job.ID, StatusAwaitingConfirmation)

	shutdownCtx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	_ = m.Shutdown(shutdownCtx)

	err := m.Confirm(job.ID, ConfirmPayload{})
	if !errors.Is(err, ErrShuttingDown) {
		t.Errorf("err = %v, want ErrShuttingDown", err)
	}
}

// --- Shutdown path tests ---

func TestShutdownZeroJobs(t *testing.T) {
	m, _, _, cancel := newTestManager(t)
	defer cancel()

	shutdownCtx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	if err := m.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown with no jobs: %v", err)
	}
}

func TestShutdownDrainsInflight(t *testing.T) {
	m, pipe, _, cancel := newTestManager(t)
	defer cancel()

	// Make transform slow so Submit is still running when Shutdown starts.
	pipe.transformDelay = 100 * time.Millisecond

	job, _ := m.Submit("x", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("hi")})

	shutdownCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
	defer c()
	if err := m.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown drain: %v", err)
	}

	// Job should have reached a terminal state.
	got := m.Get(job.ID)
	if got.Status != StatusAwaitingConfirmation && got.Status != StatusFailed {
		t.Errorf("after shutdown, job status = %q", got.Status)
	}
}

func TestShutdownTimeout(t *testing.T) {
	m, pipe, _, cancel := newTestManager(t)
	defer cancel()

	// Job sleeps longer than the shutdown timeout.
	pipe.transformDelay = 500 * time.Millisecond

	_, _ = m.Submit("slow", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("hi")})

	// Shutdown with a 50ms budget — should return the deadline error.
	shutdownCtx, c := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer c()
	err := m.Shutdown(shutdownCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

// --- Query path tests ---

func TestGetList(t *testing.T) {
	m, _, _, cancel := newTestManager(t)
	defer cancel()

	j1, _ := m.Submit("one", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("a")})
	// Sleep long enough to guarantee distinct CreatedAt timestamps on
	// systems with low time.Now() resolution (Windows).
	time.Sleep(20 * time.Millisecond)
	j2, _ := m.Submit("two", &pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("b")})

	if got := m.Get(j1.ID); got == nil || got.Label != "one" {
		t.Errorf("Get j1 = %+v", got)
	}
	if got := m.Get("nope"); got != nil {
		t.Errorf("Get missing = %+v", got)
	}

	list := m.List()
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2", len(list))
	}
	// List should be newest-first.
	if list[0].ID != j2.ID {
		t.Errorf("List[0] = %s, want %s (newest first)", list[0].ID, j2.ID)
	}
}

// --- Helpers ---

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
