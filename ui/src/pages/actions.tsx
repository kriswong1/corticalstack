import { useState, useMemo, useRef, useEffect } from "react"
import type { KeyboardEvent } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useSearchParams } from "react-router-dom"
import { toast } from "sonner"
import { Breadcrumbs } from "@/components/layout/breadcrumbs"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { api, ApiError, getErrorMessage } from "@/lib/api"
import type {
  Action,
  ActionStatus,
  ActionPriority,
  ActionEffort,
  Project,
} from "@/types/api"
import {
  RefreshCw,
  Plus,
  Star,
  Sun,
  CalendarDays,
  ListChecks,
  Circle,
  CheckCircle2,
  X,
  ChevronRight,
  ChevronDown,
  StickyNote,
  Folder,
  CircleDashed,
} from "lucide-react"

// SmartListId is the closed set of built-in lists (not project-scoped).
type SmartListId = "my-day" | "important" | "planned" | "all"

// ListSelection is the discriminated union driving the right pane.
// Persisted in the URL via ?list=my-day | important | planned | all | p:<uuid>.
type ListSelection =
  | { kind: "smart"; id: SmartListId }
  | { kind: "project"; projectId: string }

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
const allPriorities: ActionPriority[] = ["p1", "p2", "p3"]
const allEfforts: ActionEffort[] = ["xs", "s", "m", "l", "xl"]
const allContexts = ["deep-work", "quick", "research", "write", "review"]

const priorityLabels: Record<ActionPriority, string> = {
  p1: "High",
  p2: "Med",
  p3: "Low",
}

// statusLabels keeps the dropdown human-friendly. Tailwind class strings
// for status colors stay literal in callsites so the JIT extractor picks
// them up — see the equivalent comment in the previous version of this
// file.
const priorityColors: Record<string, string> = {
  p1: "bg-destructive/20 text-destructive",
  p2: "bg-secondary text-secondary-foreground",
  p3: "bg-muted text-muted-foreground",
}

function parseListParam(raw: string | null): ListSelection {
  if (!raw || raw === "my-day") return { kind: "smart", id: "my-day" }
  if (raw === "important" || raw === "planned" || raw === "all") {
    return { kind: "smart", id: raw }
  }
  if (raw.startsWith("p:")) return { kind: "project", projectId: raw.slice(2) }
  return { kind: "smart", id: "my-day" }
}

function encodeListParam(sel: ListSelection): string {
  return sel.kind === "smart" ? sel.id : `p:${sel.projectId}`
}

// listLabel produces the heading shown above the right pane and in
// breadcrumbs. Project names come from the loaded projects list; missing
// project (deleted/renamed) falls back to "Unknown list".
function listLabel(sel: ListSelection, projects: Project[]): string {
  if (sel.kind === "smart") {
    return {
      "my-day": "My Day",
      important: "Important",
      planned: "Planned",
      all: "Tasks",
    }[sel.id]
  }
  return projects.find((p) => p.uuid === sel.projectId)?.name ?? "Unknown list"
}

