import { useState, useEffect } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useSearchParams } from "react-router-dom"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { api } from "@/lib/api"
import type { Action, ActionStatus } from "@/types/api"
import { RefreshCw, Pencil } from "lucide-react"

const allStatuses: ActionStatus[] = [
  "inbox",
  "next",
  "waiting",
  "doing",
  "someday",
  "deferred",
  "done",
  "cancelled",
]

const statusColors: Record<string, string> = {
  inbox: "bg-muted text-muted-foreground",
  next: "bg-secondary text-secondary-foreground",
  waiting: "bg-[var(--stripe-lemon)]/20 text-[var(--stripe-lemon)]",
  doing: "bg-primary/20 text-primary",
  someday: "bg-muted text-muted-foreground",
  deferred: "bg-muted text-muted-foreground",
  done: "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)]",
  cancelled: "bg-destructive/20 text-destructive",
}

const priorityColors: Record<string, string> = {
  p1: "bg-destructive/20 text-destructive",
  p2: "bg-secondary text-secondary-foreground",
  p3: "bg-muted text-muted-foreground",
}

const priorityLabels: Record<string, string> = {
  p1: "High",
  p2: "Med",
  p3: "Low",
}

const allPriorities = ["p1", "p2", "p3"]
const allEfforts = ["xs", "s", "m", "l", "xl"]
const allContexts = ["deep-work", "quick", "research", "write", "review"]

// STALLED_MS matches the backend's StalledThreshold (7 days). An action
// is stalled when its status is doing or waiting AND its updated_at is
// older than this threshold. Click destinations from the dashboard
// /actions?stalled=true filter to this set.
const STALLED_MS = 7 * 24 * 60 * 60 * 1000

