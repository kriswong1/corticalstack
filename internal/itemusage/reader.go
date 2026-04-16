package itemusage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Reader scans item-usage.jsonl on demand. No caching — at the
// expected volumes (a few hundred entries per day on a single-user
// vault) a full scan is sub-millisecond, and the file is the source
// of truth so any cache layer would need invalidation hooks the
// recorder doesn't currently fire.
type Reader struct {
	path  string
	clock func() time.Time
}

// NewReader returns a Reader bound to the given file path. A missing
// file is not an error — it just means no entries have been recorded
// yet (typical fresh-install state).
func NewReader(path string) *Reader {
	return &Reader{path: path, clock: time.Now}
}

// WithClock returns a copy of the reader with a custom clock. Used
// by tests to make time-window filters deterministic.
func (r *Reader) WithClock(clock func() time.Time) *Reader {
	return &Reader{path: r.path, clock: clock}
}

// loadAll scans the file once and returns every entry it can parse.
// Malformed lines are silently skipped — same append-only discipline
// as the recorder. A missing file returns an empty slice with no
// error.
func (r *Reader) loadAll() ([]Entry, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("itemusage: open %s: %w", r.path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var out []Entry
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // malformed — skip silently
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("itemusage: scan %s: %w", r.path, err)
	}
	return out, nil
}

// Aggregate scans the file once, applies the filter, and returns
// the summed totals. ByModel is always non-nil so JSON consumers can
// iterate without a nil check.
func (r *Reader) Aggregate(f Filter) (Aggregate, error) {
	all, err := r.loadAll()
	if err != nil {
		return Aggregate{}, err
	}

	now := f.Now
	if now.IsZero() {
		now = r.clock()
	}
	idSet := buildIDSet(f.IDs)

	out := Aggregate{ByModel: make(map[string]ModelTotals)}
	for _, e := range all {
		if !matches(e, f, now, idSet) {
			continue
		}
		out.Calls++
		out.CostUSD += e.CostUSD
		out.InputTokens += e.InputTokens
		out.OutputTokens += e.OutputTokens
		out.CacheCreationTokens += e.CacheCreationTokens
		out.CacheReadTokens += e.CacheReadTokens

		model := e.Model
		if model == "" {
			model = "unknown"
		}
		mt := out.ByModel[model]
		mt.Calls++
		mt.CostUSD += e.CostUSD
		mt.InputTokens += e.InputTokens
		mt.OutputTokens += e.OutputTokens
		mt.CacheCreationTokens += e.CacheCreationTokens
		mt.CacheReadTokens += e.CacheReadTokens
		out.ByModel[model] = mt
	}
	return out, nil
}

// Recent returns the matching entries, newest first, capped at limit.
// limit <= 0 means "no cap". Used by debugging / inspection paths;
// the dashboard cards page uses Aggregate, not Recent.
func (r *Reader) Recent(filter Filter, limit int) ([]Entry, error) {
	all, err := r.loadAll()
	if err != nil {
		return nil, err
	}
	now := filter.Now
	if now.IsZero() {
		now = r.clock()
	}
	idSet := buildIDSet(filter.IDs)

	var matched []Entry
	for _, e := range all {
		if matches(e, filter, now, idSet) {
			matched = append(matched, e)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].Timestamp.After(matched[j].Timestamp)
	})
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	if matched == nil {
		return []Entry{}, nil
	}
	return matched, nil
}

// matches applies a Filter to one Entry. Pulled out so Aggregate and
// Recent share exactly the same selection logic.
func matches(e Entry, f Filter, now time.Time, idSet map[string]bool) bool {
	if f.Type != "" && e.ItemType != f.Type {
		return false
	}
	if idSet != nil {
		if !idSet[e.ItemID] {
			return false
		}
	}
	if f.Window > 0 {
		cutoff := now.Add(-f.Window)
		if e.Timestamp.Before(cutoff) {
			return false
		}
	}
	return true
}

// buildIDSet returns a presence map for non-empty IDs, or nil when
// the filter list is empty. Returning nil signals "no id constraint"
// to matches() so a zero-length slice and a nil slice behave the
// same — both mean "match anything" for this dimension.
func buildIDSet(ids []string) map[string]bool {
	if len(ids) == 0 {
		return nil
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
