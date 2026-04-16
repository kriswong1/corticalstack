package itemusage

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestRecorder(t *testing.T) (*JSONLRecorder, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "item-usage.jsonl")
	rec, err := NewJSONLRecorder(path)
	if err != nil {
		t.Fatalf("NewJSONLRecorder: %v", err)
	}
	t.Cleanup(func() { rec.Close() })
	return rec, path
}

func TestRecordRoundTrip(t *testing.T) {
	rec, path := newTestRecorder(t)
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)

	rec.Record(Entry{
		Timestamp:   now,
		ItemType:    "product",
		ItemID:      "thread-1",
		Model:       "claude-sonnet-4-6",
		InputTokens: 100,
		OutputTokens: 50,
		CostUSD:     0.01,
	})
	rec.Record(Entry{
		Timestamp:   now.Add(time.Minute),
		ItemType:    "product",
		ItemID:      "thread-2",
		Model:       "claude-sonnet-4-6",
		InputTokens: 200,
		OutputTokens: 75,
		CostUSD:     0.02,
	})
	rec.Record(Entry{
		Timestamp:   now.Add(2 * time.Minute),
		ItemType:    "meeting",
		ItemID:      "meeting-1",
		Model:       "claude-opus-4-6",
		InputTokens: 50,
		OutputTokens: 25,
		CostUSD:     0.005,
	})

	r := NewReader(path).WithClock(func() time.Time { return now.Add(time.Hour) })

	// Aggregate across everything.
	got, err := r.Aggregate(Filter{})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 3 {
		t.Errorf("calls = %d, want 3", got.Calls)
	}
	if got.InputTokens != 350 {
		t.Errorf("input tokens = %d, want 350", got.InputTokens)
	}
	if want := 0.035; got.CostUSD < want-1e-9 || got.CostUSD > want+1e-9 {
		t.Errorf("cost = %f, want %f", got.CostUSD, want)
	}
	if got.ByModel["claude-sonnet-4-6"].Calls != 2 {
		t.Errorf("sonnet calls = %d, want 2", got.ByModel["claude-sonnet-4-6"].Calls)
	}
}

func TestFilterByType(t *testing.T) {
	rec, path := newTestRecorder(t)
	rec.Record(Entry{Timestamp: time.Now(), ItemType: "product", ItemID: "p1", InputTokens: 100})
	rec.Record(Entry{Timestamp: time.Now(), ItemType: "meeting", ItemID: "m1", InputTokens: 50})

	r := NewReader(path)
	got, err := r.Aggregate(Filter{Type: "product"})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 1 || got.InputTokens != 100 {
		t.Errorf("got = %+v", got)
	}

	got, err = r.Aggregate(Filter{Type: "meeting"})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 1 || got.InputTokens != 50 {
		t.Errorf("got = %+v", got)
	}
}

func TestFilterByIDs(t *testing.T) {
	rec, path := newTestRecorder(t)
	rec.Record(Entry{Timestamp: time.Now(), ItemType: "product", ItemID: "a", InputTokens: 10})
	rec.Record(Entry{Timestamp: time.Now(), ItemType: "product", ItemID: "b", InputTokens: 20})
	rec.Record(Entry{Timestamp: time.Now(), ItemType: "product", ItemID: "c", InputTokens: 40})

	r := NewReader(path)

	// One id.
	got, err := r.Aggregate(Filter{IDs: []string{"a"}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 1 || got.InputTokens != 10 {
		t.Errorf("one-id got = %+v", got)
	}

	// Two ids — note that {a, c} should sum 50, NOT 30.
	got, err = r.Aggregate(Filter{IDs: []string{"a", "c"}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 2 || got.InputTokens != 50 {
		t.Errorf("two-id got = %+v", got)
	}

	// Empty IDs slice should match everything (no constraint).
	got, err = r.Aggregate(Filter{IDs: []string{}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 3 {
		t.Errorf("empty-id got = %+v", got)
	}
}

func TestFilterByWindow(t *testing.T) {
	rec, path := newTestRecorder(t)
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	// Old entry — outside the 1-hour window.
	rec.Record(Entry{Timestamp: now.Add(-2 * time.Hour), ItemType: "product", ItemID: "old", InputTokens: 100})
	// Recent entry — inside the 1-hour window.
	rec.Record(Entry{Timestamp: now.Add(-30 * time.Minute), ItemType: "product", ItemID: "new", InputTokens: 50})

	r := NewReader(path).WithClock(func() time.Time { return now })
	got, err := r.Aggregate(Filter{Window: time.Hour})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 1 || got.InputTokens != 50 {
		t.Errorf("got = %+v", got)
	}
}

func TestRecordDropsEmptyTypeOrID(t *testing.T) {
	rec, path := newTestRecorder(t)
	// Both should be silently dropped.
	rec.Record(Entry{ItemType: "", ItemID: "p1", InputTokens: 10})
	rec.Record(Entry{ItemType: "product", ItemID: "", InputTokens: 20})

	r := NewReader(path)
	got, err := r.Aggregate(Filter{})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 0 {
		t.Errorf("dropped entries should not appear, got = %+v", got)
	}
}

func TestReaderMissingFile(t *testing.T) {
	r := NewReader(filepath.Join(t.TempDir(), "does-not-exist.jsonl"))
	got, err := r.Aggregate(Filter{})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 0 {
		t.Errorf("missing file should aggregate to empty, got = %+v", got)
	}
}

func TestRecentNewestFirst(t *testing.T) {
	rec, path := newTestRecorder(t)
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	rec.Record(Entry{Timestamp: now, ItemType: "product", ItemID: "old"})
	rec.Record(Entry{Timestamp: now.Add(time.Hour), ItemType: "product", ItemID: "new"})

	r := NewReader(path)
	got, err := r.Recent(Filter{}, 0)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].ItemID != "new" {
		t.Errorf("got[0] = %q, want newest first", got[0].ItemID)
	}
}

func TestAggregateByModelTotals(t *testing.T) {
	// Verifies the per-model breakdown isolates totals correctly:
	// a sonnet call and an opus call should not bleed into each
	// other's ByModel slice.
	rec, path := newTestRecorder(t)
	rec.Record(Entry{Timestamp: time.Now(), ItemType: "product", ItemID: "p1", Model: "claude-sonnet-4-6", InputTokens: 100, CostUSD: 0.01})
	rec.Record(Entry{Timestamp: time.Now(), ItemType: "product", ItemID: "p1", Model: "claude-opus-4-6", InputTokens: 50, CostUSD: 0.05})

	r := NewReader(path)
	got, err := r.Aggregate(Filter{Type: "product", IDs: []string{"p1"}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if got.Calls != 2 || got.InputTokens != 150 {
		t.Errorf("totals = %+v", got)
	}
	if got.ByModel["claude-sonnet-4-6"].InputTokens != 100 {
		t.Errorf("sonnet input = %d, want 100", got.ByModel["claude-sonnet-4-6"].InputTokens)
	}
	if got.ByModel["claude-opus-4-6"].InputTokens != 50 {
		t.Errorf("opus input = %d, want 50", got.ByModel["claude-opus-4-6"].InputTokens)
	}
}
