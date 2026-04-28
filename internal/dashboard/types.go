// Package dashboard aggregates read-only snapshots of ingest, actions,
// projects, and the product pipeline for the /dashboard operating view.
//
// The aggregator is single-pass over each backing store per refresh, cached
// with a clock-based TTL, and resilient to individual store failures — on
// a failed recompute the cache returns the last successful snapshot with a
// Stale flag set so the frontend can show a non-blocking banner while the
// click-through affordances keep working.
package dashboard

import "time"

// IngestBucketArticles and friends are the canonical, ordered ingest
// types surfaced on the row-1 chart of the unified dashboard. The
// order here is the legend order — the frontend renders bars left-to-
// right in this sequence, so changes are user-visible.
//
// "Other" is a catch-all so vault folders that don't map cleanly into
// the five user-facing buckets (daily, audio, non-YouTube webpages)
// stay visible on the chart instead of disappearing.
const (
	BucketArticles    = "articles"
	BucketYouTube     = "youtube"
	BucketTranscripts = "transcripts"
	BucketDocuments   = "documents"
	BucketNotes       = "notes"
	BucketOther       = "other"
)

// IngestBucketOrder is the fixed left-to-right legend order. Mirrored
// in the React IngestChart component; do not change one without the
// other.
var IngestBucketOrder = []string{
	BucketArticles,
	BucketYouTube,
	BucketTranscripts,
	BucketDocuments,
	BucketNotes,
	BucketOther,
}

// Snapshot is the full response returned by GET /api/dashboard. A single
// fetch populates all widgets plus the freshness marker.
//
// ProductPipeline is retained as an alias of Pipelines.Product so the
// existing frontend keeps reading from /api/dashboard during the
// front-end rewrite (Phase 4). New consumers should read Pipelines.
type Snapshot struct {
	IngestActivity  IngestWidget   `json:"ingest_activity"`
	Actions         ActionsWidget  `json:"actions"`
	ActiveProjects  ProjectsWidget `json:"active_projects"`
	ProductPipeline PipelineWidget `json:"product_pipeline"`
	Pipelines       PipelinesGroup `json:"pipelines"`

	// ComputedAt is the timestamp of the last successful recompute.
	ComputedAt time.Time `json:"computed_at"`
	// Stale is true when the snapshot is being returned from cache because
	// the latest recompute failed. Frontend shows a non-blocking banner.
	Stale bool `json:"stale"`
	// StaleAttemptAt is set only when Stale is true — it is the timestamp
	// of the failed retry so the frontend can show a dual-label "as of X
	// (retry failed Y)" marker without losing the freshness signal.
	StaleAttemptAt time.Time `json:"stale_attempt_at,omitempty"`
	// StaleReason is a short human-readable error summary populated when
	// Stale is true. Never includes internal paths or sensitive data.
	StaleReason string `json:"stale_reason,omitempty"`
	// AllEmpty is true iff every widget reports zero data — the frontend
	// uses this to switch /dashboard to the onboarding surface instead of
	// rendering four empty widgets.
	AllEmpty bool `json:"all_empty"`
	// Warnings collects non-fatal degradations hit during Compute — e.g.
	// prototypes.List failed so the pipeline widget is approximate, but
	// the other widgets are still correct. Each entry is a short
	// human-readable string already stripped of internal paths. Populated
	// on the happy path when individual stores fail but a partial
	// snapshot is still worth returning; propagate hard via the cache's
	// error channel only when the snapshot would be misleading.
	Warnings []string `json:"warnings,omitempty"`
}

// IngestWidget holds the 30-day ingest activity chart.
type IngestWidget struct {
	// Days is exactly 30 entries in ascending chronological order. Empty
	// days are server-side padded with zero-value buckets so a broken
	// source renders as a visible gap, not a missing day.
	Days []IngestDay `json:"days"`
	// Types is the set of ingest types that appeared across the 30 days,
	// in stable alphabetical order. Drives the legend.
	Types []string `json:"types"`
	// Total is the sum of all bucket counts across the 30 days.
	Total int `json:"total"`
	// Error is a short human-readable reason this widget could not be
	// populated (e.g. vault walk failed). Empty on the happy path. The
	// frontend renders a banner over the widget when non-empty so the
	// user can distinguish "no ingest activity" from "ingest pipeline
	// broken" — silently returning zero days was the bug we're fixing.
	Error string `json:"error,omitempty"`
}

