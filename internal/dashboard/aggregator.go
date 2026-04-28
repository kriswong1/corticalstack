package dashboard

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/documents"
	"github.com/kriswong/corticalstack/internal/meetings"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/prototypes"
	"github.com/kriswong/corticalstack/internal/shapeup"
	"github.com/kriswong/corticalstack/internal/stage"
	"github.com/kriswong/corticalstack/internal/vault"
)

// topProjectsN is how many recently-touched projects show up in the
// Active Projects widget top list. 5 is the standard card-sized count.
const topProjectsN = 5

// ingestContentFolders is the set of vault top-level folders whose
// markdown files count as "ingested content" in the activity chart.
// Anything outside this set — product/, prds/, prototypes/, usecases/,
// projects/, root-level persona/tracker files — is intentionally excluded.
// Listed explicitly rather than excluding-by-type so new output folders
// never silently pollute the chart.
var ingestContentFolders = map[string]bool{
	"articles":    true,
	"documents":   true,
	"notes":       true,
	"daily":       true,
	"transcripts": true,
	"webpages":    true,
	"audio":       true,
}

// folderToBucket maps a vault top-level folder name to the unified-
// dashboard bucket it should chart into. Five user-facing buckets
// plus an Other catch-all so daily/audio/webpages stay visible.
//
// `transcripts` and `webpages` may belong to either the YouTube or
// the Transcripts/Other bucket depending on the note's source URL,
// so they get an explicit `youtubeAware: true` flag — the bucketing
// happens at the per-note level in classifyIngestNote, not here.
var folderToBucket = map[string]string{
	"articles":  BucketArticles,
	"documents": BucketDocuments,
	"notes":     BucketNotes,
	// transcripts/webpages are URL-aware — classifyIngestNote checks
	// the source URL and overrides the static mapping below when the
	// URL points at YouTube.
	"transcripts": BucketTranscripts,
	"webpages":    BucketOther,
	"daily":       BucketOther,
	"audio":       BucketOther,
}

// isYouTubeURL returns true when raw looks like a YouTube source. A
// substring check is fine here — we don't need to be a URL parser, we
// need to catch the obvious cases (youtube.com/watch?v=..., youtu.be
// /..., m.youtube.com/...). False negatives are acceptable: a misclassified
// YouTube transcript still appears under Transcripts, which is the
// reasonable fallback.
func isYouTubeURL(raw string) bool {
	r := strings.ToLower(raw)
	return strings.Contains(r, "youtube.com") || strings.Contains(r, "youtu.be")
}

// Aggregator computes dashboard snapshots in a single pass over each
// backing store per refresh. It holds store references only; it owns no
// mutable state of its own (the cache wraps this).
type Aggregator struct {
	vault      *vault.Vault
	actions    *actions.Store
	projects   *projects.Store
	prototypes *prototypes.Store
	shapeup    *shapeup.Store
	meetings   *meetings.Store
	documents  *documents.Store

	// collectIngestNotesFn is an optional test-only override for the
	// vault-walk step. When non-nil, Compute calls it instead of the
	// real collectIngestNotes method. Production code never sets this
	// field; tests use it to inject deterministic error paths that the
	// real vault.Walk wrapper silently absorbs (it returns nil even on
	// root-level stat errors).
	collectIngestNotesFn func() ([]ingestNote, error)
}

// NewAggregator wires an aggregator with every store it needs. Any of
// the store pointers may be nil at startup (partial wiring during tests);
// the aggregator treats a nil store as an empty store rather than
// panicking, so the snapshot still renders.
func NewAggregator(
	v *vault.Vault,
	a *actions.Store,
	p *projects.Store,
	pr *prototypes.Store,
	su *shapeup.Store,
) *Aggregator {
	return &Aggregator{
		vault:      v,
		actions:    a,
		projects:   p,
		prototypes: pr,
		shapeup:    su,
	}
}

