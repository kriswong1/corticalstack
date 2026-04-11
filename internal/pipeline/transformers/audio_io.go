package transformers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// ErrDeepgramNotConfigured signals a missing Deepgram API key.
var ErrDeepgramNotConfigured = errors.New("deepgram API key not configured (set DEEPGRAM_API_KEY)")

// loadAudio returns the audio bytes and a MIME type for a RawInput.
func loadAudio(input *pipeline.RawInput) ([]byte, string, error) {
	if len(input.Content) > 0 {
		mime := input.MIMEType
		if mime == "" {
			mime = audioMimeForExt(input.Filename)
		}
		return input.Content, mime, nil
	}
	if input.Path == "" {
		return nil, "", fmt.Errorf("audio input has no path or content")
	}
	data, err := os.ReadFile(input.Path)
	if err != nil {
		return nil, "", err
	}
	mime := input.MIMEType
	if mime == "" {
		mime = audioMimeForExt(input.Path)
	}
	return data, mime, nil
}

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
