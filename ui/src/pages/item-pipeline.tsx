import { useState, useMemo } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { Link, useParams } from "react-router-dom"
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
// Stage canonical orders
// ---------------------------------------------------------------------------

const stageOrders: Record<string, string[]> = {
  product: ["idea", "frame", "shape", "breadboard", "pitch"],
  meeting: ["transcript", "audio", "note"],
  document: ["need", "in_progress", "final"],
  prototype: ["need", "in_progress", "final"],
}

// ---------------------------------------------------------------------------
// Stage colors (OKLCH, matching dashboard-card.tsx)
// ---------------------------------------------------------------------------

const stageColors: Record<string, Record<string, string>> = {
  product: {
    idea: "oklch(0.55 0.04 250)",
    frame: "oklch(0.60 0.14 200)",
    shape: "oklch(0.55 0.22 275)",
    breadboard: "oklch(0.60 0.20 325)",
    pitch: "oklch(0.62 0.19 150)",
  },
  meeting: {
    transcript: "oklch(0.65 0.15 230)",
    audio: "oklch(0.70 0.16 85)",
    note: "oklch(0.62 0.19 150)",
  },
  document: {
    need: "oklch(0.55 0.04 250)",
    in_progress: "oklch(0.70 0.16 85)",
    final: "oklch(0.62 0.19 150)",
  },
  prototype: {
    need: "oklch(0.55 0.04 250)",
    in_progress: "oklch(0.70 0.16 85)",
    final: "oklch(0.62 0.19 150)",
  },
}

const fallbackColor = "oklch(0.60 0.10 250)"

function stageColor(cardType: string, stage: string): string {
  return stageColors[cardType]?.[stage] ?? fallbackColor
}

// ---------------------------------------------------------------------------
// Stage display labels (pretty-print underscored names)
// ---------------------------------------------------------------------------

const stageLabels: Record<string, string> = {
  idea: "Idea",
  frame: "Frame",
  shape: "Shape",
  breadboard: "Breadboard",
  pitch: "Pitch",
  transcript: "Transcript",
  audio: "Audio",
  note: "Note",
  need: "Need",
  in_progress: "In Progress",
  final: "Final",
}

function stageLabel(s: string): string {
  return stageLabels[s] ?? s.charAt(0).toUpperCase() + s.slice(1)
}

// ---------------------------------------------------------------------------
// Card type labels
// ---------------------------------------------------------------------------

const typeTitles: Record<string, string> = {
  product: "Product",
  meeting: "Meeting",
  document: "Document",
  prototype: "Prototype",
}

// ---------------------------------------------------------------------------
// Normalize ShapeUp "raw" stage to "idea" for display
// ---------------------------------------------------------------------------

function normalizeStage(type: string, stage: string): string {
  if (type === "product" && stage === "raw") return "idea"
  return stage
}

// ---------------------------------------------------------------------------
// Stage status classification
// ---------------------------------------------------------------------------

type StageStatus = "completed" | "current" | "future"

function classifyStages(
  type: string,
  currentStage: string,
  artifactStages?: Set<string>,
): { stage: string; status: StageStatus }[] {
  const order = stageOrders[type] ?? []
  const normalized = normalizeStage(type, currentStage)
  const currentIdx = order.indexOf(normalized)

  return order.map((s, idx) => {
    if (type === "product" && artifactStages) {
      // For products, a stage is completed if an artifact exists for it
      if (s === normalized) return { stage: s, status: "current" }
      if (artifactStages.has(s) || idx < currentIdx) return { stage: s, status: "completed" }
      return { stage: s, status: "future" }
    }
    // For non-product types: linear progression
    if (idx < currentIdx) return { stage: s, status: "completed" }
    if (idx === currentIdx) return { stage: s, status: "current" }
    return { stage: s, status: "future" }
  })
}

// ---------------------------------------------------------------------------
// Data fetching hooks
// ---------------------------------------------------------------------------

interface PipelineData {
  title: string
  currentStage: string
  contentByStage: Map<string, string | null> // null = needs fetch, string = content
  contentPath?: string // for non-product types, single content path
  isLoading: boolean
  error: string | null
}

