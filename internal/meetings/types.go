// Package meetings models recorded meetings as a two-stage pipeline:
// a raw transcript (from audio, VTT, or auto-generated) becomes a
// structured summary note (decisions, action items, key topics).
//
// Like shapeup and prds, meetings live as markdown notes in the vault
// with YAML frontmatter declaring the stage. The store is a thin
// read-only scanner — nothing here writes notes; meetings land in the
// vault via the existing ingest pipeline (audio → Deepgram, VTT, etc.)
// and a user or downstream destination promotes a transcript to a
// summary by writing a new file with stage=summary.
package meetings

import "time"

// Stage is one of the two meeting stages.
type Stage string

const (
	StageTranscript Stage = "transcript"
	StageSummary    Stage = "summary"
)

// AllStages returns every stage in canonical order.
func AllStages() []Stage {
	return []Stage{StageTranscript, StageSummary}
}

// IsValidStage reports whether s names a real stage.
func IsValidStage(s string) bool {
	for _, v := range AllStages() {
		if string(v) == s {
			return true
		}
	}
	return false
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
