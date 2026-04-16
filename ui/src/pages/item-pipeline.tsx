import { useState, useMemo } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { Link, useParams } from "react-router-dom"
import Markdown from "react-markdown"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { PageHeader } from "@/components/layout/page-header"
import { SkeletonPage } from "@/components/shared/skeleton-card"
import { api } from "@/lib/api"
import { cn } from "@/lib/utils"
import { toast } from "sonner"
import { ArrowLeft, Check, Circle, Loader2 } from "lucide-react"
import type { ShapeUpThread, Meeting, Prototype, Document } from "@/types/api"

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const stageOrders: Record<string, string[]> = {
  product: ["idea", "frame", "shape", "breadboard", "pitch"],
  meeting: ["transcript", "note"],
  document: ["input", "note"],
  prototype: ["breadboard", "in_progress", "final"],
}

const PIPELINE_ACCENT: Record<string, string> = {
  product: "#9B8AFF",
  meeting: "#47B5E8",
  document: "#48D597",
  prototype: "#E8C547",
}

const stageColors: Record<string, Record<string, string>> = {
  product: {
    idea: "#8B8FA3", frame: "#47B5E8", shape: "#9B8AFF",
    breadboard: "#E85B9B", pitch: "#48D597",
  },
  meeting: {
    transcript: "#47B5E8", note: "#48D597",
  },
  document: {
    input: "#8B8FA3", note: "#48D597",
  },
  prototype: {
    breadboard: "#E85B9B", in_progress: "#E8C547", final: "#48D597",
  },
}

function colorFor(type: string, stage: string): string {
  return stageColors[type]?.[stage] ?? PIPELINE_ACCENT[type] ?? "#8B8FA3"
}