export function ActionsPage() {
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()

  const selection = parseListParam(searchParams.get("list"))
  const setSelection = (sel: ListSelection) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        next.set("list", encodeListParam(sel))
        // Drop legacy filter params when the user chooses a list — those
        // came from old dashboard deep links and would otherwise stick
        // around invisibly and re-narrow the new list.
        next.delete("status")
        next.delete("project")
        next.delete("stalled")
        return next
      },
      { replace: true },
    )
  }

  // Legacy dashboard deep link compat: ?status= or ?stalled= sent users
  // here from the dashboard cards. We honor them by routing to the All
  // list and showing the user a chip for the active filter.
  const legacyStatus = searchParams.get("status")
  const legacyStalled = searchParams.get("stalled") === "true"

  const { data: actions, isLoading } = useQuery({
    queryKey: ["actions"],
    queryFn: () => api.listActions(),
  })
  const { data: projects } = useQuery({
    queryKey: ["projects"],
    queryFn: api.listProjects,
  })

  // openActionId drives the slide-in detail Sheet. We resolve to the live
  // Action object inside the panel so reconcile / mutation refreshes
  // surface immediately without a manual reopen.
  const [openActionId, setOpenActionId] = useState<string | null>(null)
  const openAction = useMemo(
    () => actions?.find((a) => a.id === openActionId) ?? null,
    [actions, openActionId],
  )
  const [showCompleted, setShowCompleted] = useState(false)

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["actions"] })
    queryClient.invalidateQueries({ queryKey: ["action-counts"] })
  }

  // Subtasks (parent_id set) only render inside the detail panel — never
  // in the main list. This matches MS To Do where Steps are scoped to
  // their parent task.
  const topLevel = useMemo(
    () => (actions ?? []).filter((a) => !a.parent_id),
    [actions],
  )

  const filtered = useMemo(
    () =>
      topLevel.filter((a) => {
        if (a.status === "cancelled") return false
        // Apply legacy filters (only when present in the URL).
        if (legacyStatus && a.status !== legacyStatus) return false
        if (legacyStalled) {
          if (a.status !== "doing" && a.status !== "waiting") return false
          const updatedMs = new Date(a.updated).getTime()
          if (
            isFinite(updatedMs) &&
            Date.now() - updatedMs < 7 * 24 * 60 * 60 * 1000
          )
            return false
        }
        if (selection.kind === "smart") {
          if (selection.id === "my-day") return a.my_day === true
          if (selection.id === "important") return a.starred === true
          if (selection.id === "planned") return Boolean(a.deadline)
          // "all" → just non-cancelled
          return true
        }
        return (a.project_ids ?? []).includes(selection.projectId)
      }),
    [topLevel, selection, legacyStatus, legacyStalled],
  )

  const open = filtered.filter((a) => a.status !== "done")
  const completed = filtered.filter((a) => a.status === "done")

  // Counts shown next to each list label. Always reflect "open work the
  // user could pick up" — done/cancelled excluded, so the badge matches
  // the "what's left" intuition.
  const counts = useMemo(() => {
    const isOpen = (a: Action) =>
      a.status !== "done" && a.status !== "cancelled"
    const perProject: Record<string, number> = {}
    for (const a of topLevel) {
      if (!isOpen(a)) continue
      for (const pid of a.project_ids ?? []) {
        perProject[pid] = (perProject[pid] ?? 0) + 1
      }
    }
    return {
      myDay: topLevel.filter((a) => a.my_day && isOpen(a)).length,
      important: topLevel.filter((a) => a.starred && isOpen(a)).length,
      planned: topLevel.filter((a) => a.deadline && isOpen(a)).length,
      all: topLevel.filter(isOpen).length,
      perProject,
    }
  }, [topLevel])

  // Sort projects alphabetically; keep only ones that have at least one
  // open action so the rail doesn't grow unbounded as projects accumulate.
  // Active selection still renders even if its count is zero — losing
  // your current selection because its last task got completed would be
  // disorienting.
  const visibleProjects = useMemo(() => {
    const list = (projects ?? []).filter(
      (p) =>
        (counts.perProject[p.uuid] ?? 0) > 0 ||
        (selection.kind === "project" && selection.projectId === p.uuid),
    )
    return list.sort((a, b) => a.name.localeCompare(b.name))
  }, [projects, counts.perProject, selection])

  // --- Mutations ---

  const createMutation = useMutation({
    mutationFn: api.createAction,
    onSuccess: () => invalidate(),
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: ActionStatus }) =>
      api.setActionStatus(id, status),
    onSuccess: () => invalidate(),
    onError: (err, vars) => {
      if (
        err instanceof ApiError &&
        err.status === 409 &&
        vars?.status === "doing"
      ) {
        toast.error(
          `WIP limit reached — finish or defer one "doing" task first.`,
        )
        return
      }
      toast.error(getErrorMessage(err))
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: Partial<Action> }) =>
      api.updateAction(id, patch),
    onSuccess: () => invalidate(),
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const reconcileMutation = useMutation({
    mutationFn: api.reconcileActions,
    onSuccess: (result) => {
      invalidate()
      if (result.updated > 0) {
        toast.success(
          `Synced ${result.unique_actions} actions across ${result.scanned} files — ${result.updated} updated`,
        )
      } else {
        toast.info(
          `${result.unique_actions} actions across ${result.scanned} files — all in sync`,
        )
      }
    },
    onError: (err) => toast.error(`Reconcile failed: ${getErrorMessage(err)}`),
  })

  const toggleDone = (a: Action) => {
    statusMutation.mutate({
      id: a.id,
      status: a.status === "done" ? "next" : "done",
    })
  }

  const toggleStar = (a: Action) => {
    updateMutation.mutate({ id: a.id, patch: { starred: !a.starred } })
  }

  const toggleMyDay = (a: Action) => {
    updateMutation.mutate({ id: a.id, patch: { my_day: !a.my_day } })
  }

  const handleQuickAdd = (description: string) => {
    if (!description.trim()) return
    // Quick-add inherits the active list's identity:
    //   • My Day  → my_day=true
    //   • Important → starred=true
    //   • Project list → project_ids=[<uuid>]
    //   • Planned / All → no extra fields
    const body: Parameters<typeof api.createAction>[0] = {
      description: description.trim(),
    }
    if (selection.kind === "smart") {
      if (selection.id === "my-day") body.my_day = true
      if (selection.id === "important") body.starred = true
    } else {
      body.project_ids = [selection.projectId]
    }
    createMutation.mutate(body)
  }

  const breadcrumbLabel = listLabel(selection, projects ?? [])
  const hasLegacyFilters = legacyStatus || legacyStalled

  return (
    <>
      <Breadcrumbs
        items={[
          { label: "Dashboard", to: "/dashboard" },
          { label: "Actions", to: "/actions" },
          { label: breadcrumbLabel },
        ]}
      />
      <div className="flex h-[calc(100vh-160px)] gap-4 -mt-2">
        {/* Left rail: smart lists + project lists */}
        <ActionsListsRail
          selection={selection}
          setSelection={setSelection}
          counts={counts}
          projects={visibleProjects}
          onReconcile={() => reconcileMutation.mutate()}
          reconciling={reconcileMutation.isPending}
        />

        {/* Right pane: list contents */}
        <div className="flex-1 flex flex-col min-w-0">
          <div className="flex items-baseline justify-between mb-3">
            <h1 className="text-[22px] font-light tracking-[-0.22px]">
              {breadcrumbLabel}
            </h1>
            <span className="text-xs text-muted-foreground tabular-nums">
              {open.length} open · {completed.length} done
            </span>
          </div>

          {hasLegacyFilters && (
            <div className="flex items-center gap-2 mb-3">
              <span className="text-xs text-muted-foreground">
                Filtered by:
              </span>
              {legacyStatus && (
                <Badge className="text-[10px] font-light rounded-sm">
                  status: {legacyStatus}
                </Badge>
              )}
              {legacyStalled && (
                <Badge className="text-[10px] font-light rounded-sm bg-[var(--stripe-lemon)]/20 text-[var(--stripe-lemon)]">
                  stalled &gt; 7 days
                </Badge>
              )}
              <button
                onClick={() => {
                  setSearchParams(
                    (prev) => {
                      const next = new URLSearchParams(prev)
                      next.delete("status")
                      next.delete("stalled")
                      return next
                    },
                    { replace: true },
                  )
                }}
                className="text-xs text-primary hover:underline"
              >
                Clear
              </button>
            </div>
          )}

          <QuickAddBar onAdd={handleQuickAdd} disabled={createMutation.isPending} />

          <div className="flex-1 overflow-auto mt-2">
            {isLoading ? (
              <p className="text-muted-foreground text-sm py-6">Loading…</p>
            ) : open.length === 0 && completed.length === 0 ? (
              <EmptyState selection={selection} />
            ) : (
              <>
                <ul className="divide-y divide-border/60">
                  {open.map((a) => (
                    <ActionRow
                      key={a.id}
                      action={a}
                      stepProgress={stepProgressFor(a.id, actions)}
                      projects={projects ?? []}
                      onOpen={() => setOpenActionId(a.id)}
                      onToggleDone={() => toggleDone(a)}
                      onToggleStar={() => toggleStar(a)}
                    />
                  ))}
                </ul>
                {completed.length > 0 && (
                  <CompletedSection
                    items={completed}
                    expanded={showCompleted}
                    onToggle={() => setShowCompleted((s) => !s)}
                    actions={actions ?? []}
                    projects={projects ?? []}
                    onOpen={(id) => setOpenActionId(id)}
                    onToggleDone={(a) => toggleDone(a)}
                    onToggleStar={(a) => toggleStar(a)}
                  />
                )}
              </>
            )}
          </div>
        </div>
      </div>

      {openAction && (
        <ActionDetailSheet
          action={openAction}
          allActions={actions ?? []}
          projects={projects ?? []}
          onClose={() => setOpenActionId(null)}
          onToggleMyDay={() => toggleMyDay(openAction)}
          onToggleStar={() => toggleStar(openAction)}
          onToggleDone={() => toggleDone(openAction)}
          onSave={(patch) =>
            updateMutation.mutate({ id: openAction.id, patch })
          }
          onCreateStep={(description) =>
            createMutation.mutate({
              description,
              parent_id: openAction.id,
            })
          }
        />
      )}
    </>
  )
}

