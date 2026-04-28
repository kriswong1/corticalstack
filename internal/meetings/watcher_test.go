package meetings

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/vault"
)

// fakeTranscriber returns a canned TranscriptionResult and counts calls
// so concurrency-guard tests can assert no double-processing.
type fakeTranscriber struct {
	configured bool
	calls      int32
	transcript string
	delay      time.Duration
	fail       bool
}

func (f *fakeTranscriber) Configured() bool { return f.configured }

func (f *fakeTranscriber) Transcribe(audio []byte, mime string) (*integrations.TranscriptionResult, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.fail {
		return nil, errFakeFail
	}
	return &integrations.TranscriptionResult{
		Transcript: f.transcript,
		Duration:   42,
	}, nil
}

var errFakeFail = errFake("transcribe failed")

type errFake string

func (e errFake) Error() string { return string(e) }

// newWatcherTestEnv builds a meetings store + watcher backed by a
// temp vault with the meetings folder pre-created.
func newWatcherTestEnv(t *testing.T, tx *fakeTranscriber) (*Watcher, *Store, string) {
	t.Helper()
	dir := t.TempDir()
	v := vault.New(dir)
	s := New(v)
	if err := s.EnsureFolder(); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}
	w := NewWatcher(s, v, tx, 50*time.Millisecond)
	return w, s, dir
}

func TestWatcherTickTranscribesAudioAndSuppressesIt(t *testing.T) {
	tx := &fakeTranscriber{configured: true, transcript: "[00:00:01] hello world"}
	w, s, dir := newWatcherTestEnv(t, tx)

	audioRel := filepath.Join("meetings", "audio", "2026-04-25_drop-in.mp3")
	if err := os.WriteFile(filepath.Join(dir, audioRel), []byte("fake-audio-bytes"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	w.Tick(context.Background())

	// Tick spawns goroutines; wait briefly for the transcribe + write
	// to complete. Poll List() so the test isn't sleep-bound on slow
	// machines.
	deadline := time.Now().Add(2 * time.Second)
	var got []*Meeting
	for time.Now().Before(deadline) {
		out, err := s.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(out) == 1 && out[0].Stage == StageTranscript {
			got = out
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got == nil {
		t.Fatalf("transcript never appeared in List() within deadline")
	}

	if got[0].SourceAudio != "meetings/audio/2026-04-25_drop-in.mp3" {
		t.Errorf("source_audio = %q", got[0].SourceAudio)
	}

	// Read the transcript file to confirm the body landed.
	abs := filepath.Join(dir, got[0].Path)
	body, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if !strings.Contains(string(body), "hello world") {
		t.Errorf("transcript body missing transcribed text: %s", body)
	}
}

func TestWatcherTickSkipsUnconfiguredClient(t *testing.T) {
	// Run() short-circuits on unconfigured client; Tick() itself doesn't
	// gate on Configured (Run does), but we exercise the no-audio case
	// here to confirm an empty folder is a no-op.
	tx := &fakeTranscriber{configured: false}
	w, _, _ := newWatcherTestEnv(t, tx)
	w.Tick(context.Background())
	if got := atomic.LoadInt32(&tx.calls); got != 0 {
		t.Errorf("calls = %d, want 0 (no audio files dropped)", got)
	}
}

func TestWatcherInFlightGuardPreventsDoubleTranscribe(t *testing.T) {
	// Slow Transcribe so the second Tick fires while the first is
	// still in flight. The guard must skip the file.
	tx := &fakeTranscriber{
		configured: true,
		transcript: "concurrent",
		delay:      200 * time.Millisecond,
	}
	w, _, dir := newWatcherTestEnv(t, tx)
	audioRel := filepath.Join("meetings", "audio", "2026-04-25_busy.mp3")
	if err := os.WriteFile(filepath.Join(dir, audioRel), []byte("bytes"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	w.Tick(context.Background())
	// Fire a second tick while the first transcribe is sleeping.
	time.Sleep(20 * time.Millisecond)
	w.Tick(context.Background())

	// Wait for the slow transcribe to finish.
	time.Sleep(400 * time.Millisecond)

	if got := atomic.LoadInt32(&tx.calls); got != 1 {
		t.Errorf("calls = %d, want 1 (in-flight guard should suppress second tick)", got)
	}
}

func TestWatcherTickReleasesOnFailure(t *testing.T) {
	// A failed Transcribe must clear the in-flight bit so a retry on
	// the next tick can attempt again. Easiest assertion: same file
	// gets transcribed twice across two ticks.
	tx := &fakeTranscriber{configured: true, transcript: "x", fail: true}
	w, _, dir := newWatcherTestEnv(t, tx)
	audioRel := filepath.Join("meetings", "audio", "2026-04-25_retry.mp3")
	if err := os.WriteFile(filepath.Join(dir, audioRel), []byte("bytes"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	w.Tick(context.Background())
	time.Sleep(50 * time.Millisecond)
	w.Tick(context.Background())
	time.Sleep(50 * time.Millisecond)

	if got := atomic.LoadInt32(&tx.calls); got != 2 {
		t.Errorf("calls = %d, want 2 (in-flight should release on failure for retry)", got)
	}
}
