// Package meetings models recorded meetings as a three-stage pipeline:
// audio captures, raw transcripts, and structured notes (decisions,
// action items, key topics) live as markdown files under
// vault/meetings/{audio,transcripts,notes}/. The store is a thin
// read/write scanner — most notes land in the vault via the existing
// ingest pipeline (audio → Deepgram → transcript) or by hand-drop;
// the only mutation this store performs is SetStage for the
// dashboard's per-card stage advance.
//
// Stage rename history: this used to be a two-stage pipeline of
// transcript → summary. The unified dashboard ships three stages
// (Transcript, Audio, Note) so the old "summary" value is now an
// alias for "note" — see stage.Normalize for the migration path.
package meetings

import (
	"time"

	"github.com/kriswong/corticalstack/internal/stage"
)

// Stage re-exports the canonical stage.Stage values used for meeting
// pipeline records. The constants below are copies of the package-
// stage equivalents kept here so existing call sites that used to
// reference meetings.StageTranscript keep compiling.
type Stage = stage.Stage

const (
	StageTranscript = stage.StageTranscript
	StageAudio      = stage.StageAudio
	StageNote       = stage.StageNote
	// StageSummary is the legacy alias for StageNote. Retained as a
	// constant only for backward-compat with callers that still
	// import the symbol; new code should use StageNote directly.
	StageSummary = stage.StageNote
)

// AllStages returns every stage in canonical order.
func AllStages() []Stage {
	return stage.AllStages(stage.EntityMeeting)
}

// IsValidStage reports whether s names a real stage. Accepts the
// legacy "summary" value via stage.Normalize so on-disk notes that
// predate the rename still classify correctly.
func IsValidStage(s string) bool {
	if s == "" {
		return false
	}
	for _, v := range AllStages() {
		if string(v) == s {
			return true
		}
	}
	// Legacy alias: "summary" → StageNote.
	return s == "summary"
}

// Meeting is one stage-tagged note in the vault. The same meeting may
// appear twice in a List() result if both a transcript and a summary
// note exist for it (linked by SourceID); the dashboard groups them.
type Meeting struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Stage      Stage     `json:"stage"`
	Path       string    `json:"path"`
	SourceID   string    `json:"source_id,omitempty"`
	SourcePath string    `json:"source_path,omitempty"`
	Projects   []string  `json:"projects,omitempty"`
	Created    time.Time `json:"created"`
	Updated    time.Time `json:"updated,omitempty"`
}
