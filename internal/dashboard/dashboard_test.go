package dashboard

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/prototypes"
	"github.com/kriswong/corticalstack/internal/shapeup"
	"github.com/kriswong/corticalstack/internal/vault"
)

// fixedTime is a deterministic "now" used throughout the tests so stalled
// boundaries and 30-day windows behave identically on every run.
var fixedTime = time.Date(2026, 4, 12, 15, 0, 0, 0, time.UTC)

// --- stalled threshold boundary tests ---

func TestIsStalled_boundary(t *testing.T) {
	tests := []struct {
		name    string
		updated time.Time
		want    bool
	}{
		{"zero time never stalled", time.Time{}, false},
		{"just updated", fixedTime.Add(-1 * time.Minute), false},
		{"6 days old not stalled", fixedTime.Add(-6 * 24 * time.Hour), false},
		{"exactly 7 days old IS stalled", fixedTime.Add(-7 * 24 * time.Hour), true},
		{"7 days minus 1 second not stalled", fixedTime.Add(-7*24*time.Hour + time.Second), false},
		{"8 days old stalled", fixedTime.Add(-8 * 24 * time.Hour), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isStalled(tc.updated, fixedTime); got != tc.want {
				t.Errorf("isStalled(%v, now) = %v, want %v", tc.updated, got, tc.want)
			}
		})
	}
}

// --- 30-day padding test ---

func TestBuildIngestWidget_30DayPaddingWithSparseData(t *testing.T) {
	// Three notes spread across the 30-day window, with a gap in the middle
	// that must render as zero-count days (not collapse).
	notes := []ingestNote{
		{folder: "articles", created: fixedTime.Add(-29 * 24 * time.Hour)},
		{folder: "notes", created: fixedTime.Add(-15 * 24 * time.Hour)},
		{folder: "articles", created: fixedTime}, // today
	}
	w := buildIngestWidget(notes, fixedTime)

	if len(w.Days) != 30 {
		t.Fatalf("expected exactly 30 day buckets, got %d", len(w.Days))
	}
	if w.Total != 3 {
		t.Errorf("expected total=3, got %d", w.Total)
	}
	// First and last days have 1 note each, middle has at most 1.
	if w.Days[0].Count != 1 {
		t.Errorf("day[0] count = %d, want 1", w.Days[0].Count)
	}
	if w.Days[29].Count != 1 {
		t.Errorf("day[29] count = %d, want 1", w.Days[29].Count)
	}
	// Day 1 (after the first note) must render as explicit zero, not be missing.
	if w.Days[1].Count != 0 {
		t.Errorf("day[1] count = %d, want 0 (gap must render)", w.Days[1].Count)
	}
	if w.Days[1].Date == "" {
		t.Errorf("empty day must still carry a date label")
	}
	// The Types slice is now the fixed bucket order — every refresh
	// returns the same 6 buckets in the same order so the frontend
	// legend has stable slots even on days where some buckets are
	// empty. The previous "alphabetical filtered to seen types"
	// behavior moved into the per-day Buckets fields.
	wantTypes := []string{"articles", "youtube", "transcripts", "documents", "notes", "other"}
	if !reflect.DeepEqual(w.Types, wantTypes) {
		t.Errorf("types = %v, want %v (fixed legend order)", w.Types, wantTypes)
	}
}

// --- bucket classification tests ---

func TestClassifyBucket_staticFolders(t *testing.T) {
	// Folders that always map to the same bucket regardless of
	// frontmatter content.
	cases := map[string]string{
		"articles":  BucketArticles,
		"documents": BucketDocuments,
		"notes":     BucketNotes,
		"daily":     BucketOther,
		"audio":     BucketOther,
	}
	for folder, want := range cases {
		got := classifyBucket(folder, nil)
		if got != want {
			t.Errorf("classifyBucket(%q) = %q, want %q", folder, got, want)
		}
	}
}