// ---------------- Left rail ----------------

interface RailProps {
  selection: ListSelection
  setSelection: (sel: ListSelection) => void
  counts: {
    myDay: number
    important: number
    planned: number
    all: number
    perProject: Record<string, number>
  }
  projects: Project[]
  onReconcile: () => void
  reconciling: boolean
}

function ActionsListsRail({
  selection,
  setSelection,
  counts,
  projects,
  onReconcile,
  reconciling,
}: RailProps) {
  const isActive = (sel: ListSelection): boolean => {
    if (selection.kind !== sel.kind) return false
    if (selection.kind === "smart" && sel.kind === "smart")
      return selection.id === sel.id
    if (selection.kind === "project" && sel.kind === "project")
      return selection.projectId === sel.projectId
    return false
  }

  return (
    <aside className="w-[200px] shrink-0 flex flex-col gap-1 border-r border-border pr-3">
      <RailRow
        icon={<Sun className="h-3.5 w-3.5" />}
        label="My Day"
        count={counts.myDay}
        active={isActive({ kind: "smart", id: "my-day" })}
        onClick={() => setSelection({ kind: "smart", id: "my-day" })}
      />
      <RailRow
        icon={<Star className="h-3.5 w-3.5" />}
        label="Important"
        count={counts.important}
        active={isActive({ kind: "smart", id: "important" })}
        onClick={() => setSelection({ kind: "smart", id: "important" })}
      />
      <RailRow
        icon={<CalendarDays className="h-3.5 w-3.5" />}
        label="Planned"
        count={counts.planned}
        active={isActive({ kind: "smart", id: "planned" })}
        onClick={() => setSelection({ kind: "smart", id: "planned" })}
      />
      <RailRow
        icon={<ListChecks className="h-3.5 w-3.5" />}
        label="Tasks"
        count={counts.all}
        active={isActive({ kind: "smart", id: "all" })}
        onClick={() => setSelection({ kind: "smart", id: "all" })}
      />

      {projects.length > 0 && (
        <>
          <div className="mt-3 mb-1 px-2 text-[10px] uppercase tracking-wider text-muted-foreground">
            Lists
          </div>
          {projects.map((p) => (
            <RailRow
              key={p.uuid}
              icon={<Folder className="h-3.5 w-3.5" />}
              label={p.name}
              count={counts.perProject[p.uuid] ?? 0}
              active={isActive({ kind: "project", projectId: p.uuid })}
              onClick={() =>
                setSelection({ kind: "project", projectId: p.uuid })
              }
            />
          ))}
        </>
      )}

      <div className="flex-1" />
      <Button
        variant="ghost"
        size="sm"
        onClick={onReconcile}
        disabled={reconciling}
        className="justify-start text-xs font-normal text-muted-foreground hover:text-foreground rounded-sm"
      >
        <RefreshCw
          className={`h-3.5 w-3.5 mr-2 ${reconciling ? "animate-spin" : ""}`}
        />
        Reconcile vault
      </Button>
    </aside>
  )
}

