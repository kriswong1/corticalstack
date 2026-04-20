import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate } from "react-router-dom"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { PageHeader } from "@/components/layout/page-header"
import { SkeletonPage } from "@/components/shared/skeleton-card"
import { api } from "@/lib/api"
import { routeFor } from "@/lib/pipeline-stages"
import type {
  ActionsWidget,
  DashboardSnapshot,
  IngestDay,
  IngestWidget,
  PipelineWidget,
  ProjectsWidget,
} from "@/types/api"
import {
  AlertTriangle,
  BookOpen,
  Clock,
  Settings,
} from "lucide-react"

// --- Color palettes ---

// Ingest chart bucket colors (OKLCH)
const ingestColors: Record<string, string> = {
  articles:    "oklch(0.65 0.18 270)", // indigo
  youtube:     "oklch(0.60 0.22 25)",  // red (YouTube brand)
  transcripts: "oklch(0.70 0.15 55)",  // orange
  documents:   "oklch(0.68 0.16 175)", // teal
  notes:       "oklch(0.75 0.16 85)",  // amber
  other:       "oklch(0.65 0.06 260)", // muted blue-grey
}

// Pipeline card stage colors
const stageColors: Record<string, Record<string, string>> = {
  product: {
    idea:       "oklch(0.55 0.04 250)", // slate
    frame:      "oklch(0.60 0.14 200)", // teal
    shape:      "oklch(0.55 0.22 275)", // indigo
    breadboard: "oklch(0.60 0.20 325)", // magenta
    pitch:      "oklch(0.62 0.19 150)", // emerald
  },
  meeting: {
    transcript: "oklch(0.65 0.15 230)", // sky
    note:       "oklch(0.62 0.19 150)", // emerald
  },
  document: {
    input: "oklch(0.55 0.04 250)", // slate
    note:  "oklch(0.62 0.19 150)", // emerald
  },
  prototype: {
    breadboard:  "oklch(0.60 0.20 325)", // magenta
    in_progress: "oklch(0.70 0.16 85)",  // amber
    final:       "oklch(0.62 0.19 150)", // emerald
  },
}

const fallbackColors = [
  "oklch(0.58 0.22 5)",
  "oklch(0.55 0.18 90)",
  "oklch(0.58 0.16 180)",
  "oklch(0.55 0.18 240)",
  "oklch(0.60 0.20 325)",
]

function colorFor(key: string): string {
  if (ingestColors[key]) return ingestColors[key]
  let h = 0
  for (let i = 0; i < key.length; i++) {
    h = (h * 31 + key.charCodeAt(i)) | 0
  }
  return fallbackColors[Math.abs(h) % fallbackColors.length]
}

function stageColorFor(cardType: string, stage: string): string {
  return stageColors[cardType]?.[stage] ?? colorFor(stage)
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}

function relativeTime(iso: string): string {
  const then = new Date(iso).getTime()
  const now = Date.now()
  const diffMs = now - then
  if (diffMs < 60_000) return "just now"
  if (diffMs < 3_600_000) return `${Math.floor(diffMs / 60_000)}m ago`
  if (diffMs < 86_400_000) return `${Math.floor(diffMs / 3_600_000)}h ago`
  if (diffMs < 172_800_000) return "yesterday"
  const days = Math.floor(diffMs / 86_400_000)
  return `${days}d ago`
}

function shortDate(yyyyMmDd: string): string {
  if (!yyyyMmDd) return ""
  const [y, m, d] = yyyyMmDd.split("-").map(Number)
  if (!y || !m || !d) return yyyyMmDd
  return new Date(y, m - 1, d).toLocaleDateString([], {
    month: "short",
    day: "numeric",
  })
}

// --- Main page ---

