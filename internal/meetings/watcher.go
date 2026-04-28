package meetings

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/stage"
	"github.com/kriswong/corticalstack/internal/vault"
)

// Transcriber is the slice of *integrations.DeepgramClient the watcher
// needs. Defined here so tests can substitute a fake without touching
// the real HTTP client.
type Transcriber interface {
	Configured() bool
	Transcribe(audio []byte, mime string) (*integrations.TranscriptionResult, error)
}

// DefaultWatchInterval is the polling cadence for the drop-in watcher.
// Audio files don't appear that often and Deepgram calls aren't free,
// so a 30s tick is plenty responsive without burning API quota.
const DefaultWatchInterval = 30 * time.Second

// Watcher polls vault/meetings/audio/ for files that have no matching
// transcript and runs them through Deepgram in the background. Pairs
// with the UI-upload path: the user can either upload via the form
// (Deepgram transformer archives + transcribes inline) or drop a file
// straight into the canonical folder via Finder/Explorer (this watcher
// picks it up on the next tick).
//
// The watcher is best-effort. A failed transcription clears the
// in-flight bit so a later tick will retry. If Deepgram is unconfigured
// the watcher logs once at startup and otherwise no-ops on every tick.
type Watcher struct {
	store    *Store
	vault    *vault.Vault
	client   Transcriber
	interval time.Duration

	mu       sync.Mutex
	inFlight map[string]bool

	stopOnce sync.Once
	stop     chan struct{}
}

// NewWatcher constructs a watcher. Pass DefaultWatchInterval unless
// you need a different cadence (tests use a much shorter tick or call
// Tick directly).
func NewWatcher(store *Store, v *vault.Vault, client Transcriber, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = DefaultWatchInterval
	}
	return &Watcher{
		store:    store,
		vault:    v,
		client:   client,
		interval: interval,
		inFlight: make(map[string]bool),
		stop:     make(chan struct{}),
	}
}

// Run blocks until ctx is cancelled or Close is called, ticking on the
// configured interval. Spawn it on its own goroutine from main. The
// first tick fires immediately so a freshly-dropped file gets picked
// up without waiting a full interval.
func (w *Watcher) Run(ctx context.Context) {
	if w.client == nil || !w.client.Configured() {
		slog.Info("meetings.watcher: Deepgram not configured, drop-in transcription disabled")
		return
	}
	slog.Info("meetings.watcher: started", "interval", w.interval)
	w.Tick(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("meetings.watcher: stopped (context cancelled)")
			return
		case <-w.stop:
			slog.Info("meetings.watcher: stopped (Close)")
			return
		case <-ticker.C:
			w.Tick(ctx)
		}
	}
}

// Close signals Run to exit. Idempotent. Safe to call before Run starts.
func (w *Watcher) Close() {
	w.stopOnce.Do(func() { close(w.stop) })
}

// Tick performs a single sweep: list meetings, find Audio-stage entries
// that aren't already in-flight, and kick off transcription for each.
// Each transcription runs in its own goroutine so a slow Deepgram call
// doesn't block the next tick or other in-flight files.
func (w *Watcher) Tick(ctx context.Context) {
	list, err := w.store.List()
	if err != nil {
		slog.Warn("meetings.watcher: List failed", "error", err)
		return
	}
	for _, m := range list {
		if m.Stage != stage.StageAudio {
			continue
		}
		if !w.claim(m.Path) {
			continue
		}
		go w.process(ctx, m)
	}
}

// claim attempts to mark a path as in-flight. Returns false if another
// goroutine already holds the slot — caller should skip this file
// until the holder finishes.
func (w *Watcher) claim(path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.inFlight[path] {
		return false
	}
	w.inFlight[path] = true
	return true
}

// release clears the in-flight bit. Called from process on every exit
// path (success and failure) so a transient failure doesn't permanently
// pin the file.
func (w *Watcher) release(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.inFlight, path)
}

// process transcribes one audio file and writes the resulting transcript
// markdown. The audio file is left in place (under meetings/audio/);
// the transcript at meetings/transcripts/ carries source_audio
// frontmatter pointing back, which the store uses to suppress the
// Audio entry on the next List().
func (w *Watcher) process(ctx context.Context, m *Meeting) {
	defer w.release(m.Path)

	abs := filepath.Join(w.vault.Path(), m.Path)
	audio, err := os.ReadFile(abs)
	if err != nil {
		slog.Warn("meetings.watcher: read audio failed", "path", m.Path, "error", err)
		return
	}

	mime := audioMimeForExt(m.Path)

	slog.Info("meetings.watcher: transcribing", "path", m.Path, "bytes", len(audio))
	result, err := w.client.Transcribe(audio, mime)
	if err != nil {
		slog.Warn("meetings.watcher: transcribe failed", "path", m.Path, "error", err)
		return
	}

	relPath, err := w.writeTranscript(m, result)
	if err != nil {
		slog.Warn("meetings.watcher: write transcript failed", "path", m.Path, "error", err)
		return
	}
	slog.Info("meetings.watcher: transcribed", "audio", m.Path, "transcript", relPath, "duration", result.DurationStr())

	// Honour ctx for cleanliness, even though the per-file work is
	// already done — keeps Run's shutdown signal observable from the
	// process goroutine if a follow-up task gets added later.
	_ = ctx
}

// writeTranscript places the transcript markdown at
// meetings/transcripts/<date>_<slug>.md with source_audio frontmatter
// linking back to the original audio file. The shape mirrors what the
// Deepgram-transformer ingest path produces (template.go's
// buildFrontmatter pulls source_audio / audio_duration from metadata)
// so the store's claim-suppression logic treats both flows the same.
func (w *Watcher) writeTranscript(m *Meeting, result *integrations.TranscriptionResult) (string, error) {
	slug := vault.Slugify(m.Title)
	if slug == "" {
		slug = "untitled"
	}
	if len(slug) > 60 {
		slug = slug[:60]
	}
	date := time.Now().Format("2006-01-02")
	rel := filepath.ToSlash(filepath.Join(meetingsDir, "transcripts", fmt.Sprintf("%s_%s.md", date, slug)))

	body := strings.TrimSpace(result.Transcript) + "\n"
	note := &vault.Note{
		Frontmatter: map[string]interface{}{
			"id":             m.ID,
			"title":          m.Title,
			"stage":          string(stage.StageTranscript),
			"source":         "deepgram",
			"source_audio":   m.Path,
			"audio_duration": result.DurationStr(),
			"created":        time.Now().Format(time.RFC3339),
			"updated":        time.Now().Format(time.RFC3339),
		},
		Body: body,
	}
	if err := w.vault.WriteNote(rel, note); err != nil {
		return "", err
	}
	return rel, nil
}

// audioMimeForExt mirrors the helper in pipeline/transformers/audio_io.go.
// Duplicated to avoid pulling the transformers package (and its dep web)
// into the meetings domain.
func audioMimeForExt(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".m4a":
		return "audio/mp4"
	case ".ogg":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".webm":
		return "audio/webm"
	}
	return "audio/mpeg"
}
