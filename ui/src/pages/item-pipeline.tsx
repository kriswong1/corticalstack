import { useState, useMemo, useRef } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useParams } from "react-router-dom"
import Markdown from "react-markdown"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { PageHeader } from "@/components/layout/page-header"
import { SkeletonPage } from "@/components/shared/skeleton-card"
import { QuestionsModal } from "@/components/questions-modal"
import { ProductSubnav } from "@/components/product-subnav"
import { DownstreamArtifacts } from "@/components/downstream-artifacts"
import { api, getErrorMessage } from "@/lib/api"
import { cn } from "@/lib/utils"
import { Breadcrumbs } from "@/components/layout/breadcrumbs"
import {
  PIPELINE_ACCENT,
  colorFor,
  normalizeStage as normalizeStageShared,
  routeFor,
  stageLabel as sharedStageLabel,
  stageOrders,
  withAlpha,
} from "@/lib/pipeline-stages"
import { toast } from "sonner"
import { Check, Circle, Loader2, ArrowRight, RefreshCw, RotateCcw } from "lucide-react"
import type { ShapeUpThread, Meeting, Prototype, Document, Question, Answer } from "@/types/api"

// isAudioSource recognises file extensions that ingest sends through
// Deepgram. Meetings whose source_path has one of these extensions
// originated from an audio file and therefore passed through an
// implicit "audio" stage before reaching transcript.
const audioExtensions = [".mp3", ".m4a", ".wav", ".flac", ".ogg", ".webm"]
function isAudioSource(sourcePath?: string): boolean {
  if (!sourcePath) return false
  const lower = sourcePath.toLowerCase()
  return audioExtensions.some((ext) => lower.endsWith(ext))
}

// Thin aliases so existing call sites inside this file stay readable.
const label = sharedStageLabel
const normalizeStage = normalizeStageShared

const typeTitles: Record<string, string> = {
  product: "Product", meeting: "Meeting",
  document: "Document", prototype: "Prototype",
}

