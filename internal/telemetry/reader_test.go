package telemetry

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kriswong/corticalstack/internal/agent"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func writeFixture(t *testing.T, path string, invs []agent.Invocation) {
	t.Helper()
	rec, err := NewJSONLRecorder(path)
	if err != nil {
		t.Fatalf("NewJSONLRecorder: %v", err)
	}
	for _, inv := range invs {
		rec.Record(inv)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestReaderRecentMissingFileReturnsEmpty(t *testing.T) {
	r := NewReader(filepath.Join(t.TempDir(), "nope.jsonl"))
	got, err := r.Recent(10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestReaderRecentNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	now := time.Now().UTC()
	writeFixture(t, path, []agent.Invocation{
		{Timestamp: now.Add(-2 * time.Hour), SessionID: "old"},
		{Timestamp: now, SessionID: "newest"},
		{Timestamp: now.Add(-1 * time.Hour), SessionID: "middle"},
	})

	r := NewReader(path)
	got, err := r.Recent(10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].SessionID != "newest" || got[1].SessionID != "middle" || got[2].SessionID != "old" {
		t.Errorf("order: %q %q %q", got[0].SessionID, got[1].SessionID, got[2].SessionID)
	}
}

func TestReaderRecentRespectsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	now := time.Now().UTC()
	var invs []agent.Invocation
	for i := 0; i < 20; i++ {
		invs = append(invs, agent.Invocation{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			SessionID: "s",
		})
	}
	writeFixture(t, path, invs)

	r := NewReader(path)
	got, err := r.Recent(5)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("len = %d, want 5", len(got))
	}
}

func TestReaderRecentDefaultLimitWhenZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	now := time.Now().UTC()
	var invs []agent.Invocation
	for i := 0; i < 60; i++ {
		invs = append(invs, agent.Invocation{Timestamp: now, SessionID: "s"})
	}
	writeFixture(t, path, invs)

	r := NewReader(path)
	got, err := r.Recent(0)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 50 {
		t.Errorf("len = %d, want 50 (default)", len(got))
	}
}

func TestReaderSkipsMalformedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	// Hand-craft a file with one valid + one garbage line.
	content := `{"timestamp":"2026-04-14T00:00:00Z","session_id":"good","cost_usd":0.01}
not even close to json
{"timestamp":"2026-04-14T01:00:00Z","session_id":"also-good","cost_usd":0.02}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r := NewReader(path)
	got, err := r.Recent(10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (malformed line skipped)", len(got))
	}
}

func TestReaderSummaryAggregates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	base := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	writeFixture(t, path, []agent.Invocation{
		{Timestamp: base, Model: "claude-sonnet-4-5", InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 200, CacheReadTokens: 300, CostUSD: 0.10},
		{Timestamp: base.Add(time.Hour), Model: "claude-sonnet-4-5", InputTokens: 50, OutputTokens: 25, CostUSD: 0.05},
		{Timestamp: base.Add(2 * time.Hour), Model: "claude-opus-4-6", InputTokens: 200, OutputTokens: 100, CostUSD: 0.50},
	})

	r := NewReader(path).WithClock(func() time.Time { return base.Add(3 * time.Hour) })
	s, err := r.Summary(24 * time.Hour)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}

	if s.TotalCalls != 3 {
		t.Errorf("total_calls = %d, want 3", s.TotalCalls)
	}
	if !approxEqual(s.TotalCostUSD, 0.65) {
		t.Errorf("total_cost = %v, want 0.65", s.TotalCostUSD)
	}
	if s.TotalInputTokens != 350 {
		t.Errorf("total_input = %d, want 350", s.TotalInputTokens)
	}
	if s.TotalOutputTokens != 175 {
		t.Errorf("total_output = %d, want 175", s.TotalOutputTokens)
	}
	if s.TotalCacheCreationTokens != 200 {
		t.Errorf("total_cache_creation = %d, want 200", s.TotalCacheCreationTokens)
	}
	if s.TotalCacheReadTokens != 300 {
		t.Errorf("total_cache_read = %d, want 300", s.TotalCacheReadTokens)
	}

	sonnet := s.ByModel["claude-sonnet-4-5"]
	if sonnet.Calls != 2 || !approxEqual(sonnet.CostUSD, 0.15) {
		t.Errorf("sonnet model totals = %+v", sonnet)
	}
	opus := s.ByModel["claude-opus-4-6"]
	if opus.Calls != 1 || !approxEqual(opus.CostUSD, 0.50) {
		t.Errorf("opus model totals = %+v", opus)
	}

	if len(s.ByDay) != 1 || s.ByDay[0].Day != "2026-04-14" || s.ByDay[0].Calls != 3 {
		t.Errorf("by_day = %+v", s.ByDay)
	}
}

func TestReaderSummaryWindowExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	base := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	writeFixture(t, path, []agent.Invocation{
		{Timestamp: base.Add(-10 * time.Hour), Model: "m", CostUSD: 1.0}, // outside (too old)
		{Timestamp: base.Add(-1 * time.Hour), Model: "m", CostUSD: 2.0},  // inside
		{Timestamp: base, Model: "m", CostUSD: 3.0},                      // boundary: end is exclusive
	})

	r := NewReader(path).WithClock(func() time.Time { return base })
	s, err := r.Summary(2 * time.Hour)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if s.TotalCalls != 1 {
		t.Errorf("total_calls = %d, want 1 (only the inside row)", s.TotalCalls)
	}
	if s.TotalCostUSD != 2.0 {
		t.Errorf("total_cost = %v, want 2.0", s.TotalCostUSD)
	}
}

func TestReaderSummaryEmptyFile(t *testing.T) {
	r := NewReader(filepath.Join(t.TempDir(), "nope.jsonl"))
	s, err := r.Summary(time.Hour)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if s.TotalCalls != 0 {
		t.Errorf("total_calls = %d, want 0", s.TotalCalls)
	}
	if s.ByModel == nil {
		t.Error("by_model should be a non-nil empty map")
	}
}
