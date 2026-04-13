package dashboard

import "time"

// StalledThreshold is the age at which an in-flight item is considered
// "stuck". This is a single-source constant consumed by all four dashboard
// widgets — changing it here updates every widget at once. Per the PRD's
// NFR4, there is no configurable per-widget or runtime override.
const StalledThreshold = 7 * 24 * time.Hour

// isStalled reports whether the given update time is older than the
// stalled threshold, relative to `now`. Using `now` as a parameter (not
// time.Now()) keeps the helper deterministic in unit tests.
func isStalled(updated, now time.Time) bool {
	if updated.IsZero() {
		return false
	}
	return now.Sub(updated) >= StalledThreshold
}