function RailRow({
  icon,
  label,
  count,
  active,
  onClick,
}: {
  icon: React.ReactNode
  label: string
  count: number
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={`group flex items-center gap-2 px-2 py-1.5 rounded-sm text-sm font-normal text-left transition-colors ${
        active
          ? "bg-accent text-foreground"
          : "text-muted-foreground hover:bg-accent/60 hover:text-foreground"
      }`}
    >
      <span className="text-muted-foreground group-hover:text-foreground">
        {icon}
      </span>
      <span className="flex-1 truncate">{label}</span>
      {count > 0 && (
        <span className="text-[10px] tabular-nums text-muted-foreground">
          {count}
        </span>
      )}
    </button>
  )
}

// ---------------- Quick-add ----------------

function QuickAddBar({
  onAdd,
  disabled,
}: {
  onAdd: (description: string) => void
  disabled: boolean
}) {
  const [value, setValue] = useState("")
  const inputRef = useRef<HTMLInputElement>(null)

  const submit = () => {
    if (!value.trim()) return
    onAdd(value)
    setValue("")
    // Keep focus so user can rapid-fire several tasks Enter-to-submit
    inputRef.current?.focus()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault()
      submit()
    }
  }

  return (
    <div className="flex items-center gap-2 rounded-sm border border-border bg-card/50 px-3 py-2 focus-within:border-primary/50 focus-within:bg-card transition-colors">
      <Plus className="h-4 w-4 text-muted-foreground" />
      <input
        ref={inputRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder="Add a task"
        disabled={disabled}
        className="flex-1 bg-transparent text-sm font-normal placeholder:text-muted-foreground focus:outline-none disabled:opacity-50"
      />
      {value.trim() && (
        <button
          onClick={submit}
          disabled={disabled}
          className="text-xs text-primary hover:underline disabled:opacity-50"
        >
          Add
        </button>
      )}
    </div>
  )
}

