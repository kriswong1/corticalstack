package dashboard

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/prototypes"
	"github.com/kriswong/corticalstack/internal/shapeup"
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

// Aggregator computes dashboard snapshots in a single pass over each
// backing store per refresh. It holds store references only; it owns no
// mutable state of its own (the cache wraps this).
type Aggregator struct {
	vault      *vault.Vault
	actions    *actions.Store
	projects   *projects.Store
	prototypes *prototypes.Store
	shapeup    *shapeup.Store

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
	if len(warnings) > 0 {
		snap.Warnings = warnings
	}

	snap.AllEmpty = snap.IngestActivity.Total == 0 &&
		snap.Actions.Total == 0 &&
		snap.ActiveProjects.Active == 0 &&
		snap.ProductPipeline.Total == 0

	return snap, nil
}

// ingestNote is one content-folder markdown note captured during the
// vault walk. Holds only the fields each widget cares about so the
// aggregator is not quadratic in vault size.
type ingestNote struct {
	folder   string
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
			created:  ts,
			projects: parseFrontmatterStrings(note.Frontmatter, "projects"),
		})
	}); err != nil {
		return nil, fmt.Errorf("vault walk: %w", err)
	}
	return out, nil
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

	typeSet := make(map[string]bool)
	total := 0
	for _, n := range notes {
		if n.created.Before(startDay) || !n.created.Before(endDay) {
			continue
		}
		key := n.created.Format("2006-01-02")
		day, ok := daysByKey[key]
		if !ok {
			continue
		}
		day.Count++
		total++
		typeSet[n.folder] = true
		// Find-or-append the bucket. Per-day bucket lists stay small (≤ number of folders) so linear scan is fine.
		found := false
		for i := range day.Buckets {
			if day.Buckets[i].Type == n.folder {
				day.Buckets[i].Count++
				found = true
				break
			}
		}
		if !found {
			day.Buckets = append(day.Buckets, IngestBucket{Type: n.folder, Count: 1})
		}
	}

	// Sort the buckets within each day for stable rendering. Always
	// return non-nil Types/Days slices — see buckets note above.
	out := IngestWidget{
		Days:  make([]IngestDay, 0, 30),
		Types: []string{},
		Total: total,
	}
	for _, key := range dayKeys {
		d := daysByKey[key]
		sort.Slice(d.Buckets, func(i, j int) bool {
			return d.Buckets[i].Type < d.Buckets[j].Type
		})
		out.Days = append(out.Days, *d)
	}

	// Stable legend order.
	for t := range typeSet {
		out.Types = append(out.Types, t)
	}
	sort.Strings(out.Types)

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

	// Build a case-insensitive id+name lookup against the canonical
	// project store. Note references that match (ignoring case) collapse
	// onto the canonical ID — so "Surveil" and "surveil" both resolve to
	// the one registered "surveil" project.
	canonicalID := make(map[string]string) // lowercased-key → canonical id
	nameByID := make(map[string]string)    // canonical id → display name
	for _, p := range a.projects.List() {
		canonicalID[strings.ToLower(p.ID)] = p.ID
		canonicalID[strings.ToLower(p.Name)] = p.ID
		nameByID[p.ID] = p.Name
	}

	normalize := func(raw string) string {
		if canonical, ok := canonicalID[strings.ToLower(raw)]; ok {
			return canonical
		}
		// Unknown ID — use lowercase so different casings of the same
		// unknown project still collapse onto one row in the widget.
		return strings.ToLower(raw)
	}

	// Map of canonical project id → most recent touch time within the window.
	lastTouchByID := make(map[string]time.Time)
	touch := func(id string, when time.Time) {
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
	for id, when := range lastTouchByID {
		name, ok := nameByID[id]
		if !ok {
			name = id
		}
		w.Top = append(w.Top, ProjectTouch{ID: id, Name: name, LastTouched: when})
	}
	sort.Slice(w.Top, func(i, j int) bool {
		return w.Top[i].LastTouched.After(w.Top[j].LastTouched)
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

// --- frontmatter parsing helpers ---

// parseFrontmatterTime accepts both RFC3339 and YYYY-MM-DD strings. Returns
// zero time if the field is missing or unparseable.
func parseFrontmatterTime(fm map[string]interface{}, key string) time.Time {
	raw, ok := fm[key]
	if !ok {
		return time.Time{}
	}
	s, ok := raw.(string)
	if !ok {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
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