func TestClassifyBucket_youtubeFromTranscripts(t *testing.T) {
	// A transcript whose source URL points at YouTube buckets as
	// YouTube; one without buckets as Transcripts.
	yt := classifyBucket("transcripts", map[string]interface{}{
		"source_url": "https://www.youtube.com/watch?v=abc",
	})
	if yt != BucketYouTube {
		t.Errorf("youtube transcript bucketed as %q, want %q", yt, BucketYouTube)
	}

	other := classifyBucket("transcripts", map[string]interface{}{
		"source_url": "https://example.com/article",
	})
	if other != BucketTranscripts {
		t.Errorf("non-youtube transcript bucketed as %q, want %q", other, BucketTranscripts)
	}

	none := classifyBucket("transcripts", nil)
	if none != BucketTranscripts {
		t.Errorf("transcript without source_url bucketed as %q, want %q", none, BucketTranscripts)
	}
}

func TestClassifyBucket_youtubeFromWebpages(t *testing.T) {
	yt := classifyBucket("webpages", map[string]interface{}{
		"source_url": "https://youtu.be/abc",
	})
	if yt != BucketYouTube {
		t.Errorf("youtube webpage bucketed as %q, want %q", yt, BucketYouTube)
	}

	// Non-YouTube webpages stay in Other so daily/audio/webpages don't
	// disappear from the chart.
	other := classifyBucket("webpages", map[string]interface{}{
		"source_url": "https://example.com/post",
	})
	if other != BucketOther {
		t.Errorf("non-youtube webpage bucketed as %q, want %q", other, BucketOther)
	}
}

func TestClassifyBucket_unknownFolderFallsBackToOther(t *testing.T) {
	if got := classifyBucket("notarealfolder", nil); got != BucketOther {
		t.Errorf("unknown folder bucketed as %q, want %q", got, BucketOther)
	}
}

func TestBuildIngestWidget_notesOutsideWindowExcluded(t *testing.T) {
	notes := []ingestNote{
		{folder: "articles", created: fixedTime.Add(-60 * 24 * time.Hour)}, // too old
		{folder: "articles", created: fixedTime.Add(1 * 24 * time.Hour)},   // future
		{folder: "articles", created: fixedTime},                           // in window
	}
	w := buildIngestWidget(notes, fixedTime)
	if w.Total != 1 {
		t.Errorf("total = %d, want 1 (only today's note in window)", w.Total)
	}
}

func TestBuildIngestWidget_empty(t *testing.T) {
	w := buildIngestWidget(nil, fixedTime)
	if len(w.Days) != 30 {
		t.Errorf("empty input must still produce 30 day buckets, got %d", len(w.Days))
	}
	if w.Total != 0 {
		t.Errorf("total = %d, want 0", w.Total)
	}
}

// --- actions widget tests ---

func TestBuildActionsWidget_countsAndStalled(t *testing.T) {
	v := newTestVault(t)
	store := actions.New(v)

	// Seed actions across every status. Some in doing/waiting that are
	// stalled (old Updated), some fresh.
	fresh := fixedTime.Add(-1 * time.Hour)
	stale := fixedTime.Add(-10 * 24 * time.Hour)

	mustUpsert(t, store, &actions.Action{Description: "i1", Status: actions.StatusInbox, Updated: fresh})
	mustUpsert(t, store, &actions.Action{Description: "n1", Status: actions.StatusNext, Updated: fresh})
	mustUpsert(t, store, &actions.Action{Description: "d-fresh", Status: actions.StatusDoing, Updated: fresh})
	mustUpsert(t, store, &actions.Action{Description: "d-stale", Status: actions.StatusDoing, Updated: stale})
	mustUpsert(t, store, &actions.Action{Description: "w-stale", Status: actions.StatusWaiting, Updated: stale})
	mustUpsert(t, store, &actions.Action{Description: "done", Status: actions.StatusDone, Updated: fresh})
	mustUpsert(t, store, &actions.Action{Description: "someday", Status: actions.StatusSomeday, Updated: fresh})
	mustUpsert(t, store, &actions.Action{Description: "cancelled", Status: actions.StatusCancelled, Updated: fresh})

	// NOTE: actions.Store.Upsert stamps Updated=now on the insert path,
	// which clobbers the Updated field we pass in above. That's correct
	// for production (new actions track the moment they were created)
	// but wrong for tests that need to simulate aged items for stalled
	// detection. We reach through List() — which returns live pointers
	// — and rewrite Updated in place for the stalled-sensitive actions.
	//
	// This is NOT the MD-02 workaround (which was about Upsert bumping
	// Updated on idempotent re-ingest). MD-02 was fixed in Wave 2 via
	// the field-equality candidate check in store.go. This workaround
	// is orthogonal and still required because the insert path unconditionally
	// sets Updated=now (store.go:189-191), and the tests have no other
	// way to backdate a freshly-inserted action.
	for _, a := range store.List() {
		switch a.Description {
		case "d-stale":
			a.Updated = stale
		case "w-stale":
			a.Updated = stale
		case "d-fresh":
			a.Updated = fresh
		}
	}

	agg := &Aggregator{actions: store}
	w := agg.buildActionsWidget(fixedTime)

	if w.Open != 2 {
		t.Errorf("Open = %d, want 2 (inbox+next)", w.Open)
	}
	if w.InProgress != 2 {
		t.Errorf("InProgress = %d, want 2", w.InProgress)
	}
	if w.Blocked != 1 {
		t.Errorf("Blocked = %d, want 1", w.Blocked)
	}
	if w.Done != 1 {
		t.Errorf("Done = %d, want 1", w.Done)
	}
	if w.Total != 6 {
		t.Errorf("Total = %d, want 6 (someday+cancelled excluded)", w.Total)
	}
	if w.Stalled != 2 {
		t.Errorf("Stalled = %d, want 2 (d-stale + w-stale)", w.Stalled)
	}
}