function withAlpha(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

const stageLabels: Record<string, string> = {
  idea: "Idea", frame: "Frame", shape: "Shape",
  breadboard: "Breadboard", pitch: "Pitch",
  transcript: "Transcript", note: "Note",
  input: "Input", in_progress: "In Progress", final: "Final",
}

function label(s: string): string {
  return stageLabels[s] ?? s.charAt(0).toUpperCase() + s.slice(1)
}

const typeTitles: Record<string, string> = {
  product: "Product", meeting: "Meeting",
  document: "Document", prototype: "Prototype",
}

function normalizeStage(type: string, stage: string): string {
  if (type === "product" && stage === "raw") return "idea"
  return stage
}

function formatShortDate(iso?: string): string {
  if (!iso) return ""
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ""
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" })
}

function stripFrontmatter(raw: string): string {
  const trimmed = raw.trimStart()
  if (!trimmed.startsWith("---")) return raw
  const end = trimmed.indexOf("---", 3)
  if (end < 0) return raw
  return trimmed.slice(end + 3).trimStart()
}

// ---------------------------------------------------------------------------
// Stage classification
// ---------------------------------------------------------------------------

type StageStatus = "completed" | "current" | "future"

interface StageInfo {
  stage: string
  status: StageStatus
  date?: string
}

function classifyStages(
  type: string,
  currentStage: string,
  artifactDates?: Map<string, string>,
): StageInfo[] {
  const order = stageOrders[type] ?? []
  const currentIdx = order.indexOf(currentStage)

  return order.map((s, idx) => {
    let status: StageStatus
    if (type === "product" && artifactDates) {
      if (s === currentStage) status = "current"
      else if (artifactDates.has(s) || idx < currentIdx) status = "completed"
      else status = "future"
    } else {
      if (idx < currentIdx) status = "completed"
      else if (idx === currentIdx) status = "current"
      else status = "future"
    }
    return { stage: s, status, date: artifactDates?.get(s) }
  })
}

// ---------------------------------------------------------------------------
// Data hooks
// ---------------------------------------------------------------------------

interface PipelineData {
  title: string
  currentStage: string
  contentPath?: string
  isLoading: boolean
  error: string | null
}

function useProductData(id: string): PipelineData {
  const { data, isLoading, error } = useQuery<ShapeUpThread>({
    queryKey: ["thread", id],
    queryFn: () => api.getThread(id),
    enabled: !!id,
  })
  return {
    title: data?.title ?? "",
    currentStage: normalizeStage("product", data?.current_stage ?? "idea"),
    isLoading,
    error: error ? String(error) : null,
  }
}

function useMeetingData(id: string): PipelineData {
  const { data, isLoading, error } = useQuery<Meeting[]>({
    queryKey: ["meetings"],
    queryFn: () => api.listMeetings(),
    enabled: !!id,
  })
  const m = data?.find((x) => x.id === id)
  return {
    title: m?.title ?? "",
    currentStage: m?.stage ?? "transcript",
    contentPath: m?.path,
    isLoading,
    error: error ? String(error) : null,
  }
}

function useDocumentData(id: string): PipelineData {
  const { data, isLoading, error } = useQuery<Document>({
    queryKey: ["document", id],
    queryFn: () => api.getDocument(id),
    enabled: !!id,
  })
  return {
    title: data?.title ?? "",
    currentStage: data?.stage ?? "input",
    contentPath: data?.path,
    isLoading,
    error: error ? String(error) : null,
  }
}

function usePrototypeData(id: string): PipelineData {
  const { data, isLoading, error } = useQuery<Prototype[]>({
    queryKey: ["prototypes"],
    queryFn: () => api.listPrototypes(),
    enabled: !!id,
  })
  const p = data?.find((x) => x.id === id)
  return {
    title: p?.title ?? "",
    currentStage: p?.stage ?? "breadboard",
    contentPath: p?.folder_path ? `${p.folder_path}/spec.md` : undefined,
    isLoading,
    error: error ? String(error) : null,
  }
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export function ItemPipelinePage() {
  const { type = "", id = "" } = useParams<{ type: string; id: string }>()
  const queryClient = useQueryClient()
  const [selectedStage, setSelectedStage] = useState<string | null>(null)
  const [advanceLoading, setAdvanceLoading] = useState(false)

  const productData = useProductData(type === "product" ? id : "")
  const meetingData = useMeetingData(type === "meeting" ? id : "")
  const documentData = useDocumentData(type === "document" ? id : "")
  const prototypeData = usePrototypeData(type === "prototype" ? id : "")

  const pipelineData =
    type === "product" ? productData
    : type === "meeting" ? meetingData
    : type === "document" ? documentData
    : prototypeData

  const { title, currentStage, contentPath, isLoading, error } = pipelineData
  const accent = PIPELINE_ACCENT[type] ?? "#8B8FA3"

  // Product artifact data
  const { data: threadData } = useQuery<ShapeUpThread>({
    queryKey: ["thread", id],
    queryFn: () => api.getThread(id),
    enabled: type === "product" && !!id,
  })

  const artifactPathByStage = useMemo(() => {
    const map = new Map<string, string>()
    if (threadData?.artifacts) {
      for (const a of threadData.artifacts)
        map.set(normalizeStage("product", a.stage), a.path)
    }
    return map
  }, [threadData])

  const artifactDateByStage = useMemo(() => {
    const map = new Map<string, string>()
    if (threadData?.artifacts) {
      for (const a of threadData.artifacts)
        map.set(normalizeStage("product", a.stage), a.created)
    }
    return map
  }, [threadData])

  const stages = useMemo(
    () => classifyStages(
      type, currentStage,
      type === "product" ? artifactDateByStage : undefined,
    ),
    [type, currentStage, artifactDateByStage],
  )

  const viewStage = selectedStage ?? currentStage

  const contentFilePath = useMemo(() => {
    if (type === "product") return artifactPathByStage.get(viewStage) ?? null
    return contentPath ?? null
  }, [type, viewStage, artifactPathByStage, contentPath])

  const { data: rawContent, isLoading: contentLoading } = useQuery<string>({
    queryKey: ["vault-file", contentFilePath],
    queryFn: () => api.getVaultFile(contentFilePath!),
    enabled: !!contentFilePath,
    staleTime: 30_000,
  })

  const stageContent = rawContent ? stripFrontmatter(rawContent) : null

  // Advance
  const nextStage = useMemo(() => {
    const order = stageOrders[type] ?? []
    const idx = order.indexOf(currentStage)
    if (idx >= 0 && idx < order.length - 1) return order[idx + 1]
    return null
  }, [type, currentStage])

  const advanceMutation = useMutation({
    mutationFn: async (next: string) => {
      if (type === "product") return api.advanceThread(id, { target_stage: next })
      if (type === "meeting") return api.setMeetingStage(id, next)
      if (type === "document") return api.setDocumentStage(id, next)
      return api.setPrototypeStage(id, next)
    },
    onMutate: () => setAdvanceLoading(true),
    onSuccess: () => {
      toast.success(`Advanced to next stage`)
      if (type === "product") queryClient.invalidateQueries({ queryKey: ["thread", id] })
      else if (type === "meeting") queryClient.invalidateQueries({ queryKey: ["meetings"] })
      else if (type === "document") queryClient.invalidateQueries({ queryKey: ["document", id] })
      else queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      queryClient.invalidateQueries({ queryKey: ["card-detail", type] })
      setSelectedStage(null)
    },
    onError: (err) => toast.error(`Failed to advance: ${err instanceof Error ? err.message : "Unknown error"}`),
    onSettled: () => setAdvanceLoading(false),
  })

  if (isLoading) return <SkeletonPage />

  if (error) {
    return (
      <>
        <BackLink type={type} />
        <PageHeader title={typeTitles[type] ?? type} description="Pipeline item" />
        <Card className="rounded-md border-destructive/40 bg-destructive/5">
          <CardContent className="py-6">
            <p className="text-sm text-destructive">
              Could not load item. Refresh in a moment.
            </p>
          </CardContent>
        </Card>
      </>
    )
  }

  return (
    <div className="space-y-6">
      <BackLink type={type} />

      <PageHeader
        title={title || "Untitled"}
        description={`${typeTitles[type] ?? type} pipeline — currently at ${label(currentStage)}`}
      />

      {/* Stage flow */}
      <Card
        className="rounded-[14px] border-border shadow-stripe overflow-hidden"
        style={{ borderColor: withAlpha(accent, 0.18) }}
      >
        <CardContent className="py-6 px-4">
          <div className="overflow-x-auto">
            <div className="flex items-start gap-0 min-w-max px-2">
              {stages.map((s, idx) => (
                <div key={s.stage} className="flex items-start">
                  <StageNode
                    stage={s.stage}
                    status={s.status}
                    date={s.date}
                    color={colorFor(type, s.stage)}
                    accent={accent}
                    isSelected={viewStage === s.stage}
                    onClick={() => {
                      if (s.status !== "future") setSelectedStage(s.stage)
                    }}
                  />
                  {idx < stages.length - 1 && (
                    <Connector
                      active={s.status === "completed"}
                      color={accent}
                    />
                  )}
                </div>
              ))}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Content area */}
      <Card
        className="rounded-[14px] border-border shadow-stripe"
        style={{ borderColor: withAlpha(colorFor(type, viewStage), 0.15) }}
      >
        <CardContent className="p-6">
          <div className="flex items-center gap-3 mb-5 pb-4 border-b border-border">
            <span
              className="flex h-7 w-7 items-center justify-center rounded-md flex-shrink-0"
              style={{ background: withAlpha(colorFor(type, viewStage), 0.15) }}
            >
              <span
                className="h-2.5 w-2.5 rounded-full"
                style={{ background: colorFor(type, viewStage) }}
              />
            </span>
            <h3 className="text-[15px] font-semibold tracking-tight text-foreground">
              {label(viewStage)}
            </h3>
            {viewStage === currentStage && (
              <span
                className="text-[10px] font-bold tracking-[0.06em] uppercase px-2 py-0.5 rounded-full"
                style={{
                  background: withAlpha(accent, 0.15),
                  color: accent,
                  border: `1px solid ${withAlpha(accent, 0.3)}`,
                }}
              >
                Current
              </span>
            )}
            {stages.find(s => s.stage === viewStage)?.date && (
              <span className="text-[11px] text-muted-foreground ml-auto tabular-nums">
                {formatShortDate(stages.find(s => s.stage === viewStage)?.date)}
              </span>
            )}
          </div>

          {contentLoading ? (
            <div className="flex items-center gap-2 py-10 justify-center text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading content...
            </div>
          ) : stageContent ? (
            <div className="prose prose-sm max-w-none dark:prose-invert prose-headings:text-foreground prose-p:text-foreground/90 prose-li:text-foreground/90 prose-strong:text-foreground prose-code:text-foreground/80 prose-code:bg-muted prose-code:px-1.5 prose-code:py-0.5 prose-code:rounded prose-pre:bg-muted/60 prose-pre:border prose-pre:border-border">
              <Markdown>{stageContent}</Markdown>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground py-10 text-center italic">
              {contentFilePath
                ? "No content available for this stage."
                : type === "product"
                  ? "No artifact generated for this stage yet."
                  : "No content path available."}
            </p>
          )}
        </CardContent>
      </Card>

      {/* Advance button */}
      {nextStage && (
        <div className="flex justify-end">
          <Button
            onClick={() => advanceMutation.mutate(nextStage)}
            disabled={advanceLoading}
            className="gap-2 rounded-lg font-semibold text-[13px] px-5 py-2.5 h-auto"
            style={{
              background: colorFor(type, nextStage),
              color: "white",
              boxShadow: `0 2px 8px ${withAlpha(colorFor(type, nextStage), 0.35)}`,
            }}
          >
            {advanceLoading ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Advancing...
              </>
            ) : (
              <>Advance to {label(nextStage)} &rarr;</>
            )}
          </Button>
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Stage node — inspired by the old pipelines page style
// ---------------------------------------------------------------------------

function StageNode({
  stage,
  status,
  date,
  color,
  accent,
  isSelected,
  onClick,
}: {
  stage: string
  status: StageStatus
  date?: string
  color: string
  accent: string
  isSelected: boolean
  onClick: () => void
}) {
  const isClickable = status !== "future"
  const isFuture = status === "future"
  const isCompleted = status === "completed"
  const isCurrent = status === "current"

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!isClickable}
      className={cn(
        "relative flex flex-col items-center gap-2 rounded-xl border-2 px-6 py-4 transition-all min-w-[130px]",
        isClickable && "cursor-pointer hover:scale-[1.02] hover:shadow-md",
        !isClickable && "cursor-default",
      )}
      style={{
        borderColor: isFuture ? "var(--border)" : isSelected ? color : withAlpha(color, 0.35),
        background: isFuture
          ? "var(--card)"
          : isSelected
            ? withAlpha(color, 0.1)
            : withAlpha(color, 0.05),
        opacity: isFuture ? 0.45 : 1,
        boxShadow: isSelected
          ? `0 0 0 2px var(--background), 0 0 0 4px ${withAlpha(color, 0.5)}`
          : "none",
      }}
    >
      {/* Status icon */}
      <div
        className="flex items-center justify-center h-8 w-8 rounded-full transition-colors"
        style={{
          background: isCompleted
            ? color
            : isCurrent
              ? withAlpha(accent, 0.2)
              : withAlpha("#8B8FA3", 0.12),
          border: isCurrent ? `2px solid ${accent}` : "none",
        }}
      >
        {isCompleted ? (
          <Check className="h-4 w-4 text-white" strokeWidth={2.5} />
        ) : isCurrent ? (
          <Circle className="h-3 w-3" style={{ color: accent, fill: accent }} />
        ) : (
          <Circle className="h-3 w-3 text-muted-foreground/40" />
        )}
      </div>

      {/* Label */}
      <span
        className={cn(
          "text-[11px] font-bold tracking-[0.06em] uppercase",
          isFuture ? "text-muted-foreground/50" : "text-foreground",
        )}
      >
        {label(stage)}
      </span>

      {/* Date */}
      {date ? (
        <span className="text-[10px] tabular-nums text-muted-foreground">
          {formatShortDate(date)}
        </span>
      ) : (
        <span className="text-[10px] text-muted-foreground/40 italic">
          {isFuture ? "Pending" : "—"}
        </span>
      )}
    </button>
  )
}

// ---------------------------------------------------------------------------
// Animated SVG connector (from old pipelines page)
// ---------------------------------------------------------------------------

function Connector({ active, color }: { active: boolean; color: string }) {
  return (
    <div className="flex items-center justify-center self-center px-1 pt-2">
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

// ---------------------------------------------------------------------------
// Back link
// ---------------------------------------------------------------------------

function BackLink({ type }: { type: string }) {
  return (
    <Link
      to={`/dashboard/${type}`}
      className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors mb-4"
    >
      <ArrowLeft className="h-4 w-4" />
      {typeTitles[type] ?? type}
    </Link>
  )
}