// WithMeetings attaches a meetings store to an aggregator and returns
// it for chaining. Optional dependency — a nil meetings store renders
// the Meetings pipeline widget as empty.
func (a *Aggregator) WithMeetings(m *meetings.Store) *Aggregator {
	a.meetings = m
	return a
}

// WithDocuments attaches a documents store to an aggregator and
// returns it for chaining. Optional dependency — a nil documents
// store renders the Documents pipeline widget as empty.
func (a *Aggregator) WithDocuments(d *documents.Store) *Aggregator {
	a.documents = d
	return a
}

// Compute produces a fresh snapshot. `now` is passed explicitly so tests
// can pin time and so the whole snapshot shares one consistent "now" —
// avoiding the subtle bug where a computation spanning midnight disagrees
// on which day a note belongs to.
//
// Error-propagation policy (HI-05 / HI-06):
//
// Compute distinguishes FATAL errors from DEGRADED errors.
//
//   - FATAL = the resulting snapshot would be actively misleading. The
//     only current fatal case is a panic/internal bug; every store
//     failure is treated as degraded. Fatal errors return (nil, err) so
//     the Cache's stale-fallback branch can serve the previous good
//     snapshot.
//
//   - DEGRADED = one widget's data is missing or partial but the other
//     widgets are still correct. Compute logs the error at Warn/Error
//     level with structured fields, stamps the affected widget with an
//     Error/Warning string so the frontend can render a banner, and
//     returns a partial snapshot with nil error. This keeps the
//     dashboard useful even when one underlying store is sick.
//
// Specifically:
//   - vault.Walk failure → IngestActivity widget marked with Error
//     (user sees "ingest pipeline unavailable" banner over the chart).
//     Pipeline- and project-touched lookups lose their ingest input but
//     the other sources (actions, prototypes, shapeup) still feed them.
//   - prototypes.List failure → project-touched lookup loses prototype
//     signal. Warning attached at snapshot level. Widget otherwise
//     continues.
//   - shapeup.ListThreads failure → Product Pipeline widget renders
//     stage rows with zero counts and a snapshot-level warning. Other
//     widgets unaffected.
//   - actions.Store / projects.Store expose only non-erroring List
//     methods so there is nothing to propagate there.
func (a *Aggregator) Compute(now time.Time) (*Snapshot, error) {
	snap := &Snapshot{
		ComputedAt: now,
	}

	// Single vault walk drives both the Ingest Activity widget and the
	// project-touched lookup for the Active Projects widget. We collect
	// every content-folder note's (folder, created, projects) tuple
	// once, then fan out into both widgets. Non-content folders short-
	// circuit during Walk so we don't parse every PRD/pitch on disk.
	collect := a.collectIngestNotes
	if a.collectIngestNotesFn != nil {
		collect = a.collectIngestNotesFn
	}
	ingestNotes, ingestErr := collect()
	if ingestErr != nil {
		// Degraded: surface the error on the widget and in logs so the
		// user has a visible signal, but keep computing the other three
		// widgets so the dashboard remains useful.
		slog.Error("dashboard: vault walk failed",
			"error", ingestErr,
			"widget", "ingest_activity")
		ingestNotes = nil
	}

	snap.IngestActivity = buildIngestWidget(ingestNotes, now)
	if ingestErr != nil {
		snap.IngestActivity.Error = "ingest activity unavailable: vault walk failed"
	}

	snap.Actions = a.buildActionsWidget(now)

	// buildProjectsWidget and buildPipelineWidget accept a warning sink
	// so they can report degraded store fetches without panicking the
	// whole Compute. Each appended warning is already sanitized — no
	// internal paths or PII.
	var warnings []string
	snap.ActiveProjects = a.buildProjectsWidget(ingestNotes, now, &warnings)
	snap.ProductPipeline = a.buildPipelineWidget(now, &warnings)

	// Row-2 cards data: one PipelineWidget per dashboard entity type.
	// Product duplicates ProductPipeline above so the existing /api
	// /dashboard schema keeps working during the front-end rewrite.
	snap.Pipelines = PipelinesGroup{
		Product:    snap.ProductPipeline,
		Meetings:   a.buildMeetingsPipelineWidget(now, &warnings),
		Documents:  a.buildDocumentsPipelineWidget(now, &warnings),
		Prototypes: a.buildPrototypesPipelineWidget(now, &warnings),
	}

	if len(warnings) > 0 {
		snap.Warnings = warnings
	}

	snap.AllEmpty = snap.IngestActivity.Total == 0 &&
		snap.Actions.Total == 0 &&
		snap.ActiveProjects.Active == 0 &&
		snap.ProductPipeline.Total == 0 &&
		snap.Pipelines.Meetings.Total == 0 &&
		snap.Pipelines.Documents.Total == 0 &&
		snap.Pipelines.Prototypes.Total == 0

	return snap, nil
}

