package transformers

import (
	"path/filepath"
	"strings"

	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/pipeline"
)

// DeepgramTransformer handles audio files by calling the Deepgram REST API.
// The actual HTTP logic lives in internal/integrations/deepgram.go; this
// transformer only wraps that integration as a Transformer.
type DeepgramTransformer struct {
	Client *integrations.DeepgramClient
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

	result, err := t.Client.Transcribe(audio, mime)
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

	return &pipeline.TextDocument{
		ID:      identifierFor(input),
		Source:  "deepgram",
		Title:   title,
		Date:    fileModTime(input.Path),
		Content: result.Transcript,
		Metadata: mergeMeta(input.Metadata, map[string]string{
			"input_file":     input.Path,
			"audio_duration": result.DurationStr(),
		}),
	}, nil
}