func TestBuildActionsWidget_nilStore(t *testing.T) {
	agg := &Aggregator{}
	w := agg.buildActionsWidget(fixedTime)
	if w.Total != 0 {
		t.Errorf("nil store must produce zero widget")
	}
}

// --- pipeline widget test (stage enum alignment) ---

func TestBuildPipelineWidget_stageEnumAlignment(t *testing.T) {
	agg := &Aggregator{}
	var warnings []string
	w := agg.buildPipelineWidget(fixedTime, &warnings)

	// Must have exactly one row per shapeup stage, in canonical order,
	// even when no threads exist.
	if len(w.Stages) != len(shapeup.AllStages()) {
		t.Fatalf("stages = %d, want %d (one per shapeup stage)", len(w.Stages), len(shapeup.AllStages()))
	}
	for i, stage := range shapeup.AllStages() {
		if w.Stages[i].Stage != string(stage) {
			t.Errorf("stages[%d] = %q, want %q", i, w.Stages[i].Stage, stage)
		}
	}
	if len(warnings) != 0 {
		t.Errorf("expected zero warnings with nil shapeup store, got %v", warnings)
	}
}

// --- empty-all-four detection ---

func TestCompute_allEmptyFlagSetWhenEverythingIsZero(t *testing.T) {
	agg := &Aggregator{} // no stores wired
	snap, err := agg.Compute(fixedTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !snap.AllEmpty {
		t.Errorf("expected AllEmpty=true with no stores wired")
	}
}

func TestCompute_allEmptyFlagFalseWhenAnyWidgetHasData(t *testing.T) {
	v := newTestVault(t)
	store := actions.New(v)
	mustUpsert(t, store, &actions.Action{Description: "one", Status: actions.StatusNext})

	agg := &Aggregator{actions: store}
	snap, err := agg.Compute(fixedTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.AllEmpty {
		t.Errorf("expected AllEmpty=false when at least one widget has data")
	}
}

// --- cache TTL behavior ---
//
// The tests below drive the cache via a fake aggregator that can be
// flipped between success and failure modes on demand. This is the
// only way to exercise the stale-fallback path deterministically: the
// real Aggregator.Compute errors out only on a genuine vault failure,
// and making that happen in-process is both flaky and irrelevant to
// what we're testing — we want to prove the CACHE semantics, not the
// aggregator's internal I/O.

// fakeAggregator implements aggregatorIface for cache tests. Every call
// to Compute bumps ComputeCount (so tests can assert recompute vs cache
// hit) and either returns a Snapshot stamped with the given `now`, or
// returns the configured failure error.
type fakeAggregator struct {
	computeCount int
	fail         bool
	failErr      error
}

func (f *fakeAggregator) Compute(now time.Time) (*Snapshot, error) {
	f.computeCount++
	if f.fail {
		return nil, f.failErr
	}
	return &Snapshot{ComputedAt: now}, nil
}

func TestCache_freshComputeSuccess(t *testing.T) {
	// Case 1: fresh cache → successful compute → Stale=false,
	// ComputeCount advances to 1.
	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	current := baseTime
	clock := func() time.Time { return current }

	fake := &fakeAggregator{}
	cache := newCache(fake, 5*time.Minute, clock)

	snap, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Stale {
		t.Errorf("fresh snapshot should not be stale")
	}
	if !snap.ComputedAt.Equal(baseTime) {
		t.Errorf("ComputedAt = %v, want %v", snap.ComputedAt, baseTime)
	}
	if fake.computeCount != 1 {
		t.Errorf("computeCount = %d, want 1 after first call", fake.computeCount)
	}
}

func TestCache_withinTTLNoRecompute(t *testing.T) {
	// Case 2: within TTL, subsequent calls must NOT recompute, even
	// multiple times, and must return the same cached pointer.
	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	current := baseTime
	clock := func() time.Time { return current }

	fake := &fakeAggregator{}
	cache := newCache(fake, 5*time.Minute, clock)

	first, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	// Advance clock but stay within TTL (5min).
	current = baseTime.Add(1 * time.Minute)
	second, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	current = baseTime.Add(4 * time.Minute)
	third, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("third: %v", err)
	}

	if first != second || second != third {
		t.Errorf("within-TTL calls must return identical pointer, got %p %p %p", first, second, third)
	}
	if fake.computeCount != 1 {
		t.Errorf("computeCount = %d, want 1 (no recomputes within TTL)", fake.computeCount)
	}
}

func TestCache_recomputesAfterTTL(t *testing.T) {
	// Case 3: after TTL expiry, the next call must recompute and
	// return a snapshot whose ComputedAt has advanced.
	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	current := baseTime
	clock := func() time.Time { return current }

	fake := &fakeAggregator{}
	cache := newCache(fake, 5*time.Minute, clock)

	first, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	current = baseTime.Add(10 * time.Minute)
	second, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if !second.ComputedAt.After(first.ComputedAt) {
		t.Errorf("post-TTL snapshot should recompute: first=%v second=%v", first.ComputedAt, second.ComputedAt)
	}
	if fake.computeCount != 2 {
		t.Errorf("computeCount = %d, want 2", fake.computeCount)
	}
}

func TestCache_staleFallbackOnRecomputeError(t *testing.T) {
	// Case 4: TTL expired, recompute fails, prior cache exists
	// → return cached value with Stale=true, StaleReason populated,
	// StaleAttemptAt set. ComputeCount advances.
	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	current := baseTime
	clock := func() time.Time { return current }

	fake := &fakeAggregator{}
	cache := newCache(fake, 5*time.Minute, clock)

	// Seed a good snapshot.
	first, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if first.Stale {
		t.Fatalf("seed snapshot must not be stale")
	}
	seedComputedAt := first.ComputedAt

	// Flip the aggregator to failing mode and advance past TTL.
	fake.fail = true
	fake.failErr = errors.New("boom")
	current = baseTime.Add(10 * time.Minute)

	second, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("stale fallback returned unexpected error: %v", err)
	}
	if !second.Stale {
		t.Errorf("expected Stale=true on fallback path")
	}
	if !second.ComputedAt.Equal(seedComputedAt) {
		t.Errorf("stale snapshot must preserve original ComputedAt: got %v want %v", second.ComputedAt, seedComputedAt)
	}
	if !second.StaleAttemptAt.Equal(current) {
		t.Errorf("StaleAttemptAt = %v, want %v (time of failed retry)", second.StaleAttemptAt, current)
	}
	if second.StaleReason == "" {
		t.Errorf("StaleReason should include the compute error")
	}
	if fake.computeCount != 2 {
		t.Errorf("computeCount = %d, want 2 (seed + failed retry)", fake.computeCount)
	}
}