// ---------------- Row ----------------

interface RowProps {
  action: Action
  stepProgress: { done: number; total: number } | null
  projects: Project[]
  onOpen: () => void
  onToggleDone: () => void
  onToggleStar: () => void
}

function ActionRow({
  action,
  stepProgress,
  projects,
  onOpen,
  onToggleDone,
  onToggleStar,
}: RowProps) {
  const isDone = action.status === "done"
  const titleText = action.title || action.description
  const subText = action.title ? action.description : ""
  const projectNames = (action.project_ids ?? [])
    .map((pid) => projects.find((p) => p.uuid === pid)?.name)
    .filter(Boolean) as string[]

  return (
    <li
      onClick={onOpen}
      className="group flex items-start gap-3 px-2 py-2.5 cursor-pointer hover:bg-accent/40 rounded-sm"
    >
      <button
        onClick={(e) => {
          e.stopPropagation()
          onToggleDone()
        }}
        className="mt-0.5 text-muted-foreground hover:text-primary transition-colors"
        aria-label={isDone ? "Mark not done" : "Mark done"}
      >
        {isDone ? (
          <CheckCircle2 className="h-4 w-4 text-primary" />
        ) : (
          <Circle className="h-4 w-4" />
        )}
      </button>

      <div className="flex-1 min-w-0">
        <div
          className={`text-sm leading-tight ${
            isDone ? "text-muted-foreground line-through" : "text-foreground"
          }`}
        >
          {titleText}
        </div>
        {subText && !isDone && (
          <div className="text-xs text-muted-foreground line-clamp-1 mt-0.5">
            {subText}
          </div>
        )}
        <RowMetadata
          action={action}
          stepProgress={stepProgress}
          projectNames={projectNames}
        />
      </div>

      <button
        onClick={(e) => {
          e.stopPropagation()
          onToggleStar()
        }}
        className={`mt-0.5 transition-colors ${
          action.starred
            ? "text-[var(--stripe-lemon)] hover:text-[var(--stripe-lemon)]/80"
            : "text-muted-foreground/40 hover:text-[var(--stripe-lemon)] opacity-0 group-hover:opacity-100"
        } ${action.starred ? "opacity-100" : ""}`}
        aria-label={action.starred ? "Unstar" : "Star"}
      >
        <Star
          className={`h-4 w-4 ${action.starred ? "fill-current" : ""}`}
        />
      </button>
    </li>
  )
}

function RowMetadata({
  action,
  stepProgress,
  projectNames,
}: {
  action: Action
  stepProgress: { done: number; total: number } | null
  projectNames: string[]
}) {
  const items: React.ReactNode[] = []

  if (action.my_day) {
    items.push(
      <span
        key="myday"
        className="inline-flex items-center gap-1 text-[10px] text-[var(--stripe-lemon)]"
      >
        <Sun className="h-3 w-3" /> My Day
      </span>,
    )
  }
  if (action.deadline) {
    const overdue =
      action.status !== "done" &&
      new Date(action.deadline).getTime() < Date.now() - 24 * 60 * 60 * 1000
    items.push(
      <span
        key="due"
        className={`inline-flex items-center gap-1 text-[10px] tabular-nums ${
          overdue ? "text-destructive" : "text-muted-foreground"
        }`}
      >
        <CalendarDays className="h-3 w-3" />
        {action.deadline}
      </span>,
    )
  }
  if (stepProgress && stepProgress.total > 0) {
    items.push(
      <span
        key="steps"
        className="inline-flex items-center gap-1 text-[10px] text-muted-foreground tabular-nums"
      >
        <ListChecks className="h-3 w-3" />
        {stepProgress.done}/{stepProgress.total}
      </span>,
    )
  }
  if (action.description && action.title) {
    items.push(
      <span key="note" className="text-muted-foreground">
        <StickyNote className="h-3 w-3 inline" />
      </span>,
    )
  }
  if (action.priority && action.priority !== "p2") {
    items.push(
      <Badge
        key="pri"
        className={`text-[9px] font-light rounded-sm px-1 py-0 ${
          priorityColors[action.priority] ?? ""
        }`}
      >
        {priorityLabels[action.priority] ?? action.priority}
      </Badge>,
    )
  }
  if (action.context) {
    items.push(
      <span key="ctx" className="text-[10px] text-muted-foreground">
        @{action.context}
      </span>,
    )
  }
  for (const name of projectNames) {
    items.push(
      <span
        key={`p:${name}`}
        className="inline-flex items-center gap-1 text-[10px] text-muted-foreground"
      >
        <Folder className="h-3 w-3" />
        {name}
      </span>,
    )
  }
  if (action.status !== "done" && action.status !== "next") {
    items.push(
      <span
        key="status"
        className="text-[10px] text-muted-foreground uppercase"
      >
        {action.status}
      </span>,
    )
  }

  if (items.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1">
      {items}
    </div>
  )
}

