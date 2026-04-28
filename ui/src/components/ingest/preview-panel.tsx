import { useEffect, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { PreviewResult } from "@/types/api"
import { api } from "@/lib/api"
import { Plus, X } from "lucide-react"

const intentions = [
  "learning",
  "information",
  "research",
  "project-application",
  "other",
]

interface PreviewPanelProps {
  preview: PreviewResult
  jobId: string
  onConfirmed: () => void
}

export function PreviewPanel({ preview, jobId, onConfirmed }: PreviewPanelProps) {
  const queryClient = useQueryClient()
  const [title, setTitle] = useState(preview.suggested_title ?? "")
  const [intention, setIntention] = useState(preview.intention)
  const [why, setWhy] = useState("")
  const [confirming, setConfirming] = useState(false)
  const [creatingProject, setCreatingProject] = useState(false)
  const [projectError, setProjectError] = useState<string | null>(null)

  const { data: allProjects } = useQuery({
    queryKey: ["projects"],
    queryFn: api.listProjects,
  })

  // Phase 4 — selectedProjects is the set of canonical UUIDs the user
  // wants attached. Hydrated from preview.suggested_project_ids: those
  // come back from Claude as slugs (sourced from the active list), so
  // we resolve each to a UUID via the projects store. Anything that
  // doesn't resolve is silently dropped — the runConfirm validator will
  // do the same on the backend, so showing an unresolvable id here just
  // confuses the user.
  const [selected, setSelected] = useState<Set<string>>(new Set())
  useEffect(() => {
    if (!allProjects) return
    const next = new Set<string>()
    for (const ref of preview.suggested_project_ids ?? []) {
      const found = allProjects.find((p) => p.uuid === ref || p.slug === ref)
      if (found) next.add(found.uuid)
    }
    setSelected(next)
    // Only run when the project list arrives or the preview changes;
    // avoid re-clobbering the user's edits on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allProjects, preview.suggested_project_ids?.join(",")])

  const proposed = preview.proposed_project_name?.trim() ?? ""

  async function handleCreateProposed() {
    if (!proposed) return
    setCreatingProject(true)
    setProjectError(null)
    try {
      const created = await api.createProject({ name: proposed })
      // Refetch the list so the dropdown picks up the new entry.
      await queryClient.invalidateQueries({ queryKey: ["projects"] })
      setSelected((prev) => new Set(prev).add(created.uuid))
    } catch (err) {
      setProjectError(err instanceof Error ? err.message : String(err))
    } finally {
      setCreatingProject(false)
    }
  }

  async function handleConfirm() {
    setConfirming(true)
    try {
      await api.confirmJob(jobId, {
        title,
        intention,
        project_ids: Array.from(selected),
        why,
      })
      onConfirmed()
    } catch (err) {
      alert("Confirm failed: " + (err instanceof Error ? err.message : String(err)))
    } finally {
      setConfirming(false)
    }
  }

  const selectedProjects = (allProjects ?? []).filter((p) => selected.has(p.uuid))
  const addable = (allProjects ?? []).filter((p) => !selected.has(p.uuid))

  return (
    <div className="rounded-md border border-primary/30 bg-secondary/30 p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-base font-normal text-foreground">
          Claude's Proposal
        </h3>
        {preview.confidence != null && (
          <span className="text-xs text-muted-foreground">
            Confidence: {(preview.confidence * 100).toFixed(0)}%
          </span>
        )}
      </div>

      {preview.reasoning && (
        <p className="text-xs font-light text-muted-foreground">
          {preview.reasoning}
        </p>
      )}

      <p className="text-sm font-light text-foreground">{preview.summary}</p>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label className="text-[var(--stripe-label)] text-sm font-normal">
            Title
          </Label>
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="border-border rounded-sm"
          />
        </div>
        <div className="space-y-2">
          <Label className="text-[var(--stripe-label)] text-sm font-normal">
            Intention
          </Label>
          <Select value={intention} onValueChange={setIntention}>
            <SelectTrigger className="border-border rounded-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {intentions.map((i) => (
                <SelectItem key={i} value={i}>
                  {i}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-sm font-normal">
          Projects
        </Label>

        {/* Selected chips */}
        <div className="flex flex-wrap gap-1.5 min-h-[28px]">
          {selectedProjects.length === 0 && (
            <span className="text-xs text-muted-foreground italic">
              None selected
            </span>
          )}
          {selectedProjects.map((p) => (
            <Badge
              key={p.uuid}
              variant="outline"
              className="text-xs gap-1 pr-1"
            >
              {p.name}
              <button
                type="button"
                onClick={() =>
                  setSelected((prev) => {
                    const next = new Set(prev)
                    next.delete(p.uuid)
                    return next
                  })
                }
                className="hover:text-destructive"
                aria-label={`Remove ${p.name}`}
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>

        {/* Add from existing */}
        {addable.length > 0 && (
          <Select
            value=""
            onValueChange={(v) =>
              setSelected((prev) => new Set(prev).add(v))
            }
          >
            <SelectTrigger className="border-border rounded-sm h-8 text-xs">
              <SelectValue placeholder="+ Add project" />
            </SelectTrigger>
            <SelectContent>
              {addable.map((p) => (
                <SelectItem key={p.uuid} value={p.uuid}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        {/* Phase 4 — Claude proposed a project that doesn't exist. Surface
            a single explicit "Create new?" affordance instead of silently
            auto-creating. */}
        {proposed && !selected.has(proposed) && (
          <div className="rounded-sm border border-dashed border-primary/40 bg-primary/5 p-2.5 flex items-center justify-between gap-2">
            <span className="text-xs text-foreground">
              Claude suggests a new project: <strong>{proposed}</strong>
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleCreateProposed}
              disabled={creatingProject}
              className="h-7 text-xs gap-1"
            >
              <Plus className="h-3 w-3" />
              {creatingProject ? "Creating..." : "Create"}
            </Button>
          </div>
        )}
        {projectError && (
          <p className="text-xs text-destructive">{projectError}</p>
        )}
      </div>

      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-sm font-normal">
          Why (optional context)
        </Label>
        <Input
          value={why}
          onChange={(e) => setWhy(e.target.value)}
          placeholder="Why did you save this?"
          className="border-border rounded-sm"
        />
      </div>

      <Button
        onClick={handleConfirm}
        disabled={confirming}
        className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-base"
      >
        {confirming ? "Confirming..." : "Confirm & Extract"}
      </Button>
    </div>
  )
}