func TestCache_staleFallbackNoPriorCacheReturnsError(t *testing.T) {
	// Case 5: TTL expired, recompute fails, NO prior cache → error
	// bubbles up to caller (handler will 503).
	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	current := baseTime
	clock := func() time.Time { return current }

	fake := &fakeAggregator{
		fail:    true,
		failErr: errors.New("cold-start failure"),
	}
	cache := newCache(fake, 5*time.Minute, clock)

	snap, err := cache.Snapshot()
	if err == nil {
		t.Fatalf("expected error on cold-start failure, got snap=%v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot on cold-start failure, got %v", snap)
	}
	if !errors.Is(err, fake.failErr) {
		t.Errorf("error should wrap underlying failErr, got %v", err)
	}
}

func TestCache_staleEntryReturnedFromCacheWithinTTL(t *testing.T) {
	// Case 6 (the core HI-05 regression test): once we've served a
	// stale entry, subsequent calls WITHIN the new TTL window must
	// return the same stale value from cache WITHOUT triggering yet
	// another recompute. The old code defeated this by forcing a
	// recompute on every request whenever lastErr was non-nil, which
	// DOS'd the aggregator under sustained failure.
	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	current := baseTime
	clock := func() time.Time { return current }

	fake := &fakeAggregator{}
	cache := newCache(fake, 5*time.Minute, clock)

	// Seed success.
	if _, err := cache.Snapshot(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Fail + advance past TTL → get back a stale snapshot.
	fake.fail = true
	fake.failErr = errors.New("boom")
	current = baseTime.Add(10 * time.Minute)
	stale1, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("stale1: %v", err)
	}
	if !stale1.Stale {
		t.Fatalf("stale1 should be Stale=true")
	}
	if fake.computeCount != 2 {
		t.Fatalf("computeCount after stale1 = %d, want 2", fake.computeCount)
	}

	// Advance within the NEW TTL window — should NOT recompute.
	current = baseTime.Add(10*time.Minute + 1*time.Minute)
	stale2, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("stale2: %v", err)
	}
	if !stale2.Stale {
		t.Errorf("stale2 should still be Stale=true from cache")
	}
	if fake.computeCount != 2 {
		t.Errorf("computeCount = %d after in-TTL call, want 2 (no recompute)", fake.computeCount)
	}

	// Advance within TTL again — still no recompute.
	current = baseTime.Add(10*time.Minute + 4*time.Minute)
	if _, err := cache.Snapshot(); err != nil {
		t.Fatalf("stale3: %v", err)
	}
	if fake.computeCount != 2 {
		t.Errorf("computeCount = %d after second in-TTL call, want 2", fake.computeCount)
	}
}

