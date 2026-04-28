package transformers

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/vault"
)

// DeepgramTransformer handles audio files by calling the Deepgram REST API.
// The actual HTTP logic lives in internal/integrations/deepgram.go; this
// transformer wraps that integration AND archives the original audio
// bytes to vault/meetings/audio/ so the file is visible in the meetings
// dashboard at the Audio stage and remains available for re-transcription.
type DeepgramTransformer struct {
	Client *integrations.DeepgramClient
	// Vault is optional. When set, the original audio bytes are
	// preserved at vault/meetings/audio/<date>_<slug>.<ext> before
	// transcription so the meeting record carries a stable Audio
	// artifact. When nil, the transformer still transcribes but does
	// not archive — useful for tests and headless callers.
	Vault *vault.Vault
}

func (t *DeepgramTransformer) Name() string { return "deepgram" }

func (t *DeepgramTransformer) CanHandle(input *pipeline.RawInput) bool {
	ext := strings.ToLower(filepath.Ext(input.Path))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(input.Filename))
	}
	switch ext {
	case ".mp3", ".wav", ".m4a", ".ogg", ".flac", ".webm":
		return true
	}
	return false
}

func (t *DeepgramTransformer) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, error) {
	if t.Client == nil || !t.Client.Configured() {
		return nil, ErrDeepgramNotConfigured
	}

	audio, mime, err := loadAudio(input)
	if err != nil {
		return nil, err
	}

	title := input.Title
	if title == "" {
		name := input.Filename
		if name == "" {
			name = filepath.Base(input.Path)
		}
		title = strings.TrimSuffix(name, filepath.Ext(name))
	}

	// Archive the original audio bytes under meetings/audio/ before
	// transcribing. If Deepgram fails, the file still surfaces as an
	// Audio-stage meeting in the dashboard so the user can retry. The
	// archived path is threaded through metadata so the destination
	// can record `source_audio` frontmatter on the transcript.
	archivedPath := t.archiveAudio(input, audio, title)

	result, err := t.Client.Transcribe(audio, mime)
	if err != nil {
		return nil, err
	}

	meta := map[string]string{
		"input_file":     input.Path,
		"audio_duration": result.DurationStr(),
	}
	if archivedPath != "" {
		meta["source_audio"] = archivedPath
	}

	return &pipeline.TextDocument{
		ID:       identifierFor(input),
		Source:   "deepgram",
		Title:    title,
		Date:     fileModTime(input.Path),
		Content:  result.Transcript,
		Metadata: mergeMeta(input.Metadata, meta),
	}, nil
}

// archiveAudio writes the audio bytes under vault/meetings/audio/ and
// returns the vault-relative path. Returns "" when archival is
// disabled (no vault configured) or fails — in either case the
// caller proceeds with transcription, so a write failure logs but
// does not block the user.
func (t *DeepgramTransformer) archiveAudio(input *pipeline.RawInput, audio []byte, title string) string {
	if t.Vault == nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(input.Filename))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(input.Path))
	}
	if ext == "" {
		ext = ".mp3"
	}
	slug := vault.Slugify(title)
	if slug == "" {
		slug = "untitled"
	}
	if len(slug) > 60 {
		slug = slug[:60]
	}
	date := time.Now().Format("2006-01-02")
	rel := filepath.ToSlash(filepath.Join("meetings", "audio", fmt.Sprintf("%s_%s%s", date, slug, ext)))
	abs := filepath.Join(t.Vault.Path(), rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		slog.Warn("deepgram: archive mkdir failed", "path", abs, "error", err)
		return ""
	}
	if err := os.WriteFile(abs, audio, 0o600); err != nil {
		slog.Warn("deepgram: archive write failed", "path", abs, "error", err)
		return ""
	}
	return rel
}
