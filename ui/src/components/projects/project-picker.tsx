import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { usePinnedProject } from "@/hooks/use-pinned-project"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

const NONE = "__none__"

/**
 * ProjectPicker is the sidebar pin control. Sets the
 * `cs.pinnedProjectUUID` localStorage key, which:
 *   - becomes the default for every entity list page's ProjectFilter
 *     (unless the URL already has ?project=...),
 *   - persists across reloads,
 *   - syncs across tabs via the native `storage` event.
 *
 * Excluded from the ingest classifier prompt by design — pinning is a
 * UI preference, not a hint for extraction.
 */
export function ProjectPicker() {
  const [pinned, setPinned] = usePinnedProject()
  const { data: projects } = useQuery({
    queryKey: ["projects"],
    queryFn: api.listProjects,
  })

  return (
    <div className="px-2 py-1.5">
      <Select
        value={pinned ?? NONE}
        onValueChange={(v) => setPinned(v === NONE ? null : v)}
      >
        <SelectTrigger className="h-8 text-xs border-border rounded-sm w-full">
          <SelectValue placeholder="Pin a project..." />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NONE}>All projects</SelectItem>
          {(projects ?? []).map((p) => (
            <SelectItem key={p.uuid} value={p.uuid}>
              {p.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