// ---------------- Completed section ----------------

function CompletedSection({
  items,
  expanded,
  onToggle,
  actions,
  projects,
  onOpen,
  onToggleDone,
  onToggleStar,
}: {
  items: Action[]
  expanded: boolean
  onToggle: () => void
  actions: Action[]
  projects: Project[]
  onOpen: (id: string) => void
  onToggleDone: (a: Action) => void
  onToggleStar: (a: Action) => void
}) {
  return (
    <div className="mt-4 pt-3 border-t border-border/60">
      <button
        onClick={onToggle}
        className="flex items-center gap-2 text-xs text-muted-foreground hover:text-foreground"
      >
        {expanded ? (
          <ChevronDown className="h-3.5 w-3.5" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5" />
        )}
        Completed ({items.length})
      </button>
      {expanded && (
        <ul className="divide-y divide-border/60 mt-2">
          {items.map((a) => (
            <ActionRow
              key={a.id}
              action={a}
              stepProgress={stepProgressFor(a.id, actions)}
              projects={projects}
              onOpen={() => onOpen(a.id)}
              onToggleDone={() => onToggleDone(a)}
              onToggleStar={() => onToggleStar(a)}
            />
          ))}
        </ul>
      )}
    </div>
  )
}

function stepProgressFor(
  parentId: string,
  actions: Action[] | undefined,
): { done: number; total: number } | null {
  if (!actions) return null
  const children = actions.filter((c) => c.parent_id === parentId)
  if (children.length === 0) return null
  return {
    total: children.length,
    done: children.filter((c) => c.status === "done").length,
  }
}

// ---------------- Empty state ----------------

function EmptyState({ selection }: { selection: ListSelection }) {
  const message =
    selection.kind === "smart"
      ? {
          "my-day": "Nothing in My Day yet. Add tasks here to focus on today.",
          important: "No starred tasks. Click a star to flag what matters.",
          planned: "No tasks have a due date.",
          all: "No tasks tracked yet. Type one above to get started.",
        }[selection.id]
      : "This list is empty."
  return (
    <div className="flex flex-col items-center justify-center py-12 text-center">
      <CircleDashed className="h-8 w-8 text-muted-foreground/40 mb-3" />
      <p className="text-sm text-muted-foreground max-w-[280px]">{message}</p>
    </div>
  )
}

// ---------------- Detail sheet ----------------

interface DetailProps {
  action: Action
  allActions: Action[]
  projects: Project[]
  onClose: () => void
  onToggleMyDay: () => void
  onToggleStar: () => void
  onToggleDone: () => void
  onSave: (patch: Partial<Action>) => void
  onCreateStep: (description: string) => void
}