export function DashboardPage() {
  const { data: snapshot, isLoading, error } = useQuery({
    queryKey: ["dashboard"],
    queryFn: api.getDashboard,
    staleTime: 60_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
  })

  if (isLoading) return <SkeletonPage />

  if (error || !snapshot) {
    return (
      <>
        <PageHeader title="Dashboard" description="Operating view" />
        <Card className="rounded-md border-destructive/40 bg-destructive/5">
          <CardContent className="py-6">
            <p className="text-sm text-destructive">
              Could not load dashboard snapshot. The backend may still be
              starting up — refresh in a moment.
            </p>
          </CardContent>
        </Card>
      </>
    )
  }

  if (snapshot.all_empty) {
    return <DashboardOnboarding />
  }

  return (
    <>
      <div className="flex items-start justify-between mb-6">
        <PageHeader
          title="Dashboard"
          description="What came in, what's stuck, where work is concentrated"
        />
        <FreshnessMarker snapshot={snapshot} />
      </div>

      {snapshot.stale && <StaleBanner snapshot={snapshot} />}

      {/* Row 1: full-width ingest chart */}
      <div className="mb-5">
        <IngestChart data={snapshot.ingest_activity} />
      </div>

      {/* Row 2: pipeline cards */}
      {snapshot.pipelines && (
        <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-4 mb-5">
          <PipelineCard
            title="Product"
            type="product"
            widget={snapshot.pipelines.product}
          />
          <PipelineCard
            title="Meetings"
            type="meeting"
            widget={snapshot.pipelines.meetings}
          />
          <PipelineCard
            title="Documents"
            type="document"
            widget={snapshot.pipelines.documents}
          />
          <PipelineCard
            title="Prototypes"
            type="prototype"
            widget={snapshot.pipelines.prototypes}
          />
        </div>
      )}

      {/* Row 3: actions + projects */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <ActionsStatusCard widget={snapshot.actions} />
        <ActiveProjectsCard widget={snapshot.active_projects} />
      </div>
    </>
  )
}

// --- Freshness marker + stale banner ---

function FreshnessMarker({ snapshot }: { snapshot: DashboardSnapshot }) {
  const stale = snapshot.stale
  const dotColor = stale
    ? "bg-[var(--stripe-lemon)] ring-[var(--stripe-lemon)]/20"
    : "bg-[var(--stripe-success)] ring-[var(--stripe-success)]/20"

  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1.5 text-xs text-muted-foreground whitespace-nowrap select-none cursor-default">
      <span
        className={`h-1.5 w-1.5 rounded-full ring-4 ${dotColor}`}
        aria-hidden
      />
      <span>
        as of{" "}
        <strong className="text-foreground font-semibold tabular-nums">
          {formatTime(snapshot.computed_at)}
        </strong>
      </span>
      {stale && snapshot.stale_attempt_at && (
        <span className="pl-2 ml-1 border-l border-border text-[11px] text-muted-foreground/80">
          retry failed {formatTime(snapshot.stale_attempt_at)}
        </span>
      )}
    </span>
  )
}

function StaleBanner({ snapshot }: { snapshot: DashboardSnapshot }) {
  return (
    <div className="mb-5 flex items-start gap-3 rounded-md border border-[var(--stripe-lemon)]/40 bg-[var(--stripe-lemon)]/10 px-4 py-3">
      <AlertTriangle className="h-4 w-4 mt-0.5 text-[var(--stripe-lemon)] flex-shrink-0" />
      <div className="flex-1 text-xs text-foreground">
        <p className="font-medium">
          Showing cached data. The latest refresh failed and the cache is
          being served.
        </p>
        {snapshot.stale_reason && (
          <p className="mt-1 text-[11px] text-muted-foreground font-mono">
            {snapshot.stale_reason}
          </p>
        )}
      </div>
    </div>
  )
}

// --- Row 1: Ingest Chart (30-day stacked bars) ---

interface TooltipState {
  visible: boolean
  x: number
  y: number
  day: IngestDay | null
}