// ingestNote is one content-folder markdown note captured during the
// vault walk. Holds only the fields each widget cares about so the
// aggregator is not quadratic in vault size.
//
// `folder` is the raw vault top-level folder (e.g. "transcripts").
// `bucket` is the dashboard-facing bucket the note charts into
// (e.g. "youtube" if the source URL points at YouTube). The two are
// kept separate so future code can still ask "what folder did this
// come from" without re-running the URL classifier.
type ingestNote struct {
	folder   string
	bucket   string
	created  time.Time
	projects []string
}

// collectIngestNotes walks the vault once and returns every .md file
// under a recognized ingest content folder, with its timestamp and
// projects list extracted. The ingest pipeline writes `ingested` as an
// RFC3339 timestamp and `date` as a YYYY-MM-DD string; we try both plus
// `created` (used by some internal stores) to survive a mix of sources
// in the same vault.
//
// Returns the error from vault.Walk verbatim (wrapped with context).
// vault.Walk itself swallows per-file read errors — only a top-level
// filesystem failure propagates. That's the exact signal the caller
// should surface on the widget: "the vault root is unreadable" is real,
// "one note has a bad date" is not worth a banner.
func (a *Aggregator) collectIngestNotes() ([]ingestNote, error) {
	if a.vault == nil {
		return nil, nil
	}
	var out []ingestNote
	if err := a.vault.Walk(func(relPath string, note *vault.Note) {
		folder := topFolder(relPath)
		if !ingestContentFolders[folder] {
			return
		}
		ts := firstNonZeroTime(note.Frontmatter, "ingested", "created", "date")
		if ts.IsZero() {
			return
		}
		out = append(out, ingestNote{
			folder:   folder,
			bucket:   classifyBucket(folder, note.Frontmatter),
			created:  ts,
			projects: parseFrontmatterStrings(note.Frontmatter, "projects"),
		})
	}); err != nil {
		return nil, fmt.Errorf("vault walk: %w", err)
	}
	return out, nil
}

// classifyBucket maps a (folder, frontmatter) pair to a dashboard
// bucket. Most folders have a fixed mapping (folderToBucket); the two
// URL-aware folders (transcripts and webpages) check the source URL
// for a YouTube domain and override accordingly.
//
// Frontmatter keys checked for the source URL, in priority order:
// `source_url`, `source`, `url`. The first non-empty string wins.
func classifyBucket(folder string, fm map[string]interface{}) string {
	bucket, ok := folderToBucket[folder]
	if !ok {
		return BucketOther
	}
	if folder != "transcripts" && folder != "webpages" {
		return bucket
	}
	for _, key := range []string{"source_url", "source", "url"} {
		if raw, ok := fm[key].(string); ok && raw != "" {
			if isYouTubeURL(raw) {
				return BucketYouTube
			}
			break
		}
	}
	return bucket
}

