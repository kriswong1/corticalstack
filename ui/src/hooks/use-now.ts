import { useSyncExternalStore } from "react"

// useNow returns a reactive "current timestamp" that refreshes on an
// interval. Use it anywhere you'd otherwise call `Date.now()` during
// render — the hook makes the component re-render when the clock ticks,
// so time-sensitive derived state (stalled filters, relative dates,
// countdowns) stays live without waiting for an unrelated refetch.
//
// Default tick is 60s because every consumer here compares against
// multi-hour thresholds; a faster tick would waste renders. Pass a
// smaller interval for countdowns or sub-minute staleness.
export function useNow(intervalMs: number = 60_000): number {
  return useSyncExternalStore(
    (onStoreChange) => {
      const id = window.setInterval(onStoreChange, intervalMs)
      return () => window.clearInterval(id)
    },
    () => Date.now(),
    () => Date.now(),
  )
}