function ActionDetailSheet({
  action,
  allActions,
  projects,
  onClose,
  onToggleMyDay,
  onToggleStar,
  onToggleDone,
  onSave,
  onCreateStep,
}: DetailProps) {
  // Local form state seeded from the action; re-seed when a different
  // action opens. We intentionally save on blur or explicit Save click
  // rather than per-keystroke to avoid a write-amplification storm
  // through the markdown sync layer.
  const [title, setTitle] = useState(action.title ?? "")
  const [description, setDescription] = useState(action.description)
  const [owner, setOwner] = useState(action.owner)
  const [deadline, setDeadline] = useState(action.deadline ?? "")
  const [status, setStatus] = useState<ActionStatus>(action.status)
  const [priority, setPriority] = useState<ActionPriority>(
    (action.priority as ActionPriority) ?? "p2",
  )
  const [effort, setEffort] = useState<ActionEffort>(
    (action.effort as ActionEffort) ?? "m",
  )
  const [context, setContext] = useState(action.context ?? "")

  useEffect(() => {
    setTitle(action.title ?? "")
    setDescription(action.description)
    setOwner(action.owner)
    setDeadline(action.deadline ?? "")
    setStatus(action.status)
    setPriority((action.priority as ActionPriority) ?? "p2")
    setEffort((action.effort as ActionEffort) ?? "m")
    setContext(action.context ?? "")
  }, [action.id])

  const buildPatch = (): Partial<Action> => {
    const patch: Partial<Action> = {}
    if (title !== (action.title ?? "")) patch.title = title
    if (description !== action.description) patch.description = description
    if (owner !== action.owner) patch.owner = owner
    if (deadline !== (action.deadline ?? "")) patch.deadline = deadline
    if (status !== action.status) patch.status = status
    if (priority !== (action.priority ?? "p2")) patch.priority = priority
    if (effort !== (action.effort ?? "m")) patch.effort = effort
    if (context !== (action.context ?? "")) patch.context = context
    return patch
  }

  const patch = buildPatch()
  const hasChanges = Object.keys(patch).length > 0

  const steps = useMemo(
    () => allActions.filter((a) => a.parent_id === action.id),
    [allActions, action.id],
  )
  const projectNames = (action.project_ids ?? [])
    .map((pid) => projects.find((p) => p.uuid === pid)?.name ?? pid)
    .join(", ")

  const [stepInput, setStepInput] = useState("")

  const handleSave = () => {
    if (hasChanges) onSave(patch)
  }

  return (
    <Sheet open onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-[420px] sm:max-w-[420px] overflow-y-auto">
        <SheetHeader>
          <SheetTitle className="sr-only">Task details</SheetTitle>
        </SheetHeader>

        {/* Title row with done-circle + star */}
        <div className="flex items-start gap-3 px-2 pt-1">
          <button
            onClick={onToggleDone}
            className="mt-1 text-muted-foreground hover:text-primary transition-colors shrink-0"
            aria-label={
              action.status === "done" ? "Mark not done" : "Mark done"
            }
          >
            {action.status === "done" ? (
              <CheckCircle2 className="h-4 w-4 text-primary" />
            ) : (
              <Circle className="h-4 w-4" />
            )}
          </button>
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onBlur={handleSave}
            placeholder="Task title (optional)"
            className={`flex-1 border-0 shadow-none px-1 text-base font-normal h-auto ${
              action.status === "done" ? "line-through text-muted-foreground" : ""
            } focus-visible:ring-1`}
          />
          <button
            onClick={onToggleStar}
            className={`mt-1 transition-colors shrink-0 ${
              action.starred
                ? "text-[var(--stripe-lemon)]"
                : "text-muted-foreground/60 hover:text-[var(--stripe-lemon)]"
            }`}
            aria-label={action.starred ? "Unstar" : "Star"}
          >
            <Star
              className={`h-4 w-4 ${action.starred ? "fill-current" : ""}`}
            />
          </button>
        </div>

        {/* Steps */}
        <div className="mt-4 space-y-1 px-2">
          {steps.map((step) => (
            <StepRow key={step.id} step={step} />
          ))}
          <StepQuickAdd
            value={stepInput}
            onChange={setStepInput}
            onSubmit={() => {
              const v = stepInput.trim()
              if (!v) return
              onCreateStep(v)
              setStepInput("")
            }}
          />
        </div>

        {/* Quick toggles */}
        <div className="mt-4 px-2 grid grid-cols-1 gap-1">
          <ToggleRow
            icon={<Sun className="h-4 w-4" />}
            label={action.my_day ? "Added to My Day" : "Add to My Day"}
            active={!!action.my_day}
            onClick={onToggleMyDay}
          />
        </div>

        {/* Fields */}
        <div className="mt-4 px-2 space-y-3">
          <div className="grid grid-cols-3 gap-3">
            <FieldSelect
              label="Status"
              value={status}
              onChange={(v) => {
                setStatus(v as ActionStatus)
                onSave({ ...patch, status: v as ActionStatus })
              }}
              options={allStatuses.map((s) => ({ value: s, label: s }))}
            />
            <FieldSelect
              label="Priority"
              value={priority}
              onChange={(v) => {
                setPriority(v as ActionPriority)
                onSave({ ...patch, priority: v as ActionPriority })
              }}
              options={allPriorities.map((p) => ({
                value: p,
                label: priorityLabels[p],
              }))}
            />
            <FieldSelect
              label="Effort"
              value={effort}
              onChange={(v) => {
                setEffort(v as ActionEffort)
                onSave({ ...patch, effort: v as ActionEffort })
              }}
              options={allEfforts.map((e) => ({
                value: e,
                label: e.toUpperCase(),
              }))}
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label className="text-[var(--stripe-label)] text-xs font-normal">
                Owner
              </Label>
              <Input
                value={owner}
                onChange={(e) => setOwner(e.target.value)}
                onBlur={handleSave}
                className="border-border rounded-sm h-8 text-sm"
              />
            </div>
            <div className="space-y-1">
              <Label className="text-[var(--stripe-label)] text-xs font-normal">
                Due
              </Label>
              <Input
                type="date"
                value={deadline}
                onChange={(e) => setDeadline(e.target.value)}
                onBlur={handleSave}
                className="border-border rounded-sm h-8 text-sm"
              />
            </div>
          </div>

          <FieldSelect
            label="Context"
            value={context || "_none"}
            onChange={(v) => {
              const next = v === "_none" ? "" : v
              setContext(next)
              onSave({ ...patch, context: next })
            }}
            options={[
              { value: "_none", label: "None" },
              ...allContexts.map((c) => ({ value: c, label: `@${c}` })),
            ]}
          />

          {projectNames && (
            <div className="space-y-1">
              <Label className="text-[var(--stripe-label)] text-xs font-normal">
                Lists
              </Label>
              <div className="text-sm font-light text-foreground">
                {projectNames}
              </div>
            </div>
          )}

          <div className="space-y-1">
            <Label className="text-[var(--stripe-label)] text-xs font-normal">
              Note
            </Label>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              onBlur={handleSave}
              rows={5}
              placeholder="Add detail…"
              className="border-border rounded-sm text-sm resize-none"
            />
          </div>
        </div>

        {/* Footer */}
        <div className="mt-6 flex items-center justify-between border-t border-border pt-3 px-2">
          <div className="text-[10px] text-muted-foreground font-mono leading-tight">
            <div>id: {action.id.slice(0, 8)}</div>
            <div>created: {new Date(action.created).toLocaleDateString()}</div>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={onClose}
            className="text-xs font-normal"
          >
            <X className="h-3.5 w-3.5 mr-1" />
            Close
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  )
}