// firstNonZeroTime returns the first non-zero parsed time across a
// priority-ordered list of frontmatter keys. Returns zero time when none
// of the keys produce a parseable value.
func firstNonZeroTime(fm map[string]interface{}, keys ...string) time.Time {
	for _, k := range keys {
		if t := parseFrontmatterTime(fm, k); !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

// buildIngestWidget aggregates notes into the 30-day chart with
// server-side zero padding so missing days are explicit gaps.
//
// Bucketing: each ingestNote carries a `bucket` field set by
// classifyBucket (Articles / YouTube / Transcripts / Documents /
// Notes / Other). The widget's Types slice is always the canonical
// IngestBucketOrder so the frontend renders the same legend slots
// every refresh — even days that contain only one bucket still align
// with the others on the chart.
//
// For backward-compat with the old test that fed `folder` directly,
// notes with an empty `bucket` are bucketed via folderToBucket as a
// fallback. Production callers always come through collectIngestNotes
// so this fallback is just defensive.
func buildIngestWidget(notes []ingestNote, now time.Time) IngestWidget {
	// Window starts at 29 days ago so that inclusive 30 days end at today.
	startDay := startOfDay(now).Add(-29 * 24 * time.Hour)
	endDay := startOfDay(now).Add(24 * time.Hour)

	// Pre-seed exactly 30 day slots so empty days are visible.
	daysByKey := make(map[string]*IngestDay, 30)
	dayKeys := make([]string, 0, 30)
	for d := startDay; d.Before(endDay); d = d.Add(24 * time.Hour) {
		key := d.Format("2006-01-02")
		dayKeys = append(dayKeys, key)
		// Initialize Buckets as an empty slice (not nil) so the JSON
		// contract is always `"buckets": []` — the frontend maps over
		// this unconditionally and would blow up on null.
		daysByKey[key] = &IngestDay{Date: key, Buckets: []IngestBucket{}}
	}

	total := 0
	for _, n := range notes {
		// Window bounds are strict calendar-day: reject anything older
		// than 30 days and anything dated on or after tomorrow 00:00.
		// See the long comment on this branch in the previous version
		// of the file (preserved in git history) for the asymmetry
		// rationale vs buildProjectsWidget.
		if n.created.Before(startDay) || !n.created.Before(endDay) {
			continue
		}
		key := n.created.Format("2006-01-02")
		day, ok := daysByKey[key]
		if !ok {
			continue
		}

		bucket := n.bucket
		if bucket == "" {
			// Defensive fallback for callers that built an ingestNote
			// without going through collectIngestNotes (the old test
			// helpers do this).
			if mapped, ok := folderToBucket[n.folder]; ok {
				bucket = mapped
			} else {
				bucket = BucketOther
			}
		}

		day.Count++
		total++
		// Find-or-append the bucket. Per-day bucket lists stay small
		// (≤ 6) so linear scan is fine.
		found := false
		for i := range day.Buckets {
			if day.Buckets[i].Type == bucket {
				day.Buckets[i].Count++
				found = true
				break
			}
		}
		if !found {
			day.Buckets = append(day.Buckets, IngestBucket{Type: bucket, Count: 1})
		}
	}

	// Stable per-day bucket order: render in the canonical legend
	// order so the visual stack is consistent across days.
	bucketRank := make(map[string]int, len(IngestBucketOrder))
	for i, b := range IngestBucketOrder {
		bucketRank[b] = i
	}

	out := IngestWidget{
		Days:  make([]IngestDay, 0, 30),
		Types: append([]string(nil), IngestBucketOrder...),
		Total: total,
	}
	for _, key := range dayKeys {
		d := daysByKey[key]
		sort.Slice(d.Buckets, func(i, j int) bool {
			return bucketRank[d.Buckets[i].Type] < bucketRank[d.Buckets[j].Type]
		})
		out.Days = append(out.Days, *d)
	}

	return out
}

// buildActionsWidget walks the action store exactly once, counting by
// mapped bucket (open/in-progress/blocked/done) and computing a single
// cross-bucket stalled count.
func (a *Aggregator) buildActionsWidget(now time.Time) ActionsWidget {
	if a.actions == nil {
		return ActionsWidget{}
	}
	w := ActionsWidget{}
	for _, act := range a.actions.List() {
		status := actions.MigrateStatus(act.Status)
		switch status {
		case actions.StatusInbox, actions.StatusNext:
			w.Open++
			w.Total++
		case actions.StatusDoing:
			w.InProgress++
			w.Total++
			if isStalled(act.Updated, now) {
				w.Stalled++
			}
		case actions.StatusWaiting:
			w.Blocked++
			w.Total++
			if isStalled(act.Updated, now) {
				w.Stalled++
			}
		case actions.StatusDone:
			w.Done++
			w.Total++
			// Done is never stalled — it's complete.
		default:
			// someday / deferred / cancelled are parked states, not "stuck"
			// — excluded from every counter so they don't inflate totals.
		}
	}
	return w
}

// buildProjectsWidget finds projects touched in the last 7 days by
// scanning actions (Updated), prototypes (Created), and ingest notes
// (frontmatter created) for project references.
//
// Project IDs in note frontmatter are inconsistent in practice — the same
// project is referenced as both "Surveil" and "surveil" across notes.
// We normalize every incoming ID through canonicalProjectID so a single
// real project surfaces as one row, not N.
//
// Appends to `warnings` (never nil — caller owns the slice) when a store
// fetch degrades. A degraded prototype fetch means the widget undercounts
// touches from prototypes, but actions + ingest notes still feed it.
func (a *Aggregator) buildProjectsWidget(ingestNotes []ingestNote, now time.Time, warnings *[]string) ProjectsWidget {
	if a.projects == nil {
		return ProjectsWidget{}
	}
	cutoff := now.Add(-StalledThreshold)

	// Build a UUID-canonical lookup against the project store. Note
	// references after Phase 1 migration are UUIDs; pre-migration values
	// or hand-edited slugs resolve through the slug index. Case-insensitive
	// fallback on slugs preserves the legacy "Surveil" → "surveil" collapse.
	canonicalID := make(map[string]string) // lowercased-key → canonical UUID
	nameByID := make(map[string]string)    // canonical UUID → display name
	for _, p := range a.projects.List() {
		nameByID[p.UUID] = p.Name
		canonicalID[strings.ToLower(p.UUID)] = p.UUID
		canonicalID[strings.ToLower(p.Slug)] = p.UUID
		// Name fallback for hand-edited frontmatter that uses display name.
		if key := strings.ToLower(p.Name); key != "" {
			if _, taken := canonicalID[key]; !taken {
				canonicalID[key] = p.UUID
			}
		}
	}

	normalize := func(raw string) string {
		if canonical, ok := canonicalID[strings.ToLower(raw)]; ok {
			return canonical
		}
		// Unknown reference — use lowercase so different casings collapse
		// onto one row in the widget.
		return strings.ToLower(raw)
	}

	// Map of canonical project id → most recent touch time within the window.
	lastTouchByID := make(map[string]time.Time)
	touch := func(id string, when time.Time) {
		// Rolling 7-day window with 1-minute clock-skew tolerance on the
		// upper bound. The tolerance is intentional: this is a "did you
		// touch this project recently" freshness check, and a laptop
		// whose clock is a few seconds ahead of the server should still
		// see its own activity reflected. Intentionally asymmetric vs
		// buildIngestWidget's strict calendar-day upper bound — see
		// that site's comment and docs/code-review-go.md LO-05.
		if id == "" || when.Before(cutoff) || when.After(now.Add(time.Minute)) {
			return
		}
		canon := normalize(id)
		if existing, ok := lastTouchByID[canon]; !ok || when.After(existing) {
			lastTouchByID[canon] = when
		}
	}

	if a.actions != nil {
		for _, act := range a.actions.List() {
			for _, pid := range act.ProjectIDs {
				touch(pid, act.Updated)
			}
		}
	}

	if a.prototypes != nil {
		list, err := a.prototypes.List()
		if err != nil {
			// Degraded: log + surface a warning so the operator knows
			// prototype signal is missing from the widget this cycle.
			// Do NOT fail the whole widget — actions and ingest notes
			// still populate lastTouchByID correctly.
			slog.Warn("dashboard: prototypes.List failed",
				"error", err,
				"widget", "active_projects")
			if warnings != nil {
				*warnings = append(*warnings, "active projects: prototype signal unavailable")
			}
		} else {
			for _, p := range list {
				for _, pid := range p.Projects {
					touch(pid, p.Created)
				}
			}
		}
	}

	for _, n := range ingestNotes {
		for _, pid := range n.projects {
			touch(pid, n.created)
		}
	}

	w := ProjectsWidget{Active: len(lastTouchByID), Top: []ProjectTouch{}}
	// Slug+Name lookup off the same store pass so we don't hit the lock
	// per-entry below.
	slugByUUID := make(map[string]string, len(canonicalID))
	for _, p := range a.projects.List() {
		slugByUUID[p.UUID] = p.Slug
	}
	for id, when := range lastTouchByID {
		name, ok := nameByID[id]
		if !ok {
			name = id
		}
		w.Top = append(w.Top, ProjectTouch{
			ID:          id,
			Slug:        slugByUUID[id], // empty if id is an unknown UUID/slug — that's fine
			Name:        name,
			LastTouched: when,
		})
	}
	// NT-04: secondary sort by Name (then UUID) so two projects with
	// identical LastTouched get a deterministic, user-meaningful order
	// across dashboard refreshes. Sorting by UUID alone is deterministic
	// but visually random; Name first keeps the FE list predictable.
	sort.Slice(w.Top, func(i, j int) bool {
		if !w.Top[i].LastTouched.Equal(w.Top[j].LastTouched) {
			return w.Top[i].LastTouched.After(w.Top[j].LastTouched)
		}
		if !strings.EqualFold(w.Top[i].Name, w.Top[j].Name) {
			return strings.ToLower(w.Top[i].Name) < strings.ToLower(w.Top[j].Name)
		}
		return w.Top[i].ID < w.Top[j].ID
	})
	if len(w.Top) > topProjectsN {
		w.Top = w.Top[:topProjectsN]
	}
	return w
}

// buildPipelineWidget groups shapeup threads by current_stage and counts
// stalled threads — threads whose latest artifact is older than the
// 7-day threshold.
//
// Appends to `warnings` (caller owns the slice) when ListThreads fails.
// We still render the full set of stage rows with zero counts rather
// than returning an empty widget: the UI expects a stable 5-row shape.
func (a *Aggregator) buildPipelineWidget(now time.Time, warnings *[]string) PipelineWidget {
	stages := shapeup.AllStages()
	w := PipelineWidget{
		Stages: make([]PipelineStage, 0, len(stages)),
	}
	countByStage := make(map[shapeup.Stage]int)
	stalledByStage := make(map[shapeup.Stage]int)

	if a.shapeup != nil {
		threads, err := a.shapeup.ListThreads()
		if err != nil {
			// Degraded: log + warn; leave count/stalled maps empty so
			// the widget still emits all stage rows with zero counts.
			slog.Warn("dashboard: shapeup.ListThreads failed",
				"error", err,
				"widget", "product_pipeline")
			if warnings != nil {
				*warnings = append(*warnings, "product pipeline: shapeup store unavailable")
			}
		} else {
			for _, t := range threads {
				countByStage[t.CurrentStage]++
				// Find the artifact at the current stage — that's the one we
				// check for staleness. Artifacts are immutable per stage, so
				// Created is effectively the advance-into-stage timestamp.
				for _, art := range t.Artifacts {
					if art.Stage == t.CurrentStage {
						if isStalled(art.Created, now) {
							stalledByStage[t.CurrentStage]++
						}
						break
					}
				}
			}
		}
	}

	for _, stage := range stages {
		ps := PipelineStage{
			Stage:   string(stage),
			Count:   countByStage[stage],
			Stalled: stalledByStage[stage],
		}
		w.Stages = append(w.Stages, ps)
		w.Total += ps.Count
		w.StalledTotal += ps.Stalled
	}
	return w
}

// buildMeetingsPipelineWidget groups meeting notes by stage. Stages
// are the meeting entity's canonical list (transcript / audio /
// note). Empty stages still emit a row with Count=0 so the frontend
// renders a stable shape.
//
// Stalled detection: a meeting is stalled if its Updated timestamp
// (or Created if Updated is zero) is older than the 7-day threshold.
// Mirrors the buildPipelineWidget convention.
func (a *Aggregator) buildMeetingsPipelineWidget(now time.Time, warnings *[]string) PipelineWidget {
	stages := stage.AllStages(stage.EntityMeeting)
	w := PipelineWidget{Stages: make([]PipelineStage, 0, len(stages))}
	countByStage := make(map[stage.Stage]int)
	stalledByStage := make(map[stage.Stage]int)

	if a.meetings != nil {
		list, err := a.meetings.List()
		if err != nil {
			slog.Warn("dashboard: meetings.List failed",
				"error", err, "widget", "meetings_pipeline")
			if warnings != nil {
				*warnings = append(*warnings, "meetings pipeline: store unavailable")
			}
		} else {
			for _, m := range list {
				st := stage.Normalize(stage.EntityMeeting, string(m.Stage))
				countByStage[st]++
				touchedAt := m.Updated
				if touchedAt.IsZero() {
					touchedAt = m.Created
				}
				if isStalled(touchedAt, now) {
					stalledByStage[st]++
				}
			}
		}
	}

	for _, st := range stages {
		ps := PipelineStage{
			Stage:   string(st),
			Count:   countByStage[st],
			Stalled: stalledByStage[st],
		}
		w.Stages = append(w.Stages, ps)
		w.Total += ps.Count
		w.StalledTotal += ps.Stalled
	}
	return w
}

// buildDocumentsPipelineWidget groups documents by stage (need /
// in_progress / final). Same shape rules as the meetings widget.
func (a *Aggregator) buildDocumentsPipelineWidget(now time.Time, warnings *[]string) PipelineWidget {
	stages := stage.AllStages(stage.EntityDocument)
	w := PipelineWidget{Stages: make([]PipelineStage, 0, len(stages))}
	countByStage := make(map[stage.Stage]int)
	stalledByStage := make(map[stage.Stage]int)

	if a.documents != nil {
		list, err := a.documents.List()
		if err != nil {
			slog.Warn("dashboard: documents.List failed",
				"error", err, "widget", "documents_pipeline")
			if warnings != nil {
				*warnings = append(*warnings, "documents pipeline: store unavailable")
			}
		} else {
			for _, d := range list {
				st := stage.Normalize(stage.EntityDocument, string(d.Stage))
				countByStage[st]++
				touchedAt := d.Updated
				if touchedAt.IsZero() {
					touchedAt = d.Created
				}
				if isStalled(touchedAt, now) {
					stalledByStage[st]++
				}
			}
		}
	}

	for _, st := range stages {
		ps := PipelineStage{
			Stage:   string(st),
			Count:   countByStage[st],
			Stalled: stalledByStage[st],
		}
		w.Stages = append(w.Stages, ps)
		w.Total += ps.Count
		w.StalledTotal += ps.Stalled
	}
	return w
}

// buildPrototypesPipelineWidget groups prototypes by stage. Reuses
// the existing prototypes.Store.List and falls back through stage
// .Normalize so legacy `status: draft` notes still classify as
// in_progress.
func (a *Aggregator) buildPrototypesPipelineWidget(now time.Time, warnings *[]string) PipelineWidget {
	stages := stage.AllStages(stage.EntityPrototype)
	w := PipelineWidget{Stages: make([]PipelineStage, 0, len(stages))}
	countByStage := make(map[stage.Stage]int)
	stalledByStage := make(map[stage.Stage]int)

	if a.prototypes != nil {
		list, err := a.prototypes.List()
		if err != nil {
			slog.Warn("dashboard: prototypes.List failed",
				"error", err, "widget", "prototypes_pipeline")
			if warnings != nil {
				*warnings = append(*warnings, "prototypes pipeline: store unavailable")
			}
		} else {
			for _, p := range list {
				st := stage.Normalize(stage.EntityPrototype, string(p.Stage))
				if st == "" {
					st = stage.Normalize(stage.EntityPrototype, p.Status)
				}
				countByStage[st]++
				touchedAt := p.Updated
				if touchedAt.IsZero() {
					touchedAt = p.Created
				}
				if isStalled(touchedAt, now) {
					stalledByStage[st]++
				}
			}
		}
	}

	for _, st := range stages {
		ps := PipelineStage{
			Stage:   string(st),
			Count:   countByStage[st],
			Stalled: stalledByStage[st],
		}
		w.Stages = append(w.Stages, ps)
		w.Total += ps.Count
		w.StalledTotal += ps.Stalled
	}
	return w
}

// --- frontmatter parsing helpers ---

// parseFrontmatterTime accepts both RFC3339 and YYYY-MM-DD strings, plus
// native time.Time values that yaml.v3 may unmarshal directly when the
// scalar carries a timestamp tag. Returns zero time if the field is
// missing or unparseable.
//
// LO-03: the previous implementation only handled the string case, so a
// yaml frontmatter block with an auto-typed timestamp (`ingested:
// 2026-04-11T09:00:00Z` without explicit tagging) would silently yield
// zero time on some yaml.v3 builds.
func parseFrontmatterTime(fm map[string]interface{}, key string) time.Time {
	raw, ok := fm[key]
	if !ok {
		return time.Time{}
	}
	switch v := raw.(type) {
	case time.Time:
		return v
	case string:
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02", v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseFrontmatterStrings extracts a []string from a frontmatter field.
//
// Accepts three shapes because the value can arrive through different
// code paths:
//   - []interface{} — the shape produced by yaml.v3 unmarshal after a
//     round-trip through disk; this is what every Walk() caller sees.
//   - []string — the shape produced by in-memory writers in the ingest
//     pipeline (see route.go) when a note is read back inside the same
//     request without a disk round-trip. Dropping this shape silently
//     masked the project-touched lookup for freshly-ingested notes.
//   - string — the scalar form a user may hand-write for a single-value
//     list field (e.g. `projects: alpha`). Returned as a one-element
//     slice so the single-project happy path works without a type
//     assertion at every call site.
//
// Anything else (ints, maps, nil) yields nil.
func parseFrontmatterStrings(fm map[string]interface{}, key string) []string {
	raw, ok := fm[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		// Defensive copy + empty-string filter so callers get a clean
		// slice even if the source accidentally holds empty entries.
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}

// topFolder returns the first path component of relPath (empty string
// for root-level files). Uses forward slashes since vault.Walk emits
// relative paths with ToSlash() already.
func topFolder(relPath string) string {
	if idx := strings.Index(relPath, "/"); idx >= 0 {
		return relPath[:idx]
	}
	return ""
}

// startOfDay returns t truncated to the start of the local day. time.Truncate
// is not timezone-aware, so we zero out the hour/min/sec/nsec fields manually
// while keeping the location.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