// sectionTitles are what the breadcrumb trail calls each pipeline
// listing. They match the sidebar labels so users see consistent
// naming across navigation surfaces.
const sectionTitles: Record<string, string> = {
  product: "Threads",
  meeting: "Meetings",
  document: "Documents",
  prototype: "Prototypes",
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

type StageStatus = "completed" | "current" | "future" | "skipped"

interface StageInfo {
  stage: string
  status: StageStatus
  date?: string
}

function classifyStages(
  type: string,
  currentStage: string,
  artifactDates?: Map<string, string>,
  opts?: { audioSourced?: boolean },
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
    // Meeting: Audio stage is a UX-level stage inferred from the source
    // file, not a real stage the backend transitions through. For
    // text-sourced meetings it's marked "skipped" so the pipeline shape
    // stays 3 wide without claiming the meeting passed through Deepgram.
    if (type === "meeting" && s === "audio" && !opts?.audioSourced) {
      status = "skipped"
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
  hasHTML?: boolean
  prototypeId?: string
  prototype?: Prototype
  // Meeting-specific: set when the meeting originated from an audio
  // file so the pipeline view can mark Audio as completed vs. skipped.
  audioSourced?: boolean
  audioSourcePath?: string
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
    audioSourced: isAudioSource(m?.source_path),
    audioSourcePath: isAudioSource(m?.source_path) ? m?.source_path : undefined,
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
    hasHTML: p?.has_html,
    prototypeId: p?.id,
    prototype: p,
    isLoading,
    error: error ? String(error) : null,
  }
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

interface ItemPipelinePageProps {
  /** Passed from the router so each type's URL can be flat
      (`/product/:id`, `/meetings/:id`, ...) instead of carrying the
      type in the path. Legacy URLs still supply it via params. */
  type?: string
}

export function ItemPipelinePage({ type: typeProp }: ItemPipelinePageProps = {}) {
  const params = useParams<{ type: string; id: string }>()
  const type = typeProp ?? params.type ?? ""
  const id = params.id ?? ""
  const queryClient = useQueryClient()
  const [selectedStage, setSelectedStage] = useState<string | null>(null)
  const downstreamRef = useRef<HTMLDivElement | null>(null)

  const scrollToDownstream = () => {
    downstreamRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })
  }

  const productData = useProductData(type === "product" ? id : "")
  const meetingData = useMeetingData(type === "meeting" ? id : "")
  const documentData = useDocumentData(type === "document" ? id : "")
  const prototypeData = usePrototypeData(type === "prototype" ? id : "")

  const pipelineData =
    type === "product" ? productData
    : type === "meeting" ? meetingData
    : type === "document" ? documentData
    : prototypeData

  const { title, currentStage, contentPath, hasHTML, prototypeId, prototype: protoObj, audioSourced, audioSourcePath, isLoading, error } = pipelineData
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
      type === "meeting" ? { audioSourced } : undefined,
    ),
    [type, currentStage, artifactDateByStage, audioSourced],
  )

  const viewStage = selectedStage ?? currentStage

  const contentFilePath = useMemo(() => {
    if (type === "product") return artifactPathByStage.get(viewStage) ?? null
    // Meeting audio stage has no vault content — the original audio
    // file is external and replaced by the transcript further down the
    // pipeline. The content area renders a dedicated info panel instead.
    if (type === "meeting" && viewStage === "audio") return null
    return contentPath ?? null
  }, [type, viewStage, artifactPathByStage, contentPath])

  const { data: rawContent, isLoading: contentLoading } = useQuery<string>({
    queryKey: ["vault-file", contentFilePath],
    queryFn: () => api.getVaultFile(contentFilePath!),
    enabled: !!contentFilePath,
    staleTime: 30_000,
  })

  // Viewing an archived prototype version substitutes the spec body
  // in the content panel. The archived spec comes in raw (with
  // frontmatter), so strip it the same way the live vault file is
  // handled so both render identically.
  const stageContent = (() => {
    if (type === "prototype" && selectedVersion !== null && archivedSpec) {
      return stripFrontmatter(archivedSpec)
    }
    return rawContent ? stripFrontmatter(rawContent) : null
  })()

  // Advance — Q&A flow for product, direct stage set for others
  const [hints, setHints] = useState("")
  const [qaModalOpen, setQaModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)

  const nextStage = useMemo(() => {
    const order = stageOrders[type] ?? []
    const idx = order.indexOf(currentStage)
    if (idx >= 0 && idx < order.length - 1) return order[idx + 1]
    return null
  }, [type, currentStage])

  // Product: ask Claude for clarifying questions before advancing
  const questionsMutation = useMutation({
    mutationFn: () => api.shapeupQuestions(id, nextStage!),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch questions: ${getErrorMessage(err)}`)
    },
  })

  // Product: advance with optional answers
  const advanceProductMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.advanceThread(id, {
        target_stage: nextStage!,
        hints: hints || undefined,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: () => {
      toast.success(`Advanced to ${label(nextStage!)}`)
      setHints("")
      setQuestions(null)
      setQaModalOpen(false)
      queryClient.invalidateQueries({ queryKey: ["thread", id] })
      queryClient.invalidateQueries({ queryKey: ["card-detail", type] })
      queryClient.invalidateQueries({ queryKey: ["dashboard"] })
      setSelectedStage(null)
    },
    onError: (err) => {
      setQuestions(null)
      setQaModalOpen(false)
      toast.error(`Advance failed: ${getErrorMessage(err)}`)
    },
  })

  // Non-product: simple stage transition
  const advanceStageMutation = useMutation({
    mutationFn: async () => {
      if (type === "meeting") return api.setMeetingStage(id, nextStage!)
      if (type === "document") return api.setDocumentStage(id, nextStage!)
      return api.setPrototypeStage(id, nextStage!)
    },
    onSuccess: () => {
      toast.success(`Advanced to ${label(nextStage!)}`)
      if (type === "meeting") queryClient.invalidateQueries({ queryKey: ["meetings"] })
      else if (type === "document") queryClient.invalidateQueries({ queryKey: ["document", id] })
      else queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      queryClient.invalidateQueries({ queryKey: ["card-detail", type] })
      queryClient.invalidateQueries({ queryKey: ["dashboard"] })
      setSelectedStage(null)
    },
    onError: (err) => toast.error(`Advance failed: ${getErrorMessage(err)}`),
  })

  // Iterate: re-run current stage with existing content as extra context.
  // Claude gets questions to refine the current draft rather than starting fresh.
  const iterateMutation = useMutation({
    mutationFn: () => api.shapeupQuestions(id, currentStage),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch iteration questions: ${getErrorMessage(err)}`)
    },
  })

  // Re-generate: advance to same stage from previous source, discarding current draft.
  const regenerateMutation = useMutation({
    mutationFn: () => api.shapeupQuestions(id, currentStage),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch questions: ${getErrorMessage(err)}`)
    },
  })

  // Shared advance for iterate/regenerate — re-targets the CURRENT stage
  const redoMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.advanceThread(id, {
        target_stage: currentStage,
        hints: hints || undefined,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: () => {
      toast.success(`Re-generated ${label(currentStage)}`)
      setHints("")
      setQuestions(null)
      setQaModalOpen(false)
      queryClient.invalidateQueries({ queryKey: ["thread", id] })
      queryClient.invalidateQueries({ queryKey: ["vault-file"] })
      queryClient.invalidateQueries({ queryKey: ["card-detail", type] })
      setSelectedStage(null)
    },
    onError: (err) => {
      setQuestions(null)
      setQaModalOpen(false)
      toast.error(`Re-generate failed: ${getErrorMessage(err)}`)
    },
  })

  // Prototype: questions before regeneration
  const protoQuestionsMutation = useMutation({
    mutationFn: () => {
      if (!protoObj) throw new Error("no prototype")
      return api.prototypeQuestions({
        title: protoObj.title,
        format: protoObj.format,
        source_paths: protoObj.source_refs ?? [],
      })
    },
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch questions: ${getErrorMessage(err)}`)
    },
  })

  // Prototype: refine — archives current version to versions/v{n}/
  // and synthesizes a new one in its place (bumping Version).
  const protoRefineMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.refinePrototype(protoObj!.id, {
        hints: hints || undefined,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: (updated) => {
      toast.success(`Refined to v${updated.version}`)
      setHints("")
      setQuestions(null)
      setQaModalOpen(false)
      queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      queryClient.invalidateQueries({ queryKey: ["card-detail", type] })
      queryClient.invalidateQueries({ queryKey: ["vault-file"] })
      queryClient.invalidateQueries({ queryKey: ["proto-versions", id] })
    },
    onError: (err) => {
      setQuestions(null)
      setQaModalOpen(false)
      toast.error(`Refine failed: ${getErrorMessage(err)}`)
    },
  })

  const isAdvancing = advanceProductMutation.isPending || advanceStageMutation.isPending || questionsMutation.isPending || redoMutation.isPending
  const isRedoing = iterateMutation.isPending || regenerateMutation.isPending || redoMutation.isPending
  const isProtoRefining = protoQuestionsMutation.isPending || protoRefineMutation.isPending

  // Poll advance progress while a product advance is in flight.
  // Returns {turn, max_turns, status, stage} or {status:"idle"}.
  const { data: advanceProgress } = useQuery({
    queryKey: ["advance-progress", id],
    queryFn: () => api.getAdvanceProgress(id),
    enabled: type === "product" && (advanceProductMutation.isPending || redoMutation.isPending),
    refetchInterval: 2000,
  })
  const progressTurn = advanceProgress?.turn ?? 0
  const progressMax = advanceProgress?.max_turns ?? 10

  // Prototype: past versions list — drives the version switcher and
  // the refine header count. Enabled only when viewing a prototype
  // item so the query doesn't fire on other pipeline types.
  const { data: protoVersions } = useQuery({
    queryKey: ["proto-versions", id],
    queryFn: () => api.listPrototypeVersions(id),
    enabled: type === "prototype" && !!id,
  })

  // When viewing an older prototype version, selectedVersion holds
  // the version number. null means "current live version".
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null)

  // Fetch the selected archived version's spec when the user picks
  // one from the switcher. Current version (null) comes from the
  // normal vault-file query above.
  const { data: archivedSpec } = useQuery({
    queryKey: ["proto-version-spec", id, selectedVersion],
    queryFn: () => api.getPrototypeVersionSpec(id, selectedVersion!),
    enabled: type === "prototype" && selectedVersion !== null,
  })

  function startProductAdvance() {
    if (!nextStage) return
    setQuestions(null)
    setQaModalOpen(true)
    questionsMutation.mutate()
  }

  if (isLoading) return <SkeletonPage />

  if (error) {
    return (
      <>
        <Breadcrumbs
          items={[
            { label: "Dashboard", to: "/dashboard" },
            { label: sectionTitles[type] ?? typeTitles[type] ?? type, to: routeFor(type) },
            { label: "Error" },
          ]}
        />
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
      {/* Product-surface items (threads + prototypes) show the shared
          subnav so the four unified views are reachable from anywhere
          inside the section. */}
      {(type === "product" || type === "prototype") && <ProductSubnav />}
      <Breadcrumbs
        items={[
          { label: "Dashboard", to: "/dashboard" },
          {
            label: sectionTitles[type] ?? typeTitles[type] ?? type,
            to: routeFor(type),
          },
          { label: title || "Untitled" },
        ]}
      />

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
              {stages.map((s, idx) => {
                // Contextual action badge on trigger stages (Phase 3):
                // Breadboard unlocks Prototype, Pitch unlocks PRD. Only
                // show once the stage has been reached (not a future
                // stage) so the affordance matches what's possible now.
                const showAction = type === "product" && s.status !== "future"
                const action =
                  showAction && s.stage === "breadboard"
                    ? { label: "+ Prototype" }
                    : showAction && s.stage === "pitch"
                      ? { label: "+ PRD" }
                      : null
                return (
                  <div key={s.stage} className="flex items-start">
                    <StageNode
                      stage={s.stage}
                      status={s.status}
                      date={s.date}
                      color={colorFor(type, s.stage)}
                      accent={accent}
                      isSelected={viewStage === s.stage}
                      isGenerating={isAdvancing && s.stage === nextStage}
                      progressTurn={isAdvancing && s.stage === nextStage ? progressTurn : undefined}
                      progressMax={isAdvancing && s.stage === nextStage ? progressMax : undefined}
                      action={action}
                      onActionClick={scrollToDownstream}
                      onClick={() => {
                        if (s.status !== "future" && s.status !== "skipped") {
                          setSelectedStage(s.stage)
                        }
                      }}
                    />
                    {idx < stages.length - 1 && (
                      <Connector
                        active={s.status === "completed"}
                        color={accent}
                      />
                    )}
                  </div>
                )
              })}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Prototype refine panel — primary interaction for iterating
          on a prototype. Lives above the advance card so the prompt
          box is visible immediately, not buried below the content. */}
      {type === "prototype" && protoObj && currentStage !== "final" && (
        <Card
          className="rounded-[14px] border-border shadow-stripe"
          style={{ borderColor: withAlpha(accent, 0.25) }}
        >
          <CardContent className="py-5 px-5 space-y-3">
            <div className="flex items-center gap-3 flex-wrap">
              <div className="flex items-center gap-2">
                <RefreshCw className="h-4 w-4" style={{ color: accent }} />
                <span className="text-[13px] font-semibold text-foreground">
                  Refine this prototype
                </span>
                <span
                  className="text-[10px] font-bold tracking-[0.06em] uppercase px-2 py-0.5 rounded-full tabular-nums"
                  style={{
                    background: withAlpha(accent, 0.15),
                    color: accent,
                    border: `1px solid ${withAlpha(accent, 0.3)}`,
                  }}
                >
                  Currently v{protoObj.version}
                </span>
              </div>
              {(protoVersions?.length ?? 0) > 0 && (
                <span className="text-[11px] text-muted-foreground">
                  {protoVersions!.length} past version
                  {protoVersions!.length === 1 ? "" : "s"} archived
                </span>
              )}
            </div>

            <div className="flex items-start gap-3 flex-wrap">
              <Input
                value={hints}
                onChange={(e) => setHints(e.target.value)}
                placeholder="Describe what to change (e.g. make the header larger, add a pricing section)"
                className="flex-1 min-w-[280px] h-9 border-border rounded-sm"
                disabled={isProtoRefining}
              />
              <Button
                onClick={() => {
                  setQuestions(null)
                  setQaModalOpen(true)
                  protoQuestionsMutation.mutate()
                }}
                disabled={isProtoRefining || !hints.trim()}
                className="gap-2 rounded-lg font-semibold text-[12px] px-4 h-9"
                style={{
                  background: accent,
                  color: "white",
                  boxShadow: `0 2px 8px ${withAlpha(accent, 0.35)}`,
                }}
              >
                {isProtoRefining ? (
                  <>
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    Refining...
                  </>
                ) : (
                  <>Create v{protoObj.version + 1}</>
                )}
              </Button>
            </div>

            {/* Version switcher — visible when past versions exist.
                Lets the user inspect old iterations without affecting
                the live version. */}
            {(protoVersions?.length ?? 0) > 0 && (
              <div className="flex items-center gap-2 flex-wrap pt-2 border-t border-border">
                <span className="text-[11px] text-muted-foreground mr-1">
                  View:
                </span>
                <button
                  type="button"
                  onClick={() => setSelectedVersion(null)}
                  className={cn(
                    "text-[11px] font-medium px-2.5 py-1 rounded-md border transition-colors",
                    selectedVersion === null
                      ? "text-foreground"
                      : "text-muted-foreground hover:text-foreground border-transparent hover:border-border",
                  )}
                  style={
                    selectedVersion === null
                      ? {
                          background: withAlpha(accent, 0.12),
                          borderColor: withAlpha(accent, 0.4),
                        }
                      : undefined
                  }
                >
                  v{protoObj.version} (current)
                </button>
                {protoVersions!
                  .slice()
                  .reverse()
                  .map((v) => (
                    <button
                      key={v.version}
                      type="button"
                      onClick={() => setSelectedVersion(v.version)}
                      title={v.hints ? `Refinement prompt: ${v.hints}` : undefined}
                      className={cn(
                        "text-[11px] font-medium px-2.5 py-1 rounded-md border transition-colors",
                        selectedVersion === v.version
                          ? "text-foreground"
                          : "text-muted-foreground hover:text-foreground border-transparent hover:border-border",
                      )}
                      style={
                        selectedVersion === v.version
                          ? {
                              background: withAlpha(accent, 0.12),
                              borderColor: withAlpha(accent, 0.4),
                            }
                          : undefined
                      }
                    >
                      v{v.version}
                    </button>
                  ))}
                {selectedVersion !== null && (
                  <span className="text-[11px] text-muted-foreground italic ml-auto">
                    Viewing archived v{selectedVersion} — read only
                  </span>
                )}
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Advance card — positioned above content for visibility */}
      {nextStage && (
        <Card
          className="rounded-[14px] border-border shadow-stripe"
          style={{ borderColor: withAlpha(colorFor(type, nextStage), 0.2) }}
        >
          <CardContent className="py-4 px-5">
            <div className="flex items-center gap-4 flex-wrap">
              <div className="flex items-center gap-2 flex-1 min-w-0">
                <ArrowRight
                  className="h-4 w-4 flex-shrink-0"
                  style={{ color: colorFor(type, nextStage) }}
                />
                <span className="text-[13px] font-semibold text-foreground">
                  {type === "prototype" && nextStage === "final"
                    ? "Finalize prototype"
                    : `Advance to ${label(nextStage)}`}
                </span>
                {type === "product" && (
                  <span className="text-[11px] text-muted-foreground">
                    — Claude will generate the {label(nextStage).toLowerCase()} draft
                  </span>
                )}
                {type === "prototype" && nextStage === "final" && (
                  <span className="text-[11px] text-muted-foreground">
                    — locks the current version as the final deliverable
                  </span>
                )}
              </div>

              {type === "product" && (
                <Input
                  value={hints}
                  onChange={(e) => setHints(e.target.value)}
                  placeholder="Optional hints for Claude..."
                  className="h-8 text-xs border-border rounded-sm max-w-[240px]"
                />
              )}

              <Button
                onClick={type === "product" ? startProductAdvance : () => advanceStageMutation.mutate()}
                disabled={isAdvancing}
                className="gap-2 rounded-lg font-semibold text-[12px] px-4 py-2 h-8"
                style={{
                  background: colorFor(type, nextStage),
                  color: "white",
                  boxShadow: `0 2px 8px ${withAlpha(colorFor(type, nextStage), 0.35)}`,
                }}
              >
                {isAdvancing ? (
                  <>
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    {type === "product" ? "Working..." : "Advancing..."}
                  </>
                ) : type === "prototype" && nextStage === "final" ? (
                  <>Finalize</>
                ) : (
                  <>Advance</>
                )}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Q&A modal for product advance / iterate / regenerate */}
      {type === "product" && (
        <QuestionsModal
          open={qaModalOpen}
          onOpenChange={(next) => {
            if (!next && !advanceProductMutation.isPending && !redoMutation.isPending) {
              setQaModalOpen(false)
              setQuestions(null)
            }
          }}
          title={
            isRedoing
              ? `Re-generate ${label(currentStage)}`
              : nextStage
                ? `Advance to ${label(nextStage)}`
                : `Refine ${label(currentStage)}`
          }
          description={
            isRedoing
              ? "Answer these to refine the current stage. Or skip to regenerate immediately."
              : "Answer these so Claude can produce a better draft. Or skip to generate immediately."
          }
          questions={questions}
          loading={questionsMutation.isPending || iterateMutation.isPending || regenerateMutation.isPending}
          submitting={advanceProductMutation.isPending || redoMutation.isPending}
          onSubmit={(answers) =>
            isRedoing
              ? redoMutation.mutate(answers)
              : advanceProductMutation.mutate(answers)
          }
          onSkip={() =>
            isRedoing
              ? redoMutation.mutate([])
              : advanceProductMutation.mutate([])
          }
        />
      )}

      {/* Q&A modal for prototype refinement */}
      {type === "prototype" && (
        <QuestionsModal
          open={qaModalOpen}
          onOpenChange={(next) => {
            if (!next && !protoRefineMutation.isPending) {
              setQaModalOpen(false)
              setQuestions(null)
            }
          }}
          title={`Refine to v${(protoObj?.version ?? 1) + 1}`}
          description="Answer these so Claude can apply your refinement accurately. Or skip to refine immediately."
          questions={questions}
          loading={protoQuestionsMutation.isPending}
          submitting={protoRefineMutation.isPending}
          onSubmit={(answers) => protoRefineMutation.mutate(answers)}
          onSkip={() => protoRefineMutation.mutate([])}
        />
      )}

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
          ) : type === "meeting" && viewStage === "audio" ? (
            <AudioStagePanel
              audioSourced={!!audioSourced}
              sourcePath={audioSourcePath}
            />
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

          {/* Prototype HTML preview. When the user is inspecting an
              archived version via the switcher, swap the iframe src to
              the version-specific HTML so preview and spec stay in
              sync. */}
          {type === "prototype" && hasHTML && prototypeId && (
            <div className="mt-6 pt-4 border-t border-border">
              <h4 className="text-[13px] font-semibold text-foreground mb-3">
                HTML Preview
                {selectedVersion !== null && (
                  <span className="ml-2 text-[11px] font-normal text-muted-foreground">
                    (v{selectedVersion})
                  </span>
                )}
              </h4>
              <iframe
                src={
                  selectedVersion !== null
                    ? api.prototypeVersionHTMLUrl(prototypeId, selectedVersion)
                    : api.prototypeHTMLUrl(prototypeId)
                }
                sandbox="allow-scripts"
                title="Prototype preview"
                className="w-full rounded-lg border border-border bg-white"
                style={{ height: "600px" }}
              />
            </div>
          )}


          {/* Iterate / Re-generate actions for current stage (product only) */}
          {type === "product" && viewStage === currentStage && currentStage !== "idea" && stageContent && (
            <div className="mt-6 pt-4 border-t border-border flex items-center gap-3">
              <span className="text-[11px] text-muted-foreground mr-auto">
                Not happy with this draft?
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  // Iterate: re-advance to the SAME stage with the current
                  // content as additional context. Claude asks new questions
                  // to refine the existing draft rather than starting from
                  // scratch.
                  setQuestions(null)
                  setQaModalOpen(true)
                  // Use the current stage as target (re-run same stage)
                  iterateMutation.mutate()
                }}
                disabled={isAdvancing || iterateMutation.isPending}
                className="gap-1.5 text-[12px] h-8 rounded-lg"
              >
                {iterateMutation.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RefreshCw className="h-3.5 w-3.5" />
                )}
                Iterate
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  // Re-generate: advance to the same stage from the
                  // PREVIOUS stage's artifact, discarding the current
                  // draft entirely.
                  setQuestions(null)
                  setQaModalOpen(true)
                  regenerateMutation.mutate()
                }}
                disabled={isAdvancing || regenerateMutation.isPending}
                className="gap-1.5 text-[12px] h-8 rounded-lg text-destructive border-destructive/30 hover:bg-destructive/10"
              >
                {regenerateMutation.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RotateCcw className="h-3.5 w-3.5" />
                )}
                Re-generate
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Downstream artifacts — prototypes, PRDs, use cases generated from
          this product thread. Only renders for the product pipeline and
          only shows sections once their trigger stage has been reached. */}
      {type === "product" && threadData && (
        <DownstreamArtifacts thread={threadData} sectionRef={downstreamRef} />
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
  isGenerating,
  progressTurn,
  progressMax,
  action,
  onActionClick,
  onClick,
}: {
  stage: string
  status: StageStatus
  date?: string
  color: string
  accent: string
  isSelected: boolean
  isGenerating?: boolean
  progressTurn?: number
  progressMax?: number
  action?: { label: string } | null
  onActionClick?: () => void
  onClick: () => void
}) {
  const isSkipped = status === "skipped"
  const isClickable = status !== "future" && !isSkipped && !isGenerating
  const isFuture = status === "future" && !isGenerating
  const isCompleted = status === "completed"
  const isCurrent = status === "current"

  return (
    <div className="flex flex-col items-center gap-1.5">
    <button
      type="button"
      onClick={onClick}
      disabled={!isClickable}
      className={cn(
        "relative flex flex-col items-center gap-2 rounded-xl border-2 px-6 py-4 transition-all min-w-[130px]",
        isClickable && "cursor-pointer hover:scale-[1.02] hover:shadow-md",
        !isClickable && !isGenerating && "cursor-default",
        isGenerating && "cursor-wait",
      )}
      style={{
        borderColor: isGenerating
          ? color
          : isSkipped
            ? "var(--border)"
            : isFuture ? "var(--border)" : isSelected ? color : withAlpha(color, 0.35),
        background: isGenerating
          ? withAlpha(color, 0.08)
          : isSkipped
            ? "transparent"
            : isFuture
              ? "var(--card)"
              : isSelected
                ? withAlpha(color, 0.1)
                : withAlpha(color, 0.05),
        opacity: isFuture ? 0.45 : isSkipped ? 0.35 : 1,
        boxShadow: isGenerating
          ? `0 0 0 2px var(--background), 0 0 0 4px ${withAlpha(color, 0.4)}`
          : isSelected
            ? `0 0 0 2px var(--background), 0 0 0 4px ${withAlpha(color, 0.5)}`
            : "none",
        animation: isGenerating ? "pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite" : "none",
        borderStyle: isSkipped ? "dashed" : "solid",
      }}
    >
      {/* Status icon */}
      <div
        className="flex items-center justify-center h-8 w-8 rounded-full transition-colors"
        style={{
          background: isGenerating
            ? withAlpha(color, 0.25)
            : isCompleted
              ? color
              : isCurrent
                ? withAlpha(accent, 0.2)
                : withAlpha("#8B8FA3", 0.12),
          border: isGenerating
            ? `2px solid ${color}`
            : isCurrent ? `2px solid ${accent}` : "none",
        }}
      >
        {isGenerating ? (
          <Loader2 className="h-4 w-4 animate-spin" style={{ color }} />
        ) : isCompleted ? (
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
          isGenerating ? "text-foreground" : isFuture ? "text-muted-foreground/50" : "text-foreground",
        )}
      >
        {label(stage)}
      </span>

      {/* Date or generating indicator */}
      {isGenerating ? (
        <span className="text-[10px] font-medium tabular-nums" style={{ color }}>
          {progressTurn && progressMax
            ? `Turn ${progressTurn} of ${progressMax}`
            : "Starting..."}
        </span>
      ) : date ? (
        <span className="text-[10px] tabular-nums text-muted-foreground">
          {formatShortDate(date)}
        </span>
      ) : (
        <span className="text-[10px] text-muted-foreground/40 italic">
          {isSkipped ? "Skipped" : isFuture ? "Pending" : "—"}
        </span>
      )}
    </button>

      {action && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onActionClick?.()
          }}
          className="text-[10px] font-medium tracking-wide text-muted-foreground hover:text-primary transition-colors rounded-sm px-1.5 py-0.5 border border-dashed border-border hover:border-primary/50"
          title={`Jump to ${action.label.replace("+ ", "")} section`}
        >
          {action.label}
        </button>
      )}
    </div>
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
// Audio stage panel (meetings only)
// ---------------------------------------------------------------------------

