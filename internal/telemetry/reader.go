package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/kriswong/corticalstack/internal/agent"
)

// Reader scans a JSONL telemetry file on demand. No caching for v1 —
// a single-file scan of <100k lines is sub-millisecond. If perf ever
// matters, see internal/dashboard/cache.go for the TTL+stale-fallback
// idiom this project uses for heavier aggregators.
//
// Read paths are independent of any concurrent JSONLRecorder writer
// on the same path: each Reader call opens its own read-only handle.
type Reader struct {
	path  string
	clock func() time.Time // injectable for tests; defaults to time.Now
}

// NewReader returns a Reader bound to the given file path. A missing
// file is not an error — it just means no calls have been recorded yet.
func NewReader(path string) *Reader {
	return &Reader{path: path, clock: time.Now}
}

// WithClock returns a copy of the reader with a custom clock. Used by
// tests to make Summary(window) deterministic without faking time.Now.
func (r *Reader) WithClock(clock func() time.Time) *Reader {
	return &Reader{path: r.path, clock: clock}
}

// loadAll scans the file once and returns every invocation it can
// parse. Malformed lines are silently skipped. A missing file returns
// an empty slice with no error.
func (r *Reader) loadAll() ([]agent.Invocation, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("telemetry: open %s: %w", r.path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var out []agent.Invocation
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var inv agent.Invocation
		if err := json.Unmarshal(line, &inv); err != nil {
			continue // malformed — skip silently, matches recorder's append-only model
		}
		out = append(out, inv)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: scan %s: %w", r.path, err)
	}
	return out, nil
}

// Recent returns the most recent N invocations, newest first. If limit
// is <= 0 a default of 50 is used. A missing file returns an empty slice.
//
// Load-and-slice is fine for v1: even at 1k calls/day, scanning a
// year of history is microseconds. If the file ever crosses 100k
// lines, swap to a reverse-scan helper — it's a 30-line change.
func (r *Reader) Recent(limit int) ([]agent.Invocation, error) {
	if limit <= 0 {
		limit = 50
	}
	all, err := r.loadAll()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})
	if len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		return []agent.Invocation{}, nil
	}
	return all, nil
}

// Summary aggregates totals over the trailing window relative to
// the reader's clock. Wraps SummaryBetween so the testable absolute-
// time path is the real implementation.
func (r *Reader) Summary(window time.Duration) (Summary, error) {
	now := r.clock()
	return r.SummaryBetween(now.Add(-window), now)
}

// SummaryBetween aggregates invocations whose Timestamp falls in
// [start, end). Inclusive on the lower bound, exclusive on the upper —
// standard half-open window so consecutive windows tile cleanly.
func (r *Reader) SummaryBetween(start, end time.Time) (Summary, error) {
	all, err := r.loadAll()
	if err != nil {
		return Summary{}, err
	}

	s := Summary{
		Start:   start,
		End:     end,
		ByModel: make(map[string]ModelTotals),
	}
	dayBuckets := make(map[string]*DayTotals)

	for _, inv := range all {
		if inv.Timestamp.Before(start) || !inv.Timestamp.Before(end) {
			continue
		}
		s.TotalCalls++
		s.TotalCostUSD += inv.CostUSD
		s.TotalInputTokens += inv.InputTokens
		s.TotalOutputTokens += inv.OutputTokens
		s.TotalCacheCreationTokens += inv.CacheCreationTokens
		s.TotalCacheReadTokens += inv.CacheReadTokens

		model := inv.Model
		if model == "" {
			model = "unknown"
		}
		mt := s.ByModel[model]
		mt.Calls++
		mt.CostUSD += inv.CostUSD
		mt.InputTokens += inv.InputTokens
		mt.OutputTokens += inv.OutputTokens
		mt.CacheCreationTokens += inv.CacheCreationTokens
		mt.CacheReadTokens += inv.CacheReadTokens
		s.ByModel[model] = mt

		dayKey := inv.Timestamp.UTC().Format("2006-01-02")
		dt, ok := dayBuckets[dayKey]
		if !ok {
			dt = &DayTotals{Day: dayKey}
			dayBuckets[dayKey] = dt
		}
		dt.Calls++
		dt.CostUSD += inv.CostUSD
		dt.InputTokens += inv.InputTokens
		dt.OutputTokens += inv.OutputTokens
		dt.CacheCreationTokens += inv.CacheCreationTokens
		dt.CacheReadTokens += inv.CacheReadTokens
	}

	s.ByDay = make([]DayTotals, 0, len(dayBuckets))
	for _, dt := range dayBuckets {
		s.ByDay = append(s.ByDay, *dt)
	}
	sort.Slice(s.ByDay, func(i, j int) bool {
		return s.ByDay[i].Day < s.ByDay[j].Day
	})

	return s, nil
}

// Summary is the aggregated view over a time window.
type Summary struct {
	Start                    time.Time              `json:"start"`
	End                      time.Time              `json:"end"`
	TotalCalls               int                    `json:"total_calls"`
	TotalCostUSD             float64                `json:"total_cost_usd"`
	TotalInputTokens         int                    `json:"total_input_tokens"`
	TotalOutputTokens        int                    `json:"total_output_tokens"`
	TotalCacheCreationTokens int                    `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int                    `json:"total_cache_read_tokens"`
	ByModel                  map[string]ModelTotals `json:"by_model"`
	ByDay                    []DayTotals            `json:"by_day"`
}

// ModelTotals is the per-model slice of a Summary.
type ModelTotals struct {
	Calls               int     `json:"calls"`
	CostUSD             float64 `json:"cost_usd"`
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
}

// DayTotals is one entry in a Summary's day-by-day breakdown. Day is
// a UTC YYYY-MM-DD string so JSON consumers don't have to parse a
// timezone offset.
type DayTotals struct {
	Day                 string  `json:"day"`
	Calls               int     `json:"calls"`
	CostUSD             float64 `json:"cost_usd"`
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
}