func TestCache_recoveryAfterStaleFallback(t *testing.T) {
	// Case 7: once the underlying store recovers, the NEXT post-TTL
	// call must produce a fresh (Stale=false) snapshot and reset the
	// cache to the healthy path — we don't want transient failures to
	// leave the cache in a "stale forever" state.
	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	current := baseTime
	clock := func() time.Time { return current }

	fake := &fakeAggregator{}
	cache := newCache(fake, 5*time.Minute, clock)

	// Seed healthy → fail+TTL → stale fallback → recover → fresh.
	if _, err := cache.Snapshot(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fake.fail = true
	fake.failErr = errors.New("boom")
	current = baseTime.Add(10 * time.Minute)
	if snap, _ := cache.Snapshot(); !snap.Stale {
		t.Fatalf("expected stale on first failed retry")
	}

	// Store recovers; advance past the NEW TTL so a retry fires.
	fake.fail = false
	current = baseTime.Add(20 * time.Minute)
	fresh, err := cache.Snapshot()
	if err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if fresh.Stale {
		t.Errorf("after recovery, new snapshot must not be stale")
	}
	if !fresh.ComputedAt.Equal(current) {
		t.Errorf("ComputedAt = %v, want %v", fresh.ComputedAt, current)
	}
	if fake.computeCount != 3 {
		t.Errorf("computeCount = %d, want 3 (seed + failed retry + successful recover)", fake.computeCount)
	}

	// And a call within the NEW (healthy) TTL must serve from cache
	// without recomputing — proving the cache is back to the happy path.
	current = baseTime.Add(21 * time.Minute)
	if _, err := cache.Snapshot(); err != nil {
		t.Fatalf("post-recovery cache hit: %v", err)
	}
	if fake.computeCount != 3 {
		t.Errorf("computeCount = %d after post-recovery in-TTL call, want 3", fake.computeCount)
	}
}

// --- HI-06 regression: vault walk errors surface on the widget ---

// TestCompute_vaultWalkErrorSurfacesOnWidget verifies that when
// collectIngestNotes fails (e.g. filesystem error under the vault root),
// Compute:
//   - does NOT fail the whole snapshot,
//   - stamps IngestActivity.Error with a human-readable reason,
//   - still returns 30 zero-padded day buckets so the frontend shape is
//     stable,
//   - still computes the other widgets (they aren't blocked by the
//     ingest degradation).
//
// We use the collectIngestNotesFn test seam to inject a failing walk
// because the real vault.Walk wrapper silently absorbs root-level stat
// errors and never returns an error to its caller — so a nonexistent-
// path trick at the vault layer cannot reach this branch.
func TestCompute_vaultWalkErrorSurfacesOnWidget(t *testing.T) {
	agg := NewAggregator(nil, nil, nil, nil, nil)
	agg.collectIngestNotesFn = func() ([]ingestNote, error) {
		return nil, errors.New("simulated vault walk failure")
	}

	snap, err := agg.Compute(fixedTime)
	if err != nil {
		t.Fatalf("Compute should not fail on degraded vault walk: %v", err)
	}
	if snap.IngestActivity.Error == "" {
		t.Errorf("IngestActivity.Error should be populated on vault walk failure")
	}
	if len(snap.IngestActivity.Days) != 30 {
		t.Errorf("IngestActivity.Days = %d, want 30 (even on error, shape must be stable)", len(snap.IngestActivity.Days))
	}
	if snap.IngestActivity.Total != 0 {
		t.Errorf("Total = %d, want 0 on degraded path", snap.IngestActivity.Total)
	}
}

// TestCompute_vaultWalkSuccessLeavesErrorEmpty is the happy-path
// counterpart: when collectIngestNotes returns notes cleanly, Compute
// must leave IngestActivity.Error empty so the frontend doesn't show a
// false-alarm banner.
func TestCompute_vaultWalkSuccessLeavesErrorEmpty(t *testing.T) {
	agg := NewAggregator(nil, nil, nil, nil, nil)
	agg.collectIngestNotesFn = func() ([]ingestNote, error) {
		return []ingestNote{
			{folder: "articles", created: fixedTime},
		}, nil
	}

	snap, err := agg.Compute(fixedTime)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if snap.IngestActivity.Error != "" {
		t.Errorf("IngestActivity.Error = %q, want empty on happy path", snap.IngestActivity.Error)
	}
	if snap.IngestActivity.Total != 1 {
		t.Errorf("Total = %d, want 1", snap.IngestActivity.Total)
	}
}

// --- MD-04 regression: parseFrontmatterStrings accepts multiple shapes ---

func TestParseFrontmatterStrings_shapes(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want []string
	}{
		{
			name: "yaml round-trip shape []interface{}",
			in:   []interface{}{"alpha", "beta"},
			want: []string{"alpha", "beta"},
		},
		{
			name: "in-memory shape []string",
			in:   []string{"alpha", "beta"},
			want: []string{"alpha", "beta"},
		},
		{
			name: "scalar string → one-element slice",
			in:   "alpha",
			want: []string{"alpha"},
		},
		{
			name: "[]interface{} with empty strings filtered",
			in:   []interface{}{"alpha", "", "beta"},
			want: []string{"alpha", "beta"},
		},
		{
			name: "[]string with empty strings filtered",
			in:   []string{"alpha", "", "beta"},
			want: []string{"alpha", "beta"},
		},
		{
			name: "[]interface{} with non-string entries dropped",
			in:   []interface{}{"alpha", 42, true, "beta"},
			want: []string{"alpha", "beta"},
		},
		{
			name: "empty []interface{}",
			in:   []interface{}{},
			want: nil,
		},
		{
			name: "empty []string",
			in:   []string{},
			want: nil,
		},
		{
			name: "empty scalar string",
			in:   "",
			want: nil,
		},
		{
			name: "unsupported type (int)",
			in:   42,
			want: nil,
		},
		{
			name: "unsupported type (map)",
			in:   map[string]interface{}{"a": 1},
			want: nil,
		},
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm := map[string]interface{}{"projects": tc.in}
			got := parseFrontmatterStrings(fm, "projects")
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseFrontmatterStrings(%v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseFrontmatterStrings_missingKey(t *testing.T) {
	got := parseFrontmatterStrings(map[string]interface{}{"other": "x"}, "projects")
	if got != nil {
		t.Errorf("missing key should return nil, got %#v", got)
	}
}

// --- test helpers ---

// newTestVault creates a temp-dir vault and returns it. Cleanup on test end.
func newTestVault(t *testing.T) *vault.Vault {
	t.Helper()
	dir, err := os.MkdirTemp("", "dashboard-test-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.MkdirAll(filepath.Join(dir, ".cortical"), 0o700); err != nil {
		t.Fatalf("mkdir .cortical: %v", err)
	}
	return vault.New(dir)
}

func mustUpsert(t *testing.T, store *actions.Store, a *actions.Action) {
	t.Helper()
	if _, err := store.Upsert(a); err != nil {
		t.Fatalf("upsert: %v", err)
	}
}

// Blank-import placeholders kept to prevent unused-import errors when
// sibling tests are selectively commented out during debugging. The
// referenced symbols are otherwise stable public surface we want the
// test file to continue touching.
var _ = prototypes.NewRegistry
var _ = projects.New

// TestParseFrontmatterTime_nativeTimeValue covers LO-03: yaml.v3 can
// unmarshal an ISO-8601 scalar directly into a time.Time rather than a
// string depending on tagging, so parseFrontmatterTime must accept both.
func TestParseFrontmatterTime_nativeTimeValue(t *testing.T) {
	want := time.Date(2026, 4, 11, 9, 0, 0, 0, time.UTC)
	fm := map[string]interface{}{
		"ingested": want,
	}
	got := parseFrontmatterTime(fm, "ingested")
	if !got.Equal(want) {
		t.Errorf("parseFrontmatterTime(time.Time) = %v, want %v", got, want)
	}
}

func TestParseFrontmatterTime_stringRFC3339(t *testing.T) {
	fm := map[string]interface{}{
		"ingested": "2026-04-11T09:00:00Z",
	}
	got := parseFrontmatterTime(fm, "ingested")
	if got.IsZero() {
		t.Errorf("parseFrontmatterTime should parse RFC3339 string")
	}
}

func TestParseFrontmatterTime_unsupportedType(t *testing.T) {
	fm := map[string]interface{}{
		"ingested": 12345, // int — unsupported
	}
	got := parseFrontmatterTime(fm, "ingested")
	if !got.IsZero() {
		t.Errorf("parseFrontmatterTime should return zero for int, got %v", got)
	}
}

// TestBuildProjectsWidget_deterministicTieBreak covers NT-04: two
// projects sharing an identical LastTouched must sort by ID for
// stability across repeated snapshots.
func TestBuildProjectsWidget_deterministicTieBreak(t *testing.T) {
	v := newTestVault(t)
	ps := projects.New(v)
	if _, err := ps.Create(projects.CreateRequest{Name: "Zebra"}); err != nil {
		t.Fatalf("create zebra: %v", err)
	}
	if _, err := ps.Create(projects.CreateRequest{Name: "Apple"}); err != nil {
		t.Fatalf("create apple: %v", err)
	}

	as := actions.New(v)
	// Both actions get the same Updated timestamp to force the tie-break.
	same := fixedTime.Add(-1 * time.Hour)
	mustUpsert(t, as, &actions.Action{
		Description: "zebra task",
		Status:      actions.StatusNext,
		Updated:     same,
		ProjectIDs:  []string{"zebra"},
	})
	mustUpsert(t, as, &actions.Action{
		Description: "apple task",
		Status:      actions.StatusNext,
		Updated:     same,
		ProjectIDs:  []string{"apple"},
	})
	// Post-upsert backdating because Upsert stamps Updated=now on insert.
	for _, a := range as.List() {
		a.Updated = same
	}

	agg := &Aggregator{actions: as, projects: ps}
	var warnings []string
	w := agg.buildProjectsWidget(nil, fixedTime, &warnings)
	if len(w.Top) != 2 {
		t.Fatalf("Top len = %d, want 2", len(w.Top))
	}
	// Apple should sort before Zebra by Name after the LastTouched tie.
	// Names are user-meaningful; UUIDs alone would be visually random.
	if w.Top[0].Slug != "apple" || w.Top[1].Slug != "zebra" {
		t.Errorf("tie-break order wrong: got %v, want [apple zebra]", w.Top)
	}

	// Re-run to confirm stability across refresh.
	for i := 0; i < 3; i++ {
		w2 := agg.buildProjectsWidget(nil, fixedTime, &warnings)
		if w2.Top[0].ID != w.Top[0].ID || w2.Top[1].ID != w.Top[1].ID {
			t.Errorf("refresh %d flipped order", i)
		}
	}
}