// IngestDay is one day of the 30-day chart.
type IngestDay struct {
	// Date is the local YYYY-MM-DD string; aligned with the server's
	// time zone (the dashboard is single-user local-first).
	Date string `json:"date"`
	// Buckets groups that day's ingested notes by type (articles, notes,
	// documents, daily, transcripts, etc.). Zero-count buckets are
	// omitted per day — the total is derivable.
	Buckets []IngestBucket `json:"buckets"`
	// Count is the sum across all buckets for this day.
	Count int `json:"count"`
}

// IngestBucket is one (type, count) pair within a single day.
type IngestBucket struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// ActionsWidget holds the action-by-status counters.
type ActionsWidget struct {
	// Open is actions in inbox or next — triaged or pending triage.
	Open int `json:"open"`
	// InProgress is actions in doing — WIP.
	InProgress int `json:"in_progress"`
	// Blocked is actions in waiting — blocked on external input.
	Blocked int `json:"blocked"`
	// Done is actions in done — completed (all time).
	Done int `json:"done"`
	// Stalled is the count of actions in doing OR waiting whose Updated
	// is older than StalledThreshold (7 days). This is NOT a subset of
	// any one status — it spans two statuses, which is why the badge
	// click destination is /actions?stalled=true, not /actions?status=X.
	Stalled int `json:"stalled"`
	// Total is the sum of open+in_progress+blocked+done (excludes
	// someday/deferred/cancelled — parked states that aren't "stuck").
	Total int `json:"total"`
}

// ProjectsWidget holds the Active Projects widget data.
type ProjectsWidget struct {
	// Active is the count of distinct projects touched in the last 7 days.
	Active int `json:"active"`
	// Top is the top-5 most recently touched active projects (or fewer
	// if there are not that many). "Recently touched" is defined as any
	// action update, prototype write, or ingest note referencing the
	// project within the 7-day window.
	Top []ProjectTouch `json:"top"`
}

// ProjectTouch is one entry in the Top list. ID carries the project's
// canonical UUID; Slug is the renameable filesystem alias (the FE uses
// either for routing, and renders Name).
type ProjectTouch struct {
	ID          string    `json:"id"` // canonical UUID
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	LastTouched time.Time `json:"last_touched"`
}

// PipelineWidget holds the Product Pipeline (shapeup) widget data.
type PipelineWidget struct {
	// Stages is the 5 shapeup stages (raw, frame, shape, breadboard,
	// pitch) in canonical order, with per-stage thread counts + stalled
	// counts. Stages with zero threads are still returned with Count=0
	// so the widget renders all 5 rows consistently.
	Stages []PipelineStage `json:"stages"`
	// Total is the count of threads across all stages.
	Total int `json:"total"`
	// StalledTotal is the sum of stalled threads across all stages.
	StalledTotal int `json:"stalled_total"`
}

// PipelineStage is one row of the Product Pipeline widget.
type PipelineStage struct {
	Stage string `json:"stage"`
	Count int    `json:"count"`
	// Stalled is the count of threads in this stage whose latest artifact
	// is older than StalledThreshold (7 days) — meaning the thread has
	// sat in this stage without advancing.
	Stalled int `json:"stalled"`
}

// PipelinesGroup is the row-2 cards data: one PipelineWidget per
// dashboard entity type. Powers the four cards (Product / Meetings /
// Documents / Prototypes) that drill into the per-card detail page.
//
// Each PipelineWidget uses the existing Stage/Count/Stalled shape but
// the stage names differ per type (see internal/stage for the
// canonical lists). The frontend reads `Pipelines.Product.Stages` and
// renders the stage cells in the order returned, so the aggregator is
// responsible for the ordering.
type PipelinesGroup struct {
	Product    PipelineWidget `json:"product"`
	Meetings   PipelineWidget `json:"meetings"`
	Documents  PipelineWidget `json:"documents"`
	Prototypes PipelineWidget `json:"prototypes"`
}
