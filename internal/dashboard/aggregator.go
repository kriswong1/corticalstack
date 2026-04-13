package dashboard

import (
	"fmt"
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
func (a *Aggregator) Compute(now time.Time) (*Snapshot, error) {
	snap := &Snapshot{
		ComputedAt: now,
	}

	// Single vault walk drives both the Ingest Activity widget and the
	// project-touched lookup for the Active Projects widget. We collect
	// every content-folder note's (folder, created, projects) tuple
	// once, then fan out into both widgets. Non-content folders short-
	// circuit during Walk so we don't parse every PRD/pitch on disk.
	ingestNotes, err := a.collectIngestNotes()
	if err != nil {
		// Vault walk errors degrade the widget but should not fail the
		// whole snapshot — the dashboard still has value with three
		// widgets working.
		ingestNotes = nil
	}

	snap.IngestActivity = buildIngestWidget(ingestNotes, now)
	snap.Actions = a.buildActionsWidget(now)
	snap.ActiveProjects = a.buildProjectsWidget(ingestNotes, now)
	snap.ProductPipeline = a.buildPipelineWidget(now)

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
func (a *Aggregator) collectIngestNotes() ([]ingestNote, error) {
	if a.vault == nil {
		return nil, nil
	}
	var out []ingestNote
	err := a.vault.Walk(func(relPath string, note *vault.Note) {
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
	})
	return out, err
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
func (a *Aggregator) buildProjectsWidget(ingestNotes []ingestNote, now time.Time) ProjectsWidget {
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
		list, _ := a.prototypes.List()
		for _, p := range list {
			for _, pid := range p.Projects {
				touch(pid, p.Created)
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
func (a *Aggregator) buildPipelineWidget(now time.Time) PipelineWidget {
	stages := shapeup.AllStages()
	w := PipelineWidget{
		Stages: make([]PipelineStage, 0, len(stages)),
	}
	countByStage := make(map[shapeup.Stage]int)
	stalledByStage := make(map[shapeup.Stage]int)

	if a.shapeup != nil {
		threads, _ := a.shapeup.ListThreads()
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

// parseFrontmatterStrings extracts a []string from a frontmatter array
// field. Handles the yaml→interface{} round-trip where arrays come back
// as []interface{} of string values.
func parseFrontmatterStrings(fm map[string]interface{}, key string) []string {
	raw, ok := fm[key]
	if !ok {
		return nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
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

// Sentinel error for the cache error-path test. Kept as a typed value so
// the cache can return it consistently.
var errNoSnapshot = fmt.Errorf("dashboard: no snapshot available yet")
