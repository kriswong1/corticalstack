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

// Cache wraps an Aggregator with a clock-based TTL and a degraded-path
// fallback — on a recompute failure, it keeps returning the last
// successful snapshot with Stale=true so /dashboard remains usable.
//
// Single-writer semantics: Snapshot() serializes recomputes under a
// mutex. A burst of requests arriving during a slow recompute will all
// block on the same recompute rather than firing parallel walks, so the
// aggregator's file I/O budget is not multiplied by request count.
type Cache struct {
	agg  *Aggregator
	ttl  time.Duration
	now  func() time.Time

	mu          sync.Mutex
	last        *Snapshot
	lastErr     error
	lastAttempt time.Time
}

// NewCache wires a TTL cache around an aggregator. Pass nil `now` to use
// time.Now — tests inject a fake clock.
func NewCache(agg *Aggregator, ttl time.Duration, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{agg: agg, ttl: ttl, now: now}
}

// Snapshot returns a fresh snapshot if the cache is empty or expired,
// otherwise returns the cached value. On a recompute error with a cached
// snapshot available, returns the cached snapshot with Stale=true. On
// error with no cache, returns the error so the handler can emit 503.
func (c *Cache) Snapshot() (*Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()

	// Serve from cache if fresh.
	if c.last != nil && now.Sub(c.last.ComputedAt) < c.ttl && c.lastErr == nil {
		return c.last, nil
	}

	// TTL expired or first request — attempt a recompute.
	fresh, err := c.agg.Compute(now)
	c.lastAttempt = now
	if err != nil {
		c.lastErr = err
		if c.last != nil {
			// Return the previous good snapshot with a stale flag so
			// /dashboard keeps working while we log the error upstream.
			stale := *c.last
			stale.Stale = true
			stale.StaleAttemptAt = now
			stale.StaleReason = fmt.Sprintf("recompute failed: %v", err)
			return &stale, nil
		}
		return nil, fmt.Errorf("dashboard snapshot: %w", err)
	}

	c.last = fresh
	c.lastErr = nil
	return fresh, nil
}

// Invalidate drops the cached snapshot. Exists for tests and for a
// potential future manual "refresh" affordance — not called by the
// production code path.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = nil
	c.lastErr = nil
}
