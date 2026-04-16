// Package meetings models recorded meetings as a two-stage pipeline:
// raw transcripts and structured notes (decisions, action items, key
// topics) live as markdown files under vault/meetings/{transcripts,
// notes}/. The store is a thin read/write scanner — most notes land
// in the vault via the existing ingest pipeline (audio → Deepgram →
// transcript) or by hand-drop; the only mutation this store performs
// is SetStage for the dashboard's per-card stage advance.
//
// Legacy aliases: "summary" → "note", "audio" → "transcript".
package meetings

import (
	"time"

	"github.com/kriswong/corticalstack/internal/stage"
)

// Stage re-exports the canonical stage.Stage values used for meeting
// pipeline records.
type Stage = stage.Stage

const (
	StageTranscript = stage.StageTranscript
	StageNote       = stage.StageNote
	// StageSummary is the legacy alias for StageNote.
	StageSummary = stage.StageNote
)

// AllStages returns every stage in canonical order.
func AllStages() []Stage {
	return stage.AllStages(stage.EntityMeeting)
}

// IsValidStage reports whether s names a real stage. Accepts legacy
// "summary" and "audio" values so on-disk notes still classify.
func IsValidStage(s string) bool {
	if s == "" {
		return false
	}
	for _, v := range AllStages() {
		if string(v) == s {
			return true
		}
	}
	return s == "summary" || s == "audio"
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
