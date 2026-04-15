import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { ChevronDown, Lightbulb, Mic, FileCheck } from "lucide-react"
import type { LucideIcon } from "lucide-react"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { PageHeader } from "@/components/layout/page-header"
import { SkeletonPage } from "@/components/shared/skeleton-card"
import { api } from "@/lib/api"
import { cn } from "@/lib/utils"
import type { Meeting, PRD, ShapeUpThread } from "@/types/api"

// Per-pipeline accent colors. These are deliberate — distinct enough
// from the existing UI palette to give each pipeline its own identity
// without clashing with shadcn / theme tokens. Tints are derived in
// JS so the same hex feeds backgrounds, borders, and badges.
const PIPELINE_COLORS = {
  productIdeas: "#E8C547", // gold
  meetings: "#47B5E8", // sky
  prd: "#E85B7A", // coral
} as const

function withAlpha(hex: string, alpha: number): string {
  // hex like "#RRGGBB" → "rgba(r,g,b,a)". Avoids importing color libs
  // and is fine for a fixed-palette accent system.
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

interface Stage {
  key: string
  label: string
  description?: string
}

interface PipelineDef {
  id: string
  name: string
  description: string
  color: string
  icon: LucideIcon
  stages: Stage[]
}

interface PipelineItem {
  id: string
  name: string
  stageKey: string
  hint?: string // optional secondary line (e.g. project name)
}

// PIPELINES is the source of truth for stage order and labels. Items
// are derived from live queries below and grouped by stage key.
const PIPELINES: PipelineDef[] = [
  {
    id: "product-ideas",
    name: "Product Ideas",
    description: "Source signal → pitchable idea, ShapeUp style",
    color: PIPELINE_COLORS.productIdeas,
    icon: Lightbulb,
    stages: [
      { key: "frame", label: "Frame", description: "Source → Problem → Outcome" },
      { key: "shape", label: "Shape", description: "Requirements & solution options" },
      { key: "breadboard", label: "Breadboard", description: "UI & code affordances" },
      { key: "pitch", label: "Pitch", description: "Stakeholder-ready package" },
    ],
  },
  {
    id: "meetings",
    name: "Meetings",
    description: "Recorded meeting → actionable summary",
    color: PIPELINE_COLORS.meetings,
    icon: Mic,
    stages: [
      { key: "transcript", label: "Transcript", description: "Raw audio or auto-transcript" },
      { key: "summary", label: "Summary", description: "Decisions, action items, topics" },
    ],
  },
  {
    id: "prd",
    name: "PRD",
    description: "Approved pitch → engineering-ready PRD",
    color: PIPELINE_COLORS.prd,
    icon: FileCheck,
    stages: [
      { key: "pitch_input", label: "Pitch", description: "Approved pitch awaiting PRD" },
      { key: "shaped_prd", label: "Shaped PRD", description: "Full PRD ready for engineering" },
    ],
  },
]

export function PipelinesPage() {
  const threadsQuery = useQuery({
    queryKey: ["pipelines", "threads"],
    queryFn: api.listThreads,
    staleTime: 60_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
  })
  const meetingsQuery = useQuery({
    queryKey: ["pipelines", "meetings"],
    queryFn: api.listMeetings,
    staleTime: 60_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
  })
  const prdsQuery = useQuery({
    queryKey: ["pipelines", "prds"],
    queryFn: api.listPRDs,
    staleTime: 60_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
  })

  const isLoading =
    threadsQuery.isLoading || meetingsQuery.isLoading || prdsQuery.isLoading
  const isError = threadsQuery.error || meetingsQuery.error || prdsQuery.error

  // Derive items per pipeline. All three live queries collapse into a
  // single { pipelineId → items[] } map, keyed by stage. useMemo
  // recomputes only when any underlying query changes.
  const itemsByPipeline = useMemo(() => {
    const threads = threadsQuery.data ?? []
    const meetings = meetingsQuery.data ?? []
    const prds = prdsQuery.data ?? []

    // Product Ideas: every ShapeUp thread, classified by its current
    // stage. Threads at "raw" are skipped — the spec's pipeline starts
    // at frame, and raw is essentially a pre-pipeline inbox.
    const productIdeaItems: PipelineItem[] = threads
      .filter((t) => t.current_stage !== "raw")
      .map((t: ShapeUpThread) => ({
        id: t.id,
        name: t.title,
        stageKey: t.current_stage,
      }))

    // Meetings: every meeting note, at its declared stage. A single
    // logical meeting may appear twice (transcript + summary) — that's
    // intentional: the dashboard shows where work-in-progress lives.
    const meetingItems: PipelineItem[] = meetings.map((m: Meeting) => ({
      id: m.id,
      name: m.title,
      stageKey: m.stage,
    }))

    // PRD pipeline:
    //   pitch_input  = ShapeUp threads at "pitch" that don't yet have a
    //                  corresponding PRD pointing at their pitch path
    //   shaped_prd   = every PRD in the store
    // The dedup makes the dashboard show "pitches still waiting to
    // become PRDs" on the left, "PRDs that exist" on the right —
    // visually conveying conversion progress.
    const prdSourcePitches = new Set<string>(
      prds.map((p: PRD) => p.source_pitch).filter(Boolean),
    )
    const pendingPitches: PipelineItem[] = threads
      .filter((t) => t.current_stage === "pitch")
      .filter((t) => {
        const pitchArtifact = t.artifacts.find((a) => a.stage === "pitch")
        return !pitchArtifact || !prdSourcePitches.has(pitchArtifact.path)
      })
      .map((t) => ({
        id: `pitch:${t.id}`,
        name: t.title,
        stageKey: "pitch_input",
      }))
    const shapedPRDs: PipelineItem[] = prds.map((p: PRD) => ({
      id: p.id,
      name: p.title,
      stageKey: "shaped_prd",
      hint: p.status,
    }))

    return {
      "product-ideas": productIdeaItems,
      meetings: meetingItems,
      prd: [...pendingPitches, ...shapedPRDs],
    } as Record<string, PipelineItem[]>
  }, [threadsQuery.data, meetingsQuery.data, prdsQuery.data])

  if (isLoading) return <SkeletonPage />

  if (isError) {
    return (
      <>
        <PageHeader title="Pipelines" description="Stage-by-stage view of every workflow" />
        <Card className="rounded-md border-destructive/40 bg-destructive/5">
          <CardContent className="py-6">
            <p className="text-sm text-destructive">
              Could not load pipeline data. Some endpoints may be unreachable.
            </p>
          </CardContent>
        </Card>
      </>
    )
  }

  const totalItems = Object.values(itemsByPipeline).reduce(
    (sum, items) => sum + items.length,
    0,
  )

  return (
    <>
      <div className="flex items-start justify-between mb-6">
        <PageHeader
          title="Pipelines"
          description="Stage-by-stage view of every workflow"
        />
        <span className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1.5 text-xs text-muted-foreground whitespace-nowrap select-none">
          <strong className="text-foreground font-semibold tabular-nums">
            {totalItems}
          </strong>
          <span>items in flight</span>
        </span>
      </div>

      {/* Summary strip */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 mb-6">
        {PIPELINES.map((p) => (
          <SummaryCard
            key={p.id}
            pipeline={p}
            items={itemsByPipeline[p.id] ?? []}
          />
        ))}
      </div>

      {/* Pipeline rows */}
      <div className="flex flex-col gap-4">
        {PIPELINES.map((p) => (
          <PipelineRow
            key={p.id}
            pipeline={p}
            items={itemsByPipeline[p.id] ?? []}
          />
        ))}
      </div>
    </>
  )
}

// --- Summary strip card ---

function SummaryCard({
  pipeline,
  items,
}: {
  pipeline: PipelineDef
  items: PipelineItem[]
}) {
  const Icon = pipeline.icon

  // Distribution: width per segment proportional to item count, with
  // a minimum so even single-item stages are visible.
  const segments = pipeline.stages.map((stage) => {
    const count = items.filter((i) => i.stageKey === stage.key).length
    return { stage, count }
  })
  const total = items.length
  const maxCount = Math.max(1, ...segments.map((s) => s.count))

  return (
    <Card
      className="rounded-[14px] border-border shadow-stripe hover:shadow-stripe-elevated transition-shadow"
      style={{ borderColor: total > 0 ? withAlpha(pipeline.color, 0.18) : undefined }}
    >
      <CardContent className="py-5">
        <div className="flex items-start justify-between mb-3">
          <div className="flex items-center gap-2">
            <span
              className="flex h-7 w-7 items-center justify-center rounded-md"
              style={{ background: withAlpha(pipeline.color, 0.12) }}
            >
              <Icon className="h-4 w-4" style={{ color: pipeline.color }} />
            </span>
            <span className="text-[14px] font-semibold tracking-tight text-foreground">
              {pipeline.name}
            </span>
          </div>
          <span
            className="text-[24px] font-bold tracking-tight text-foreground tabular-nums leading-none"
            style={{ fontFeatureSettings: '"tnum"' }}
          >
            {total}
          </span>
        </div>

        {/* Mini distribution bar */}
        {total > 0 ? (
          <div className="flex h-1.5 gap-[2px] rounded-full overflow-hidden">
            {segments.map(({ stage, count }) => {
              const opacity =
                count === 0 ? 0.08 : 0.3 + 0.7 * (count / maxCount)
              return (
                <div
                  key={stage.key}
                  className="flex-1"
                  style={{
                    background: withAlpha(pipeline.color, opacity),
                  }}
                  title={`${stage.label}: ${count}`}
                />
              )
            })}
          </div>
        ) : (
          <div className="h-1.5 rounded-full bg-muted/40" />
        )}

        <p className="mt-3 text-[11px] text-muted-foreground">
          {pipeline.description}
        </p>
      </CardContent>
    </Card>
  )
}

// --- Pipeline row (collapsible) ---

function PipelineRow({
  pipeline,
  items,
}: {
  pipeline: PipelineDef
  items: PipelineItem[]
}) {
  const [expanded, setExpanded] = useState(true)
  const Icon = pipeline.icon

  return (
    <Card className="rounded-[14px] border-border shadow-stripe overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded((e) => !e)}
        className="w-full flex items-center gap-3 px-5 py-4 text-left hover:bg-muted/40 transition-colors"
        aria-expanded={expanded}
      >
        <span
          className="flex h-9 w-9 items-center justify-center rounded-lg flex-shrink-0"
          style={{ background: withAlpha(pipeline.color, 0.12) }}
        >
          <Icon className="h-5 w-5" style={{ color: pipeline.color }} />
        </span>
        <div className="flex-1 min-w-0">
          <div className="flex items-baseline gap-3">
            <h3 className="text-[17px] font-bold tracking-tight text-foreground">
              {pipeline.name}
            </h3>
            <span className="text-[12px] text-muted-foreground truncate">
              {pipeline.description}
            </span>
          </div>
        </div>
        <span
          className="inline-flex items-center justify-center rounded-full px-2.5 py-0.5 text-[11.5px] font-semibold tabular-nums flex-shrink-0"
          style={{
            background: withAlpha(pipeline.color, 0.15),
            color: pipeline.color,
            border: `1px solid ${withAlpha(pipeline.color, 0.3)}`,
          }}
        >
          {items.length} {items.length === 1 ? "item" : "items"}
        </span>
        <ChevronDown
          className={cn(
            "h-4 w-4 text-muted-foreground transition-transform flex-shrink-0",
            expanded && "rotate-180",
          )}
        />
      </button>

      {expanded && <StageFlow pipeline={pipeline} items={items} />}
    </Card>
  )
}

// --- Stage flow (horizontal scrollable row of stage nodes + connectors) ---

function StageFlow({
  pipeline,
  items,
}: {
  pipeline: PipelineDef
  items: PipelineItem[]
}) {
  return (
    <div className="border-t border-border bg-muted/10 px-5 py-5">
      <div className="flex items-stretch gap-2 overflow-x-auto pb-1">
        {pipeline.stages.map((stage, i) => {
          const stageItems = items.filter((it) => it.stageKey === stage.key)
          const isLast = i === pipeline.stages.length - 1
          return (
            <div key={stage.key} className="flex items-stretch">
              <StageNode
                stage={stage}
                items={stageItems}
                color={pipeline.color}
              />
              {!isLast && (
                <Connector
                  active={stageItems.length > 0}
                  color={pipeline.color}
                />
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// --- Stage node ---

function StageNode({
  stage,
  items,
  color,
}: {
  stage: Stage
  items: PipelineItem[]
  color: string
}) {
  const empty = items.length === 0
  return (
    <div
      className={cn(
        "min-w-[200px] max-w-[260px] rounded-xl border p-3 flex flex-col gap-2 transition-colors",
        empty ? "bg-muted/20" : "bg-card hover:border-foreground/20",
      )}
      style={{
        borderColor: empty
          ? undefined
          : withAlpha(color, 0.18),
      }}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-[10px] font-bold tracking-[0.06em] uppercase text-muted-foreground">
          {stage.label}
        </span>
        <span
          className="inline-flex h-5 min-w-[20px] items-center justify-center rounded-full px-1.5 text-[10px] font-bold tabular-nums"
          style={{
            background: empty ? "transparent" : withAlpha(color, 0.15),
            color: empty ? "var(--muted-foreground)" : color,
            border: `1px solid ${empty ? "var(--border)" : withAlpha(color, 0.3)}`,
          }}
        >
          {items.length}
        </span>
      </div>

      {empty ? (
        <p className="py-3 text-center text-[11px] italic text-muted-foreground/60">
          No items
        </p>
      ) : (
        <div className="flex flex-col gap-1.5">
          {items.map((item) => (
            <ItemPill key={item.id} item={item} color={color} />
          ))}
        </div>
      )}
    </div>
  )
}

// --- Item pill ---

function ItemPill({ item, color }: { item: PipelineItem; color: string }) {
  const initials = useMemo(() => {
    const words = item.name
      .split(/\s+/)
      .filter(Boolean)
      .filter((w) => /[a-zA-Z0-9]/.test(w))
    if (words.length === 0) return "??"
    if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
    return (words[0][0] + words[1][0]).toUpperCase()
  }, [item.name])

  return (
    <div
      className="flex items-center gap-2 rounded-md px-2 py-1.5 text-[11px]"
      style={{
        background: withAlpha(color, 0.08),
        border: `1px solid ${withAlpha(color, 0.18)}`,
      }}
    >
      <span
        className="flex h-5 w-5 items-center justify-center rounded-full text-[9px] font-bold flex-shrink-0"
        style={{
          background: withAlpha(color, 0.18),
          color: color,
        }}
      >
        {initials}
      </span>
      <span className="flex-1 truncate text-foreground/85" title={item.name}>
        {item.name}
      </span>
      {item.hint && (
        <Badge
          variant="outline"
          className="h-4 px-1 text-[9px] font-medium uppercase tracking-wider"
        >
          {item.hint}
        </Badge>
      )}
    </div>
  )
}

// --- Connector (SVG arrow between stage nodes) ---

function Connector({ active, color }: { active: boolean; color: string }) {
  // 56px wide × stage card height. The line + arrowhead are static; the
  // dash animation only runs when the source stage has items.
  return (
    <div className="flex items-center justify-center self-center px-1">
      <svg
        width="56"
        height="20"
        viewBox="0 0 56 20"
        aria-hidden
        className="overflow-visible"
      >
        <line
          x1="0"
          y1="10"
          x2="46"
          y2="10"
          stroke={active ? color : "currentColor"}
          strokeOpacity={active ? 0.7 : 0.18}
          strokeWidth="2"
          strokeLinecap="round"
          strokeDasharray={active ? "5 5" : "0"}
        >
          {active && (
            <animate
              attributeName="stroke-dashoffset"
              from="0"
              to="-20"
              dur="1.2s"
              repeatCount="indefinite"
            />
          )}
        </line>
        <polygon
          points="44,4 56,10 44,16"
          fill={active ? color : "currentColor"}
          fillOpacity={active ? 0.75 : 0.2}
        />
      </svg>
    </div>
  )
}