function ToggleRow({
  icon,
  label,
  active,
  onClick,
}: {
  icon: React.ReactNode
  label: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-2 px-2 py-2 rounded-sm text-sm font-normal text-left transition-colors ${
        active
          ? "bg-[var(--stripe-lemon)]/15 text-[var(--stripe-lemon)]"
          : "text-muted-foreground hover:bg-accent hover:text-foreground"
      }`}
    >
      {icon}
      <span className="flex-1">{label}</span>
      {active && <CheckCircle2 className="h-3.5 w-3.5" />}
    </button>
  )
}

function FieldSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <div className="space-y-1">
      <Label className="text-[var(--stripe-label)] text-xs font-normal">
        {label}
      </Label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger className="border-border rounded-sm h-8 text-sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

// ---------------- Step (subtask) row ----------------

function StepRow({ step }: { step: Action }) {
  const queryClient = useQueryClient()
  const toggleStep = useMutation({
    mutationFn: (s: ActionStatus) => api.setActionStatus(step.id, s),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["actions"] })
    },
  })
  const isDone = step.status === "done"
  return (
    <div className="flex items-center gap-2 py-1 px-1">
      <button
        onClick={() => toggleStep.mutate(isDone ? "next" : "done")}
        className="text-muted-foreground hover:text-primary transition-colors shrink-0"
      >
        {isDone ? (
          <CheckCircle2 className="h-3.5 w-3.5 text-primary" />
        ) : (
          <Circle className="h-3.5 w-3.5" />
        )}
      </button>
      <span
        className={`text-sm flex-1 ${
          isDone ? "text-muted-foreground line-through" : "text-foreground"
        }`}
      >
        {step.title || step.description}
      </span>
    </div>
  )
}

function StepQuickAdd({
  value,
  onChange,
  onSubmit,
}: {
  value: string
  onChange: (v: string) => void
  onSubmit: () => void
}) {
  return (
    <div className="flex items-center gap-2 py-1 px-1">
      <Plus className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault()
            onSubmit()
          }
        }}
        placeholder="Add step"
        className="flex-1 bg-transparent text-sm font-normal placeholder:text-muted-foreground focus:outline-none"
      />
    </div>
  )
}