function IngestChart({ data }: { data: IngestWidget }) {
  const navigate = useNavigate()
  const days = useMemo(() => data.days ?? [], [data.days])
  const types = useMemo(() => data.types ?? [], [data.types])
  const emptyDays = useMemo(
    () => days.filter((d) => d.count === 0).length,
    [days],
  )
  const maxCount = useMemo(
    () => Math.max(1, ...days.map((d) => d.count)),
    [days],
  )
  const typeColors = useMemo(
    () => Object.fromEntries(types.map((t) => [t, colorFor(t)])),
    [types],
  )

  const [tooltip, setTooltip] = useState<TooltipState>({
    visible: false,
    x: 0,
    y: 0,
    day: null,
  })
  const rafRef = useRef<number | null>(null)

  const showTooltip = (day: IngestDay, e: React.MouseEvent) => {
    setTooltip({ visible: true, x: e.clientX, y: e.clientY, day })
  }
  const moveTooltip = (e: React.MouseEvent) => {
    const x = e.clientX
    const y = e.clientY
    if (rafRef.current != null) cancelAnimationFrame(rafRef.current)
    rafRef.current = requestAnimationFrame(() => {
      rafRef.current = null
      setTooltip((t) => (t.visible ? { ...t, x, y } : t))
    })
  }
  const hideTooltip = () => {
    if (rafRef.current != null) {
      cancelAnimationFrame(rafRef.current)
      rafRef.current = null
    }
    setTooltip((t) => ({ ...t, visible: false }))
  }
  useEffect(() => {
    return () => {
      if (rafRef.current != null) cancelAnimationFrame(rafRef.current)
    }
  }, [])

  const isEmpty = data.total === 0

  return (
    <>
      <Card className="rounded-[14px] border-border shadow-stripe hover:shadow-stripe-elevated transition-shadow">
        <CardHeader className="pb-4">
          <div className="flex items-start justify-between gap-4">
            <div>
              <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
                Ingest activity
              </CardTitle>
              <p className="mt-0.5 text-xs text-muted-foreground">
                Last 30 days · stacked by type
              </p>
            </div>
            {types.length > 0 && (
              <div className="inline-flex gap-0.5 rounded-lg border border-border bg-muted/50 p-1">
                {types.map((t) => (
                  <Link
                    key={t}
                    to={`/library?type=${t}`}
                    className="inline-flex items-center gap-1.5 rounded-[7px] px-2.5 py-1 text-[11px] font-medium text-muted-foreground hover:bg-card hover:text-foreground hover:shadow-stripe transition-all"
                  >
                    <span
                      className="inline-block h-2 w-2 rounded-[2px]"
                      style={{ background: colorFor(t) }}
                    />
                    {t}
                  </Link>
                ))}
              </div>
            )}
          </div>
        </CardHeader>
        <CardContent className="pb-5">
          {isEmpty ? (
            <EmptyState
              message="No ingested notes in the last 30 days."
              cta="Go to Library"
              to="/library"
            />
          ) : (
            <>
              <div
                className="flex items-end gap-1 h-[190px] pt-3 pb-2 border-b border-border relative"
                onMouseLeave={hideTooltip}
              >
                {days.map((day) => {
                  const pct = (day.count / maxCount) * 100
                  const totalDay = day.count
                  const buckets = day.buckets ?? []
                  const isEmptyDay = totalDay === 0
                  const outerLabel = `${day.date}: ${totalDay} note${
                    totalDay === 1 ? "" : "s"
                  }`
                  const outerNavigate = () =>
                    navigate(`/library?date=${day.date}`)
                  return (
                    <div
                      key={day.date}
                      role="link"
                      tabIndex={0}
                      onClick={outerNavigate}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault()
                          outerNavigate()
                        }
                      }}
                      className="flex-1 h-full flex flex-col justify-end min-w-0 rounded-t hover:bg-accent/50 transition-colors px-[1px] cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/60"
                      onMouseEnter={(e) => showTooltip(day, e)}
                      onMouseMove={moveTooltip}
                      aria-label={outerLabel}
                    >
                      {isEmptyDay ? (
                        <div
                          className="w-full bg-border rounded-sm opacity-60"
                          style={{ height: "2px" }}
                        />
                      ) : (
                        <div
                          className="w-full flex flex-col-reverse overflow-hidden rounded-t-[3px]"
                          style={{ height: `${Math.max(6, pct)}%` }}
                        >
                          {buckets.map((b) => (
                            <Link
                              key={b.type}
                              to={`/library?date=${day.date}&type=${b.type}`}
                              onClick={(e) => e.stopPropagation()}
                              style={{
                                flex: b.count,
                                background: typeColors[b.type] ?? colorFor(b.type),
                                minHeight: "2px",
                              }}
                              className="w-full block hover:brightness-110 transition-[filter]"
                              aria-label={`${day.date} ${b.type}: ${b.count}`}
                            />
                          ))}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
              <div className="flex justify-between px-[2px] pt-2 text-[11px] text-muted-foreground tabular-nums">
                <span>{shortDate(days[0]?.date ?? "")}</span>
                <span>{shortDate(days[Math.floor(days.length / 2)]?.date ?? "")}</span>
                <span>{shortDate(days[days.length - 1]?.date ?? "")}</span>
              </div>
              <div className="mt-4 pt-4 border-t border-border/60 text-xs text-muted-foreground">
                Total this window:{" "}
                <strong className="text-foreground font-semibold tabular-nums">
                  {data.total}
                </strong>{" "}
                ingests ·{" "}
                <strong className="text-foreground font-semibold tabular-nums">
                  {emptyDays}
                </strong>{" "}
                empty days
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {tooltip.visible && tooltip.day && (
        <ChartTooltip x={tooltip.x} y={tooltip.y} day={tooltip.day} />
      )}
    </>
  )
}

function ChartTooltip({
  x,
  y,
  day,
}: {
  x: number
  y: number
  day: IngestDay
}) {
  const tooltipRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const el = tooltipRef.current
    if (!el) return
    const pad = 14
    const rect = el.getBoundingClientRect()
    const vw = window.innerWidth
    const vh = window.innerHeight
    const leftRaw =
      x + pad + rect.width > vw ? x - rect.width - pad : x + pad
    const topRaw =
      y + pad + rect.height > vh ? y - rect.height - pad : y + pad
    el.style.left = `${Math.max(4, leftRaw)}px`
    el.style.top = `${Math.max(4, topRaw)}px`
  }, [x, y, day])

  const buckets = day.buckets ?? []
  const isEmpty = day.count === 0

  return (
    <div
      ref={tooltipRef}
      className="fixed pointer-events-none z-[100] min-w-[180px] rounded-lg bg-[#18181b] px-3 py-2.5 text-[11.5px] text-zinc-300 shadow-2xl"
      style={{ left: x + 14, top: y + 14 }}
    >
      <div className="mb-1.5 text-[12px] font-semibold text-white">
        {shortDate(day.date)}
      </div>
      {buckets.length > 0 ? (
        buckets.map((b) => (
          <div
            key={b.type}
            className="flex items-center gap-1.5 py-0.5"
          >
            <span
              className="inline-block h-2 w-2 rounded-[2px] flex-shrink-0"
              style={{ background: colorFor(b.type) }}
            />
            <span>{b.type}</span>
            <span className="ml-auto text-white font-semibold tabular-nums">
              {b.count}
            </span>
          </div>
        ))
      ) : null}
      <div className="mt-1.5 pt-1.5 border-t border-zinc-700 text-zinc-200 font-semibold">
        Total {day.count}
        {isEmpty && (
          <span className="ml-1 text-[#f59e0b] italic font-medium">
            · empty day
          </span>
        )}
      </div>
    </div>
  )
}

// --- Row 2: Pipeline Cards ---

function PipelineCard({
  title,
  type,
  widget,
}: {
  title: string
  type: string
  widget: PipelineWidget
}) {
  const stages = widget.stages ?? []
  const total = widget.total

  return (
    <Link to={routeFor(type)} className="block">
      <Card className="rounded-[14px] border-border shadow-stripe hover:shadow-stripe-elevated hover:-translate-y-[1px] transition-all cursor-pointer">
        <CardHeader className="pb-3">
          <div className="flex items-baseline justify-between">
            <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
              {title}
            </CardTitle>
            <span className="text-[22px] font-bold tabular-nums tracking-tight text-foreground">
              {total}
            </span>
          </div>
        </CardHeader>
        <CardContent className="pb-4">
          {total === 0 ? (
            <div className="h-2 rounded-full bg-muted/60" />
          ) : (
            <>
              <div className="flex h-2 rounded-full overflow-hidden">
                {stages.map((s) =>
                  s.count > 0 ? (
                    <div
                      key={s.stage}
                      className="h-full first:rounded-l-full last:rounded-r-full"
                      style={{
                        flex: s.count,
                        background: stageColorFor(type, s.stage),
                      }}
                      title={`${s.stage}: ${s.count}`}
                    />
                  ) : null,
                )}
              </div>
              <div className="flex flex-wrap gap-x-3 gap-y-1 mt-2.5">
                {stages.map((s) => (
                  <span
                    key={s.stage}
                    className="inline-flex items-center gap-1 text-[11px] text-muted-foreground"
                  >
                    <span
                      className="inline-block h-1.5 w-1.5 rounded-full flex-shrink-0"
                      style={{ background: stageColorFor(type, s.stage) }}
                    />
                    {s.stage}{" "}
                    <span className="font-semibold text-foreground tabular-nums">
                      {s.count}
                    </span>
                  </span>
                ))}
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </Link>
  )
}

// --- Row 3: Actions + Projects ---

function ActionsStatusCard({ widget }: { widget: ActionsWidget }) {
  const rows = [
    {
      label: "Open",
      count: widget.open,
      status: "next",
      dot: "oklch(0.55 0.22 275)",
    },
    {
      label: "In progress",
      count: widget.in_progress,
      status: "doing",
      dot: "oklch(0.65 0.17 65)",
    },
    {
      label: "Blocked",
      count: widget.blocked,
      status: "waiting",
      dot: "oklch(0.58 0.22 5)",
    },
    {
      label: "Done",
      count: widget.done,
      status: "done",
      dot: "oklch(0.62 0.19 150)",
    },
  ]
  const isEmpty = widget.total === 0

  return (
    <Card className="rounded-[14px] border-border shadow-stripe hover:shadow-stripe-elevated transition-shadow">
      <CardHeader className="pb-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
              Actions
            </CardTitle>
            <p className="mt-0.5 text-xs text-muted-foreground">
              By status · click a row to filter
            </p>
          </div>
          <div className="flex items-center gap-2">
            {widget.stalled > 0 && (
              <StalledPill count={widget.stalled} to="/actions?stalled=true" />
            )}
            <Link
              to="/actions"
              className="text-[11px] text-primary hover:underline whitespace-nowrap"
            >
              View all &rarr;
            </Link>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {isEmpty ? (
          <EmptyState
            message="No actions tracked yet."
            cta="Go to Actions"
            to="/actions"
          />
        ) : (
          <div className="flex flex-col gap-0.5">
            {rows.map((row) => (
              <Link
                key={row.status}
                to={`/actions?status=${row.status}`}
                className="flex items-center gap-3 rounded-lg px-3 py-[11px] text-left hover:bg-muted/60 hover:translate-x-[2px] transition-all"
              >
                <span
                  className="h-2 w-2 rounded-full flex-shrink-0"
                  style={{ background: row.dot }}
                />
                <span className="flex-1 text-[13.5px] font-medium text-foreground">
                  {row.label}
                </span>
                <span className="text-[15px] font-bold tabular-nums tracking-tight text-foreground">
                  {row.count}
                </span>
              </Link>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function ActiveProjectsCard({ widget }: { widget: ProjectsWidget }) {
  const top = widget.top ?? []
  const isEmpty = widget.active === 0

  return (
    <Card className="rounded-[14px] border-border shadow-stripe hover:shadow-stripe-elevated transition-shadow">
      <CardHeader className="pb-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
              Active projects
            </CardTitle>
            <p className="mt-0.5 text-xs text-muted-foreground">
              Touched in last 7 days
            </p>
          </div>
          <Link
            to="/projects"
            className="text-[11px] text-primary hover:underline whitespace-nowrap"
          >
            View all &rarr;
          </Link>
        </div>
      </CardHeader>
      <CardContent>
        {isEmpty ? (
          <EmptyState
            message="No projects touched in the last 7 days."
            cta="Go to Projects"
            to="/projects"
          />
        ) : (
          <>
            <Link
              to="/projects"
              className="flex items-baseline gap-2.5 px-3 py-2.5 rounded-lg hover:bg-muted/60 transition-colors w-full"
            >
              <span className="text-[40px] font-bold tracking-[-0.03em] leading-none text-foreground tabular-nums">
                {widget.active}
              </span>
              <span className="text-xs font-medium text-muted-foreground">
                active &middot; touched last 7d
              </span>
            </Link>
            {top.length > 0 && (
              <div className="mt-2 pt-3 border-t border-border/60 flex flex-col gap-0.5">
                {top.map((p) => (
                  <Link
                    key={p.id}
                    to="/projects"
                    className="flex items-center gap-2.5 rounded-lg px-3 py-2 hover:bg-muted/60 hover:translate-x-[2px] transition-all"
                    title={p.id}
                  >
                    <span className="h-1 w-1 rounded-full bg-muted-foreground/60 flex-shrink-0" />
                    <span className="flex-1 text-[13px] font-medium text-foreground truncate">
                      {p.name}
                    </span>
                    <span className="text-[11.5px] text-muted-foreground tabular-nums whitespace-nowrap">
                      {relativeTime(p.last_touched)}
                    </span>
                  </Link>
                ))}
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}

// --- Reusable bits ---

function StalledPill({ count, to }: { count: number; to: string }) {
  return (
    <Link
      to={to}
      className="inline-flex items-center gap-1.5 rounded-full border border-[var(--stripe-ruby)]/30 bg-[var(--stripe-ruby)]/10 px-2.5 py-1 text-[11.5px] font-semibold text-[var(--stripe-ruby)] hover:bg-[var(--stripe-ruby)]/20 hover:-translate-y-[1px] transition-all whitespace-nowrap"
      title="Blocked or in-progress and not updated in 7+ days"
    >
      <Clock className="h-3 w-3" />
      {count} stalled &gt; 7d
    </Link>
  )
}

function EmptyState({
  message,
  cta,
  to,
}: {
  message: string
  cta: string
  to: string
}) {
  return (
    <div className="py-8 text-center">
      <p className="text-sm text-muted-foreground mb-2">{message}</p>
      <Link to={to} className="text-xs text-primary hover:underline">
        {cta} &rarr;
      </Link>
    </div>
  )
}

// --- Fully-empty onboarding surface ---

function DashboardOnboarding() {
  return (
    <>
      <PageHeader title="Dashboard" description="Welcome to CorticalStack" />
      <Card className="rounded-[14px] border-border shadow-stripe-elevated max-w-[760px] mx-auto">
        <CardContent className="py-12 px-12">
          <div className="text-center">
            <span className="inline-block rounded-full bg-primary/10 px-3 py-1 text-[11.5px] font-semibold text-primary tracking-wide mb-4">
              FIRST-RUN
            </span>
            <h2 className="text-2xl font-bold tracking-tight mb-2.5">
              Nothing to show yet
            </h2>
            <p className="text-sm text-muted-foreground max-w-[560px] mx-auto mb-8">
              Your vault is empty. Start feeding CorticalStack — the dashboard
              fills itself as soon as any widget has data. There's no manual
              cataloguing and no empty-state chrome once something lands.
            </p>
            <div className="flex items-center justify-center gap-3">
              <Button
                asChild
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-md font-medium gap-1.5"
              >
                <Link to="/library">
                  <BookOpen className="h-4 w-4" /> Browse Library
                </Link>
              </Button>
              <Button
                asChild
                variant="outline"
                className="border-border rounded-md font-medium gap-1.5"
              >
                <Link to="/config">
                  <Settings className="h-4 w-4" /> Configure integrations
                </Link>
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </>
  )
}