export function ActionsPage() {
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const [editAction, setEditAction] = useState<Action | null>(null)
  // Hydrate filters from the URL on first render so dashboard deep links
  // land on the right view. Subsequent filter chip clicks update local
  // state AND the URL so back/forward navigation stays consistent.
  const [filterStatus, setFilterStatus] = useState<string | null>(
    searchParams.get("status"),
  )
  const [filterProject, setFilterProject] = useState<string | null>(null)
  const [filterContext, setFilterContext] = useState<string | null>(null)
  const [filterStalled, setFilterStalled] = useState<boolean>(
    searchParams.get("stalled") === "true",
  )

  // Sync filter state back to the URL so the address bar reflects the
  // current view. `replace: true` keeps history tidy — every chip click
  // shouldn't push a new history entry.
  useEffect(() => {
    const next = new URLSearchParams(searchParams)
    if (filterStatus) next.set("status", filterStatus)
    else next.delete("status")
    if (filterStalled) next.set("stalled", "true")
    else next.delete("stalled")
    setSearchParams(next, { replace: true })
    // searchParams intentionally omitted — including it creates an
    // update loop since setSearchParams produces a new instance.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterStatus, filterStalled])

  const { data: actions, isLoading } = useQuery({
    queryKey: ["actions"],
    queryFn: () => api.listActions(),
  })

  const { data: counts } = useQuery({
    queryKey: ["action-counts"],
    queryFn: api.getActionCounts,
  })

  // Derive unique projects and contexts from actions for filter dropdowns.
  const uniqueProjects = Array.from(
    new Set(actions?.flatMap((a) => a.project_ids ?? []) ?? []),
  ).sort()
  const uniqueContexts = Array.from(
    new Set(actions?.map((a) => a.context).filter(Boolean) as string[]),
  ).sort()

  // Apply client-side filters. Stalled is cross-status (doing + waiting
  // older than STALLED_MS) and not exposed via the normal filter chips —
  // it's triggered by the dashboard link and cleared via the "Clear
  // filters" button.
  const now = Date.now()
  const filtered = actions?.filter((a) => {
    if (filterStatus && a.status !== filterStatus) return false
    if (filterProject && !(a.project_ids ?? []).includes(filterProject)) return false
    if (filterContext && a.context !== filterContext) return false
    if (filterStalled) {
      if (a.status !== "doing" && a.status !== "waiting") return false
      const updatedMs = new Date(a.updated).getTime()
      if (!isFinite(updatedMs) || now - updatedMs < STALLED_MS) return false
    }
    return true
  })

  const hasFilters = filterStatus || filterProject || filterContext || filterStalled

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["actions"] })
    queryClient.invalidateQueries({ queryKey: ["action-counts"] })
  }

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      api.setActionStatus(id, status),
    onSuccess: () => {
      invalidate()
      toast.success("Status updated")
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Status update failed")
    },
  })

  const reconcileMutation = useMutation({
    mutationFn: api.reconcileActions,
    onSuccess: (result) => {
      invalidate()
      if (result.updated > 0) {
        toast.success(`Synced ${result.unique_actions} actions across ${result.scanned} files — ${result.updated} updated`)
      } else {
        toast.info(`${result.unique_actions} actions across ${result.scanned} files — all in sync`)
      }
    },
  })

  return (
    <>
      <PageHeader title="Actions" description="GTD-inspired action tracking with WIP limits">
        <Button
          variant="outline"
          onClick={() => reconcileMutation.mutate()}
          disabled={reconcileMutation.isPending}
          className="border-border rounded-sm font-normal gap-1.5"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${reconcileMutation.isPending ? "animate-spin" : ""}`} />
          Reconcile
        </Button>
      </PageHeader>

      {counts && (
        <div className="flex flex-wrap gap-2 mb-4">
          {allStatuses.map((s) => (
            <button
              key={s}
              onClick={() => setFilterStatus(filterStatus === s ? null : s)}
              className="cursor-pointer"
            >
              <Badge
                className={`text-[10px] font-light rounded-sm px-1.5 py-px transition-opacity ${statusColors[s] ?? ""} ${filterStatus && filterStatus !== s ? "opacity-40" : ""} ${filterStatus === s ? "ring-1 ring-ring" : ""}`}
              >
                {s}: {counts[s] ?? 0}
              </Badge>
            </button>
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-3 mb-4">
        {uniqueProjects.length > 0 && (
          <Select value={filterProject ?? "_all"} onValueChange={(v) => setFilterProject(v === "_all" ? null : v)}>
            <SelectTrigger className="h-7 w-36 text-xs border-border rounded-sm">
              <SelectValue placeholder="All projects" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="_all">All projects</SelectItem>
              {uniqueProjects.map((p) => (
                <SelectItem key={p} value={p}>{p}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {uniqueContexts.length > 0 && (
          <Select value={filterContext ?? "_all"} onValueChange={(v) => setFilterContext(v === "_all" ? null : v)}>
            <SelectTrigger className="h-7 w-36 text-xs border-border rounded-sm">
              <SelectValue placeholder="All contexts" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="_all">All contexts</SelectItem>
              {uniqueContexts.map((c) => (
                <SelectItem key={c} value={c}>@{c}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {filterStalled && (
          <span className="rounded-sm bg-[var(--stripe-lemon)]/20 px-1.5 py-0.5 text-[11px] font-normal text-[var(--stripe-lemon)]">
            stalled &gt; 7 days
          </span>
        )}
        {hasFilters && (
          <button
            onClick={() => {
              setFilterStatus(null)
              setFilterProject(null)
              setFilterContext(null)
              setFilterStalled(false)
            }}
            className="text-xs text-primary hover:underline"
          >
            Clear filters
          </button>
        )}
        {hasFilters && (
          <span className="text-xs text-muted-foreground">
            {filtered?.length ?? 0} of {actions?.length ?? 0} actions
          </span>
        )}
      </div>

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <div className="rounded-md border border-border overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal w-8" />
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Description</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal w-28">Status</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal w-16">Pri</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal w-16">Effort</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal w-20">Context</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Deadline</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Projects</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered?.map((action) => (
                <TableRow key={action.id}>
                  <TableCell className="px-2">
                    <button
                      onClick={() => setEditAction(action)}
                      className="rounded-sm p-1 text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                  </TableCell>
                  <TableCell className="text-sm font-light max-w-[300px]">
                    {action.title && (
                      <span className="font-normal text-foreground line-clamp-1">{action.title}</span>
                    )}
                    <span className={`line-clamp-2 ${action.title ? "text-xs text-muted-foreground" : ""}`}>
                      {action.description}
                    </span>
                  </TableCell>
                  <TableCell>
                    <Select
                      value={action.status}
                      onValueChange={(value) =>
                        statusMutation.mutate({ id: action.id, status: value })
                      }
                    >
                      <SelectTrigger className="h-7 w-24 text-xs border-border rounded-sm">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {allStatuses.map((s) => (
                          <SelectItem key={s} value={s}>{s}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </TableCell>
                  <TableCell>
                    {action.priority && (
                      <Badge className={`text-[10px] font-light rounded-sm px-1.5 py-px ${priorityColors[action.priority] ?? ""}`}>
                        {priorityLabels[action.priority] ?? action.priority}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    {action.effort && (
                      <Badge variant="outline" className="text-[10px] font-normal rounded-sm px-1 uppercase">
                        {action.effort}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    {action.context && (
                      <span className="text-[10px] text-muted-foreground">@{action.context}</span>
                    )}
                  </TableCell>
                  <TableCell className="text-xs font-light text-muted-foreground font-[font-feature-settings:'tnum'] whitespace-nowrap">
                    {action.deadline || "\u2014"}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {action.project_ids?.map((pid) => (
                        <Badge key={pid} variant="outline" className="text-[10px] font-normal rounded-sm px-1">
                          {pid}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {filtered?.length === 0 && (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-sm text-muted-foreground py-8">
                    {hasFilters ? "No actions match the current filters." : "No actions tracked yet."}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}

      {editAction && (
        <EditActionDialog
          action={editAction}
          onClose={() => setEditAction(null)}
          onSaved={() => {
            setEditAction(null)
            invalidate()
          }}
        />
      )}
    </>
  )
}

function EditActionDialog({
  action,
  onClose,
  onSaved,
}: {
  action: Action
  onClose: () => void
  onSaved: () => void
}) {
  const [title, setTitle] = useState(action.title ?? "")
  const [description, setDescription] = useState(action.description)
  const [owner, setOwner] = useState(action.owner)
  const [deadline, setDeadline] = useState(action.deadline ?? "")
  const [status, setStatus] = useState(action.status)
  const [priority, setPriority] = useState<string>(action.priority ?? "p2")
  const [effort, setEffort] = useState<string>(action.effort ?? "m")
  const [context, setContext] = useState(action.context ?? "")
  const [saving, setSaving] = useState(false)

  async function handleSave() {
    setSaving(true)
    try {
      await api.updateAction(action.id, {
        title,
        description,
        owner,
        deadline,
        status,
        priority: priority as Action["priority"],
        effort: effort as Action["effort"],
        context,
      })
      toast.success("Action updated")
      onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Update failed")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }}>
      <DialogContent className="shadow-stripe-deep rounded-md max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-[22px] font-light tracking-[-0.22px]">
            Edit Action
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label className="text-[var(--stripe-label)] text-sm font-normal">Title</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Short action title" className="border-border rounded-sm" />
          </div>
          <div className="space-y-2">
            <Label className="text-[var(--stripe-label)] text-sm font-normal">Description</Label>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="border-border rounded-sm"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label className="text-[var(--stripe-label)] text-sm font-normal">Owner</Label>
              <Input value={owner} onChange={(e) => setOwner(e.target.value)} className="border-border rounded-sm" />
            </div>
            <div className="space-y-2">
              <Label className="text-[var(--stripe-label)] text-sm font-normal">Deadline</Label>
              <Input
                type="date"
                value={deadline}
                onChange={(e) => setDeadline(e.target.value)}
                className="border-border rounded-sm"
              />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label className="text-[var(--stripe-label)] text-sm font-normal">Status</Label>
              <Select value={status} onValueChange={(v) => setStatus(v as ActionStatus)}>
                <SelectTrigger className="border-border rounded-sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {allStatuses.map((s) => (
                    <SelectItem key={s} value={s}>{s}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label className="text-[var(--stripe-label)] text-sm font-normal">Priority</Label>
              <Select value={priority} onValueChange={setPriority}>
                <SelectTrigger className="border-border rounded-sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {allPriorities.map((p) => (
                    <SelectItem key={p} value={p}>{priorityLabels[p]} ({p})</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label className="text-[var(--stripe-label)] text-sm font-normal">Effort</Label>
              <Select value={effort} onValueChange={setEffort}>
                <SelectTrigger className="border-border rounded-sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {allEfforts.map((e) => (
                    <SelectItem key={e} value={e}>{e.toUpperCase()}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-2">
            <Label className="text-[var(--stripe-label)] text-sm font-normal">Context</Label>
            <Select value={context || "_none"} onValueChange={(v) => setContext(v === "_none" ? "" : v)}>
              <SelectTrigger className="border-border rounded-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="_none">None</SelectItem>
                {allContexts.map((c) => (
                  <SelectItem key={c} value={c}>@{c}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center justify-between pt-2">
            <span className="text-xs text-muted-foreground font-mono">{action.id}</span>
            <div className="flex gap-2">
              <Button
                variant="outline"
                onClick={onClose}
                className="border-border rounded-sm font-normal"
              >
                Cancel
              </Button>
              <Button
                onClick={handleSave}
                disabled={saving || !description.trim()}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {saving ? "Saving..." : "Save Changes"}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