// AudioStagePanel renders the content body when the user is viewing the
// synthetic "Audio" stage on a meeting. There's no vault file behind it
// — the audio is an external source that gets consumed into the
// transcript — so we show the source path and a short explanation of
// what this stage means. For text-sourced meetings the panel explains
// why this stage is skipped.
function AudioStagePanel({
  audioSourced,
  sourcePath,
}: {
  audioSourced: boolean
  sourcePath?: string
}) {
  if (!audioSourced) {
    return (
      <div className="py-8 text-center">
        <p className="text-sm text-muted-foreground italic">
          This meeting was created from text — no audio stage.
        </p>
        <p className="mt-1 text-[11px] text-muted-foreground/70">
          Meetings that start from an audio file pass through Deepgram
          before reaching the transcript stage.
        </p>
      </div>
    )
  }
  return (
    <div className="rounded-md border border-border bg-muted/20 p-4">
      <p className="text-[11px] font-bold tracking-[0.06em] uppercase text-muted-foreground">
        Source audio
      </p>
      {sourcePath ? (
        <p className="mt-1.5 break-all text-[13px] font-mono text-foreground">
          {sourcePath}
        </p>
      ) : (
        <p className="mt-1.5 text-[13px] italic text-muted-foreground">
          Source path not recorded for this meeting.
        </p>
      )}
      <p className="mt-3 text-[11px] text-muted-foreground/80">
        Transcribed by Deepgram into the transcript stage. The audio file
        itself lives outside the vault.
      </p>
    </div>
  )
}
