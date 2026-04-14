package dashboard

import (
	"fmt"
	"sync"
	"time"
)

// CacheTTL bounds how stale a cached snapshot may be before a refresh is
// forced on the next request. The PRD mandates this as a single backend
// constant with no runtime configuration (NFR4).
const CacheTTL = 15 * time.Minute

// aggregatorIface is the subset of *Aggregator the cache depends on.
// Extracted as an interface so tests can substitute a failing fake
// without touching real stores. Production always passes *Aggregator.
type aggregatorIface interface {
	Compute(now time.Time) (*Snapshot, error)
}

// Cache wraps an Aggregator with a clock-based TTL and a degraded-path
// fallback — on a recompute failure, it keeps returning the last
// successful snapshot with Stale=true so /dashboard remains usable.
//
// Single-writer semantics: Snapshot() serializes recomputes under a
// mutex. A burst of requests arriving during a slow recompute will all
// block on the same recompute rather than firing parallel walks, so the
// aggregator's file I/O budget is not multiplied by request count.
//
// TTL semantics (fixed in HI-05):
//
//   - Fresh cache entry → serve from cache without recomputing, even
//     if the cached entry is flagged Stale from a prior failure. This
//     is the critical difference from the old code, which forced a
//     recompute on every request whenever lastErr was set and defeated
//     the whole purpose of the TTL.
//
//   - Expired cache entry, recompute succeeds → update cache and
//     expiresAt, serve the fresh snapshot with Stale=false.
//
//   - Expired cache entry, recompute fails, prior cache exists → serve
//     the prior cache with Stale=true. Do NOT touch expiresAt so the
//     next call within the NEW TTL window keeps returning the stale
//     value without retrying. This bounds retry rate under sustained
//     failure instead of hammering the aggregator once per request.
//
//   - Expired cache entry, recompute fails, no prior cache → return
//     the error so the handler can emit 503 (handler already does).
//
//   - Invalidate() drops cache so the next call recomputes.
type Cache struct {
	agg aggregatorIface
	ttl time.Duration
	now func() time.Time

	mu        sync.Mutex
	last      *Snapshot
	expiresAt time.Time
}

// NewCache wires a TTL cache around an aggregator. Pass nil `now` to use
// time.Now — tests inject a fake clock. The aggregator arg accepts any
// aggregatorIface so tests can pass a failing fake; production always
// hands it a *Aggregator.
func NewCache(agg *Aggregator, ttl time.Duration, now func() time.Time) *Cache {
	return newCache(agg, ttl, now)
}

// newCache is the interface-typed internal constructor used by tests to
// inject fake aggregators. Kept unexported so the public surface still
// takes a concrete *Aggregator.
func newCache(agg aggregatorIface, ttl time.Duration, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{agg: agg, ttl: ttl, now: now}
}

// Snapshot returns a fresh snapshot if the cache is empty or expired,
// otherwise returns the cached value. See Cache's doc comment for the
// full TTL / error semantics.
func (c *Cache) Snapshot() (*Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()

	// Serve from cache if still within the TTL window. We explicitly do
	// NOT skip this branch when c.last.Stale is true — a cached-stale
	// entry should still be served from cache until its own TTL expires
	// again, otherwise a transient error turns every request into a
	// fresh Compute until the aggregator recovers (the HI-05 bug).
	if c.last != nil && now.Before(c.expiresAt) {
		return c.last, nil
	}

	// First request OR TTL expired — attempt a recompute.
	fresh, err := c.agg.Compute(now)
	if err != nil {
		if c.last != nil {
			// Return the previous good snapshot with a stale flag so
			// /dashboard keeps working. Crucially, do NOT advance
			// expiresAt: once the new TTL window elapses we retry the
			// recompute again. Advancing expiresAt on failure would
			// mask a permanent store outage behind a forever-stale UI.
			//
			// But: we DO need to guarantee we don't reattempt every
			// single request while the underlying store is sick. The
			// Cache-fresh branch above already handles that for the
			// happy path; after a failure, the next call that arrives
			// within CacheTTL/N will bypass this branch and try again.
			// That's intentional: failed recomputes should be retried
			// at roughly the TTL cadence, which gives the store a
			// chance to recover without hammering it every request.
			//
			// We achieve that by setting expiresAt to now+ttl on a
			// FAILED recompute as well, but mutating the cached value
			// in place to Stale=true so callers see the staleness
			// signal. On the next expiry we retry again. If the retry
			// succeeds, we replace the cache with a fresh (Stale=false)
			// value; if it keeps failing, we keep serving the same
			// stale entry with the retry metadata updated.
			stale := *c.last
			stale.Stale = true
			stale.StaleAttemptAt = now
			stale.StaleReason = fmt.Sprintf("recompute failed: %v", err)
			c.last = &stale
			c.expiresAt = now.Add(c.ttl)
			return c.last, nil
		}
		// No prior cache — nothing to fall back to. Leave expiresAt
		// zero so the very next call retries immediately (no point
		// back-pressuring when there's nothing useful to return).
		return nil, fmt.Errorf("dashboard snapshot: %w", err)
	}

	// Success: cache the fresh snapshot for a full TTL window.
	c.last = fresh
	c.expiresAt = now.Add(c.ttl)
	return fresh, nil
}

// Invalidate drops the cached snapshot. Exists for tests and for a
// potential future manual "refresh" affordance — not called by the
// production code path.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = nil
	c.expiresAt = time.Time{}
}