function useProductData(id: string): PipelineData {
  const { data, isLoading, error } = useQuery<ShapeUpThread>({
    queryKey: ["thread", id],
    queryFn: () => api.getThread(id),
    enabled: !!id,
  })

  const contentByStage = useMemo(() => {
    const map = new Map<string, string | null>()
    if (data?.artifacts) {
      for (const a of data.artifacts) {
        const normalized = normalizeStage("product", a.stage)
        // Body is not in JSON response (json:"-"), so mark as needing fetch
        map.set(normalized, null)
      }
    }
    return map
  }, [data])

  return {
    title: data?.title ?? "",
    currentStage: normalizeStage("product", data?.current_stage ?? "idea"),
    contentByStage,
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

  const meeting = data?.find((m) => m.id === id)

  return {
    title: meeting?.title ?? "",
    currentStage: meeting?.stage ?? "transcript",
    contentByStage: new Map(),
    contentPath: meeting?.path,
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
    currentStage: data?.stage ?? "need",
    contentByStage: new Map(),
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

  const proto = data?.find((p) => p.id === id)

  return {
    title: proto?.title ?? "",
    currentStage: proto?.stage ?? "need",
    contentByStage: new Map(),
    contentPath: proto?.folder_path ? `${proto.folder_path}/spec.md` : undefined,
    isLoading,
    error: error ? String(error) : null,
  }
}

// ---------------------------------------------------------------------------
// Main page component
// ---------------------------------------------------------------------------

export function ItemPipelinePage() {
  const { type = "", id = "" } = useParams<{ type: string; id: string }>()
  const queryClient = useQueryClient()
  const [selectedStage, setSelectedStage] = useState<string | null>(null)
  const [advanceLoading, setAdvanceLoading] = useState(false)

  // Pick the right data hook based on type
  const productData = useProductData(type === "product" ? id : "")
  const meetingData = useMeetingData(type === "meeting" ? id : "")
  const documentData = useDocumentData(type === "document" ? id : "")
  const prototypeData = usePrototypeData(type === "prototype" ? id : "")

  const pipelineData =
    type === "product"
      ? productData
      : type === "meeting"
        ? meetingData
        : type === "document"
          ? documentData
          : prototypeData

  const { title, currentStage, contentByStage, contentPath, isLoading, error } =
    pipelineData

  // For product items, get the artifact paths indexed by normalized stage
  const { data: threadData } = useQuery<ShapeUpThread>({
    queryKey: ["thread", id],
    queryFn: () => api.getThread(id),
    enabled: type === "product" && !!id,
  })

  const artifactPathByStage = useMemo(() => {
    const map = new Map<string, string>()
    if (threadData?.artifacts) {
      for (const a of threadData.artifacts) {
        map.set(normalizeStage("product", a.stage), a.path)
      }
    }
    return map
  }, [threadData])

  const artifactStages = useMemo(
    () => new Set(artifactPathByStage.keys()),
    [artifactPathByStage],
  )

  // Classify stages
  const stages = useMemo(
    () => classifyStages(type, currentStage, type === "product" ? artifactStages : undefined),
    [type, currentStage, artifactStages],
  )

  // Determine which stage to show content for
  const viewStage = selectedStage ?? currentStage

  // Determine the vault file path for the selected stage's content
  const contentFilePath = useMemo(() => {
    if (type === "product") {
      return artifactPathByStage.get(viewStage) ?? null
    }
    return contentPath ?? null
  }, [type, viewStage, artifactPathByStage, contentPath])

  // Fetch the content for the viewed stage
  const { data: stageContent, isLoading: contentLoading } = useQuery<string>({
    queryKey: ["vault-file", contentFilePath],
    queryFn: () => api.getVaultFile(contentFilePath!),
    enabled: !!contentFilePath,
    staleTime: 30_000,
  })

  // Advance mutation
  const advanceMutation = useMutation({
    mutationFn: async (nextStage: string) => {
      if (type === "product") {
        return api.advanceThread(id, { target_stage: nextStage })
      } else if (type === "meeting") {
        return api.setMeetingStage(id, nextStage)
      } else if (type === "document") {
        return api.setDocumentStage(id, nextStage)
      } else {
        return api.setPrototypeStage(id, nextStage)
      }
    },
    onMutate: () => setAdvanceLoading(true),
    onSuccess: () => {
      toast.success(`Advanced to next stage`)
      // Invalidate relevant queries
      if (type === "product") {
        queryClient.invalidateQueries({ queryKey: ["thread", id] })
      } else if (type === "meeting") {
        queryClient.invalidateQueries({ queryKey: ["meetings"] })
      } else if (type === "document") {
        queryClient.invalidateQueries({ queryKey: ["document", id] })
      } else {
        queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      }
      queryClient.invalidateQueries({ queryKey: ["card-detail", type] })
      setSelectedStage(null)
    },
    onError: (err) => {
      toast.error(`Failed to advance: ${err instanceof Error ? err.message : "Unknown error"}`)
    },
    onSettled: () => setAdvanceLoading(false),
  })

  // Next stage computation
  const nextStage = useMemo(() => {
    const order = stageOrders[type] ?? []
    const idx = order.indexOf(currentStage)
    if (idx >= 0 && idx < order.length - 1) return order[idx + 1]
    return null
  }, [type, currentStage])

  if (isLoading) return <SkeletonPage />

  if (error) {
    return (
      <>
        <BackLink type={type} />
        <PageHeader
          title={typeTitles[type] ?? type}
          description="Pipeline item"
        />
        <Card className="rounded-md border-destructive/40 bg-destructive/5">
          <CardContent className="py-6">
            <p className="text-sm text-destructive">
              Could not load item. The backend may still be starting up —
              refresh in a moment.
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
        description={`${typeTitles[type] ?? type} pipeline — currently at ${stageLabel(currentStage)}`}
      />

      {/* Stage flow */}
      <div className="overflow-x-auto pb-2">
        <div className="flex items-center gap-0 min-w-max">
          {stages.map((s, idx) => (
            <div key={s.stage} className="flex items-center">
              <StageNode
                stage={s.stage}
                status={s.status}
                color={stageColor(type, s.stage)}
                isSelected={viewStage === s.stage}
                onClick={() => {
                  if (s.status === "completed" || s.status === "current") {
                    setSelectedStage(s.stage)
                  }
                }}
              />
              {idx < stages.length - 1 && (
                <StageConnector
                  completed={
                    s.status === "completed" ||
                    (s.status === "current" && stages[idx + 1]?.status === "current")
                  }
                />
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Content area */}
      <Card className="rounded-[14px] border-border shadow-stripe">
        <CardContent className="p-6">
          <div className="flex items-center gap-2 mb-4">
            <span
              className="inline-block h-2.5 w-2.5 rounded-full flex-shrink-0"
              style={{ background: stageColor(type, viewStage) }}
            />
            <h3 className="text-sm font-semibold text-foreground">
              {stageLabel(viewStage)}
            </h3>
            {viewStage === currentStage && (
              <span className="text-[11px] text-muted-foreground bg-muted px-2 py-0.5 rounded-full">
                Current
              </span>
            )}
          </div>

          {contentLoading ? (
            <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading content...
            </div>
          ) : stageContent ? (
            <pre className="whitespace-pre-wrap text-sm text-foreground/90 font-mono leading-relaxed max-h-[600px] overflow-y-auto">
              {stageContent}
            </pre>
          ) : (
            <p className="text-sm text-muted-foreground py-8 text-center">
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
            className="gap-2"
            style={{
              background: stageColor(type, nextStage),
              color: "white",
            }}
          >
            {advanceLoading ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Advancing...
              </>
            ) : (
              <>Advance to {stageLabel(nextStage)} &rarr;</>
            )}
          </Button>
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Stage node component
// ---------------------------------------------------------------------------

function StageNode({
  stage,
  status,
  color,
  isSelected,
  onClick,
}: {
  stage: string
  status: StageStatus
  color: string
  isSelected: boolean
  onClick: () => void
}) {
  const isClickable = status === "completed" || status === "current"

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!isClickable}
      className={cn(
        "relative flex flex-col items-center gap-1.5 rounded-xl border-2 px-5 py-3 transition-all min-w-[110px]",
        isClickable && "cursor-pointer hover:scale-[1.03]",
        !isClickable && "cursor-default opacity-40",
        isSelected && "ring-2 ring-offset-2 ring-offset-background",
      )}
      style={{
        borderColor: status === "future" ? "var(--border)" : color,
        background:
          status === "future"
            ? "var(--card)"
            : `color-mix(in oklch, ${color} 12%, transparent)`,
        ...(isSelected ? { ringColor: color } as React.CSSProperties : {}),
        // Use boxShadow for the ring since CSS custom ring-color with oklch is tricky
        ...(isSelected
          ? { boxShadow: `0 0 0 2px var(--background), 0 0 0 4px ${color}` }
          : {}),
      }}
    >
      {/* Status icon */}
      <div
        className="flex items-center justify-center h-6 w-6 rounded-full"
        style={{
          background:
            status === "completed"
              ? color
              : status === "current"
                ? `color-mix(in oklch, ${color} 25%, transparent)`
                : "var(--muted)",
        }}
      >
        {status === "completed" ? (
          <Check className="h-3.5 w-3.5 text-white" />
        ) : status === "current" ? (
          <Circle className="h-3 w-3" style={{ color, fill: color }} />
        ) : (
          <Circle className="h-3 w-3 text-muted-foreground/50" />
        )}
      </div>

      {/* Stage label */}
      <span
        className={cn(
          "text-[12px] font-medium",
          status === "future" ? "text-muted-foreground/60" : "text-foreground",
        )}
      >
        {stageLabel(stage)}
      </span>
    </button>
  )
}

// ---------------------------------------------------------------------------
// Stage connector (SVG arrow)
// ---------------------------------------------------------------------------

function StageConnector({ completed }: { completed: boolean }) {
  return (
    <svg
      width="48"
      height="24"
      viewBox="0 0 48 24"
      fill="none"
      className="flex-shrink-0"
    >
      <line
        x1="4"
        y1="12"
        x2="36"
        y2="12"
        stroke={completed ? "var(--foreground)" : "var(--border)"}
        strokeWidth="2"
        strokeDasharray={completed ? "none" : "4 3"}
        className={cn(
          !completed && "animate-pulse",
        )}
      />
      {/* Arrowhead */}
      <path
        d="M34 7 L42 12 L34 17"
        stroke={completed ? "var(--foreground)" : "var(--border)"}
        strokeWidth="2"
        fill="none"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
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
