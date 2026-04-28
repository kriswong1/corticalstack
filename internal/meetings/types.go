// Package meetings models recorded meetings as a three-stage pipeline:
// audio files awaiting transcription (vault/meetings/audio/), raw
// transcripts (vault/meetings/transcripts/), and structured notes
// (vault/meetings/notes/). A meeting may enter at either Audio (a
// dropped or uploaded audio file) or Transcript (pasted text / VTT)
// and progresses through Note once Claude extracts the summary.
//
// The store is a thin read/write scanner — files land in the vault
// via the ingest pipeline or by hand-drop. The only mutation this
// store performs is SetStage for the dashboard's per-card stage
// advance. Audio files are detected by extension (.mp3 / .wav /
// .m4a / .ogg / .flac / .webm); transcripts and notes are markdown
// with `stage` frontmatter. A transcript carrying a `source_audio`
// frontmatter pointing at a file in vault/meetings/audio/ is
// considered the same meeting that started at the Audio stage —
// List() suppresses the audio entry to avoid double-counting.
//
// Legacy alias: "summary" → "note".
package meetings

import (
	"time"

	"github.com/kriswong/corticalstack/internal/stage"
)

// Stage re-exports the canonical stage.Stage values used for meeting
// pipeline records.
type Stage = stage.Stage

const (
	StageAudio      = stage.StageAudio
	StageTranscript = stage.StageTranscript
	StageNote       = stage.StageNote
	// StageSummary is the legacy alias for StageNote.
	StageSummary = stage.StageNote
)

// AllStages returns every stage in canonical order.
func AllStages() []Stage {
	return stage.AllStages(stage.EntityMeeting)
}

// IsValidStage reports whether s names a real stage. Accepts the
// legacy "summary" value so on-disk notes still classify.
func IsValidStage(s string) bool {
	if s == "" {
		return false
	}
	for _, v := range AllStages() {
		if string(v) == s {
			return true
		}
	}
	return s == "summary"
}

// Meeting is one stage-tagged record in the vault. Most are markdown
// files (transcripts, notes); audio-stage meetings are the audio file
// itself with no markdown wrapper. The same meeting may appear twice
// in a List() result if both a transcript and a note exist for it
// (linked by SourceID); the dashboard groups them.
type Meeting struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Stage      Stage     `json:"stage"`
	Path       string    `json:"path"`
	SourceID   string    `json:"source_id,omitempty"`
	SourcePath string    `json:"source_path,omitempty"`
	// SourceAudio, when set on a transcript, is the vault-relative
	// path to the audio file the transcript was generated from. Used
	// by the store to suppress the matching audio entry so a meeting
	// that has progressed past Audio doesn't appear twice in List().
	SourceAudio string    `json:"source_audio,omitempty"`
	Projects    []string  `json:"projects,omitempty"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated,omitempty"`
}
