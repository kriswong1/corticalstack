// Package stage defines the per-entity pipeline stages used by the
// unified dashboard. Every entity in the dashboard's row-2 cards
// (Product, Meetings, Documents, Prototypes) reports its progress as
// one of a fixed, ordered list of stages. The values are kept here so
// the dashboard aggregator, the per-entity stores, and the HTTP
// handlers all agree on the same canonical names without copy-pasted
// constants drifting across packages.
//
// The stage names are stable wire values — they ship in JSON over
// /api/dashboard and /api/cards/{type} and are matched by the React
// frontend, so renaming any of them is a breaking change.
//
// Legacy values (raw, summary, draft, exported) are accepted by
// Normalize so already-on-disk frontmatter and store records keep
// reading correctly while the new values become canonical for any
// future writes.
package stage

import (
	"fmt"
	"strings"
)

// EntityType names one of the four dashboard pipelines.
type EntityType string

const (
	EntityProduct   EntityType = "product"
	EntityMeeting   EntityType = "meeting"
	EntityDocument  EntityType = "document"
	EntityPrototype EntityType = "prototype"
)

// AllEntityTypes returns every supported entity type in canonical order.
// Order matches the row-2 card order on the dashboard.
func AllEntityTypes() []EntityType {
	return []EntityType{EntityProduct, EntityMeeting, EntityDocument, EntityPrototype}
}

// Stage is the wire-stable identifier of one pipeline stage.
type Stage string

// Product stages — five-stage ShapeUp arc. The on-disk shapeup store
// still uses "raw" for the first stage (artifactRelPath writes to
// vault/product/raw/), so Normalize maps "raw" → "idea" for any
// caller that wants the dashboard-facing name. The reverse mapping
// (idea → raw) lives in the dashboard aggregator's stage-counting code
// where it bridges the two namespaces — see internal/dashboard
// /aggregator.go.
const (
	StageIdea       Stage = "idea"
	StageFrame      Stage = "frame"
	StageShape      Stage = "shape"
	StageBreadboard Stage = "breadboard"
	StagePitch      Stage = "pitch"
)

// Meeting stages — three-stage capture flow. A meeting may enter at
// either StageAudio (raw audio file dropped or uploaded; awaiting
// transcription) or StageTranscript (transcript text supplied directly,
// e.g. pasted notes / VTT). Both progress through StageNote once Claude
// extracts the structured summary. "summary" is a legacy value still on
// disk; Normalize folds it into "note".
const (
	StageAudio      Stage = "audio"
	StageTranscript Stage = "transcript"
	StageNote       Stage = "note"
)

// Document stages — two-stage flow: raw input material in, refined
// note out. "need", "in_progress", "final" are legacy aliases from
// the first stage definition; Normalize maps them.
const (
	StageInput    Stage = "input"
	StageDocNote  Stage = "note" // alias — same wire value as meeting StageNote
)

// Prototype stages — three-stage flow sourced from a ShapeUp
// breadboard artifact. "need" is the legacy alias for "breadboard".
const (
	StageProtoBreadboard Stage = "breadboard" // alias — same wire value as product StageBreadboard
	StageInProgress      Stage = "in_progress"
	StageFinal           Stage = "final"
)

// AllStages returns the canonical, ordered list of stages for one
// entity type. Order is the on-card display order — earliest stage
// first. Returns nil for an unknown entity type so callers can detect
// misuse without panicking.
func AllStages(entity EntityType) []Stage {
	switch entity {
	case EntityProduct:
		return []Stage{StageIdea, StageFrame, StageShape, StageBreadboard, StagePitch}
	case EntityMeeting:
		return []Stage{StageAudio, StageTranscript, StageNote}
	case EntityDocument:
		return []Stage{StageInput, StageNote}
	case EntityPrototype:
		return []Stage{StageProtoBreadboard, StageInProgress, StageFinal}
	}
	return nil
}

// Validate reports whether s is a recognized stage for the given
// entity type. Empty string is invalid — callers wanting a default
// should use FallbackStage.
func Validate(entity EntityType, s string) bool {
	for _, v := range AllStages(entity) {
		if string(v) == s {
			return true
		}
	}
	return false
}

// FallbackStage returns the default stage for items missing a stage
// field. Each entity type has a sensible "fresh / not started" stage
// that the dashboard renders without surprise. For Product items the
// fallback is Idea (matches the on-disk "raw" stage after Normalize).
func FallbackStage(entity EntityType) Stage {
	switch entity {
	case EntityProduct:
		return StageIdea
	case EntityMeeting:
		// Transcript is the fallback (not Audio) so a meeting note
		// missing a stage frontmatter still renders alongside other
		// transcript-stage entries — most legacy data is text.
		return StageTranscript
	case EntityDocument:
		return StageInput
	case EntityPrototype:
		return StageProtoBreadboard
	}
	return ""
}

// Normalize maps a raw frontmatter / store value to the canonical
// dashboard stage for the given entity type. Empty strings and
// unrecognized values fall through to FallbackStage so on-disk notes
// without a stage field still render in the right place.
//
// Legacy aliases (kept for backward compat with on-disk data):
//   - product:   "raw"      → "idea"
//   - meeting:   "summary"  → "note"
//   - prototype: "draft"    → "in_progress"
//   - prototype: "exported" → "final"
//
// Normalization is case-insensitive on the input — frontmatter
// authors write "Idea" and "IDEA" interchangeably and we don't want
// stage assignment to depend on shift-key state.
func Normalize(entity EntityType, raw string) Stage {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return FallbackStage(entity)
	}
	switch entity {
	case EntityProduct:
		if v == "raw" {
			return StageIdea
		}
	case EntityMeeting:
		// "summary" → "note" remains a legacy alias. "audio" used to
		// be folded into Transcript when audio wasn't a real stage;
		// it is now canonical, so the switch is a no-op for that
		// value (the canonical Validate path below picks it up).
		if v == "summary" {
			return StageNote
		}
	case EntityDocument:
		switch v {
		case "need":
			return StageInput
		case "in_progress", "final":
			return StageDocNote
		}
	case EntityPrototype:
		switch v {
		case "need":
			return StageProtoBreadboard
		case "draft":
			return StageInProgress
		case "exported":
			return StageFinal
		}
	}
	if Validate(entity, v) {
		return Stage(v)
	}
	return FallbackStage(entity)
}

// Parse is the strict variant of Normalize used by HTTP handlers
// validating user-supplied stage values. Returns an error for empty
// strings, unknown entity types, and unrecognized stages so the
// caller can reply 400 instead of silently coercing the value.
func Parse(entity EntityType, raw string) (Stage, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return "", fmt.Errorf("stage required")
	}
	if AllStages(entity) == nil {
		return "", fmt.Errorf("unknown entity type: %q", entity)
	}
	if !Validate(entity, v) {
		// Accept the same legacy aliases Normalize accepts so a
		// strict POST /api/items/{type}/{id}/stage with raw="raw"
		// still works for back-compat callers.
		s := Normalize(entity, v)
		if s != FallbackStage(entity) || v == string(FallbackStage(entity)) {
			return s, nil
		}
		return "", fmt.Errorf("unknown %s stage: %q", entity, raw)
	}
	return Stage(v), nil
}
