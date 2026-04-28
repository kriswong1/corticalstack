import { useEffect } from "react"
import { useSearchParams } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

// ALL is the dropdown value that means "no filter". We don't use empty
// string because Shadcn's Select disallows empty-string SelectItem values.
export const ALL_PROJECTS = "__all__"

/**
 * useProjectFilter is the URL-synced project-filter state. Reads/writes
 * `?project=<uuid>` so deep links from the dashboard or notification
 * land on the right view, and back/forward navigation stays consistent.
 *
 * Returns [value, setValue] where value is the selected project UUID
 * (or null when no filter is active). setValue(null) clears the filter.
 *
 * Pages that already had hand-rolled state machinery (actions.tsx,
 * prds.tsx) drop their useState + useEffect URL-sync and call this hook
 * instead — the component owns the URL contract.
 */
export function useProjectFilter(): [string | null, (v: string | null) => void] {
  const [params, setParams] = useSearchParams()
  const value = params.get("project")

  const setValue = (next: string | null) => {
    setParams(
      (prev) => {
        const merged = new URLSearchParams(prev)
        if (next) merged.set("project", next)
        else merged.delete("project")
        if (merged.toString() === prev.toString()) return prev
        return merged
      },
      { replace: true },
    )
  }

  return [value, setValue]
}

interface ProjectFilterProps {
  value: string | null
  onChange: (v: string | null) => void
  /** Optional: limit the dropdown to a subset (e.g. PRDs page filters
   * down to projects that actually have a pitch-ready thread). */
  filterFn?: (projectUuid: string) => boolean
  className?: string
}

/**
 * ProjectFilter is the single-select dropdown shared by every entity
 * list page. Reuses the same underlying api.listProjects query key so
 * pages don't refetch — TanStack Query dedupes by queryKey.
 *
 * The optional `filterFn` lets a page narrow the displayed list (for
 * example, prds.tsx wants only projects that have a pitch-ready
 * thread). Filtering at the dropdown layer keeps the URL contract
 * uniform across pages.
 */
export function ProjectFilter({
  value,
  onChange,
  filterFn,
  className,
}: ProjectFilterProps) {
  const { data: projects } = useQuery({
    queryKey: ["projects"],
    queryFn: api.listProjects,
  })

  // If the URL points at a project that no longer exists (deleted, vault
  // out of sync), self-heal by clearing the filter so the page shows
  // everything instead of an empty list.
  useEffect(() => {
    if (!value || !projects) return
    if (!projects.some((p) => p.uuid === value)) onChange(null)
  }, [value, projects, onChange])

  const visible = filterFn
    ? (projects ?? []).filter((p) => filterFn(p.uuid))
    : (projects ?? [])

  return (
    <Select
      value={value ?? ALL_PROJECTS}
      onValueChange={(v) => onChange(v === ALL_PROJECTS ? null : v)}
    >
      <SelectTrigger className={className ?? "border-border rounded-sm w-[200px]"}>
        <SelectValue placeholder="All projects" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL_PROJECTS}>All projects</SelectItem>
        {visible.map((p) => (
          <SelectItem key={p.uuid} value={p.uuid}>
            {p.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
