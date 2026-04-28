import { useEffect, useState } from "react"

const KEY = "cs.pinnedProjectUUID"

// Module-level event so multiple hook instances on the same page (sidebar
// picker + filter on the same page + chips elsewhere) all repaint when
// the pin changes — without going through context. The native `storage`
// event only fires across tabs, not within the same window.
const listeners = new Set<(v: string | null) => void>()
function emit(v: string | null) {
  for (const fn of listeners) fn(v)
}

function read(): string | null {
  if (typeof window === "undefined") return null
  try {
    return window.localStorage.getItem(KEY)
  } catch {
    return null
  }
}

function write(v: string | null) {
  if (typeof window === "undefined") return
  try {
    if (v) window.localStorage.setItem(KEY, v)
    else window.localStorage.removeItem(KEY)
  } catch {
    // localStorage may be disabled (incognito + strict mode); persistence
    // best-effort, the in-memory listeners still fire so the UI updates.
  }
  emit(v)
}

/**
 * usePinnedProject is the localStorage-backed pinned-project hook.
 * Returns [uuid, setUuid] where setUuid(null) clears the pin.
 *
 * Persists across reloads via localStorage; updates ripple to every
 * mounted instance of the hook in the same window via a module-level
 * listener set. The native `storage` event handles cross-tab sync.
 */
export function usePinnedProject(): [string | null, (v: string | null) => void] {
  const [value, setValue] = useState<string | null>(read)

  useEffect(() => {
    const onLocal = (v: string | null) => setValue(v)
    listeners.add(onLocal)
    const onStorage = (e: StorageEvent) => {
      if (e.key === KEY) setValue(e.newValue)
    }
    if (typeof window !== "undefined") {
      window.addEventListener("storage", onStorage)
    }
    return () => {
      listeners.delete(onLocal)
      if (typeof window !== "undefined") {
        window.removeEventListener("storage", onStorage)
      }
    }
  }, [])

  return [value, write]
}
