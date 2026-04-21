import { useEffect, useMemo, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { Link } from "react-router-dom"
import Markdown from "react-markdown"
import { toast } from "sonner"
import { Card, CardContent } from "@/components/ui/card"
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
import { QuestionsModal } from "@/components/questions-modal"
import { api, getErrorMessage } from "@/lib/api"
import { cn } from "@/lib/utils"
import {
  Box,
  FileCheck,
  FileText,
  Plus,
  ExternalLink,
  Loader2,
  ChevronDown,
  ChevronRight,
  RotateCcw,
  CheckCircle2,
} from "lucide-react"
import type {
  Answer,
  PRD,
  Prototype,
  Question,
  ShapeUpThread,
  UseCase,
} from "@/types/api"

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const protoFormats = [
  "interactive-html",
  "screen-flow",
  "component-spec",
  "user-journey",
]

const prdStatusColors: Record<string, string> = {
  draft: "bg-muted text-muted-foreground",
  review: "bg-secondary text-secondary-foreground",
  approved:
    "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)]",
  shipped: "bg-primary/20 text-primary",
  archived: "bg-muted text-muted-foreground",
}

function formatDate(iso?: string): string {
  if (!iso) return ""
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ""
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" })
}

// stripFrontmatter removes a leading YAML frontmatter block from a
// markdown string so the preview shows the body only. Mirrors the
// helper in item-pipeline.tsx.
function stripFrontmatter(raw: string): string {
  const trimmed = raw.trimStart()
  if (!trimmed.startsWith("---")) return raw
  const end = trimmed.indexOf("---", 3)
  if (end < 0) return raw
  return trimmed.slice(end + 3).trimStart()
}

// sourcesFromThread lists the artifact paths (excluding the raw idea)
// that Claude uses as context when generating a prototype from a thread.
function sourcesFromThread(t: ShapeUpThread): string[] {
  return t.artifacts
    .filter((a) => a.stage !== "raw" && a.path)
    .map((a) => a.path)
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function DownstreamArtifacts({
  thread,
  sectionRef,
}: {
  thread: ShapeUpThread
  sectionRef?: React.Ref<HTMLDivElement>
}) {
  const { data: prototypes } = useQuery<Prototype[]>({
    queryKey: ["prototypes"],
    queryFn: api.listPrototypes,
  })
  const { data: prds } = useQuery<PRD[]>({
    queryKey: ["prds"],
    queryFn: api.listPRDs,
  })
  const { data: useCases } = useQuery<UseCase[]>({
    queryKey: ["usecases"],
    queryFn: api.listUseCases,
  })

  const threadPrototypes = useMemo(
    () => (prototypes ?? []).filter((p) => p.source_thread === thread.id),
    [prototypes, thread.id],
  )

  const threadPRDs = useMemo(
    () => (prds ?? []).filter((p) => p.source_thread === thread.id),
    [prds, thread.id],
  )

  // A use case is linked to this thread if any of its source refs points
  // at one of the thread's artifacts OR one of the thread's PRDs. This
  // avoids a schema change — the `source[].path` already records the
  // document path the use case was derived from.
  const threadPRDPaths = useMemo(
    () =>
      new Set(
        threadPRDs
          .map((p) => p.path)
          .filter((p): p is string => !!p),
      ),
    [threadPRDs],
  )
  const threadArtifactPaths = useMemo(
    () => new Set(thread.artifacts.map((a) => a.path).filter(Boolean)),
    [thread.artifacts],
  )
  const threadUseCases = useMemo(() => {
    if (!useCases) return []
    const targets = new Set<string>([...threadPRDPaths, ...threadArtifactPaths])
    return useCases.filter((uc) =>
      uc.source?.some((s) => s.path && targets.has(s.path)),
    )
  }, [useCases, threadPRDPaths, threadArtifactPaths])

  const hasBreadboard = thread.artifacts.some((a) => a.stage === "breadboard")
  const hasPitch = thread.artifacts.some((a) => a.stage === "pitch")
  const hasAnyPRD = threadPRDs.length > 0

  const pitchArtifact = thread.artifacts.find((a) => a.stage === "pitch")

  // If nothing has been unlocked yet (no breadboard) there's nothing to
  // render. Keeping the block silent avoids an empty "Downstream Artifacts"
  // header on fresh threads still in frame/shape.
  if (!hasBreadboard) return null

  return (
    <div ref={sectionRef} className="space-y-5">
      <div className="flex items-center gap-2 pt-2">
        <div className="h-px flex-1 bg-border" />
        <span className="text-[11px] font-bold tracking-[0.08em] uppercase text-muted-foreground">
          Downstream Artifacts
        </span>
        <div className="h-px flex-1 bg-border" />
      </div>

      {hasBreadboard && (
        <PrototypesSection
          thread={thread}
          items={threadPrototypes}
          sources={sourcesFromThread(thread)}
        />
      )}

      {hasPitch && pitchArtifact && (
        <PRDsSection
          thread={thread}
          pitchPath={pitchArtifact.path}
          items={threadPRDs}
        />
      )}

      {hasAnyPRD && (
        <UseCasesSection
          thread={thread}
          prds={threadPRDs}
          items={threadUseCases}
        />
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Section wrapper
// ---------------------------------------------------------------------------

function Section({
  title,
  icon: Icon,
  accent,
  count,
  children,
}: {
  title: string
  icon: React.ComponentType<{ className?: string }>
  accent: string
  count: number
  children: React.ReactNode
}) {
  return (
    <Card
      className="rounded-[14px] border-border shadow-stripe"
      style={{ borderColor: `${accent}30` }}
    >
      <CardContent className="p-5">
        <div className="mb-4 flex items-center gap-2">
          <span
            className="flex h-7 w-7 items-center justify-center rounded-md"
            style={{ background: `${accent}26` }}
          >
            <Icon className="h-3.5 w-3.5" />
          </span>
          <h3 className="text-[14px] font-semibold tracking-tight text-foreground">
            {title}
          </h3>
          <span className="text-[11px] text-muted-foreground tabular-nums">
            {count > 0 ? count : ""}
          </span>
        </div>
        {children}
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Prototypes section
// ---------------------------------------------------------------------------

type ProtoAction =
  | { kind: "create" }
  | { kind: "iterate"; id: string }

function PrototypesSection({
  thread,
  items,
  sources,
}: {
  thread: ShapeUpThread
  items: Prototype[]
  sources: string[]
}) {
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [format, setFormat] = useState("interactive-html")
  const [hints, setHints] = useState("")
  const [modalOpen, setModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)
  const [action, setAction] = useState<ProtoAction | null>(null)

  const createQuestionsMutation = useMutation({
    mutationFn: () =>
      api.prototypeQuestions({
        title: thread.title,
        format,
        source_paths: sources,
        hints,
      }),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch prototype questions: ${getErrorMessage(err)}`)
    },
  })

  const iterateQuestionsMutation = useMutation({
    mutationFn: (proto: Prototype) =>
      api.prototypeQuestions({
        title: proto.title,
        format: proto.format,
        source_paths: proto.source_refs ?? sources,
      }),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch iterate questions: ${getErrorMessage(err)}`)
    },
  })

  const createMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.createPrototype({
        title: thread.title,
        source_paths: sources,
        format,
        hints,
        source_thread: thread.id,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      setShowForm(false)
      setHints("")
      setQuestions(null)
      setModalOpen(false)
      setAction(null)
      toast.success("Prototype generated")
    },
    onError: (err) => {
      setQuestions(null)
      setModalOpen(false)
      setAction(null)
      toast.error(`Prototype generation failed: ${getErrorMessage(err)}`)
    },
  })

  // Q&A-based iterate — now the secondary "Need help framing?" path.
  // Carries any hints the user had already typed in the inline form so
  // Claude's questions can build on them.
  const regenerateMutation = useMutation({
    mutationFn: ({
      id,
      answers,
      hints: iterateHints,
    }: {
      id: string
      answers: Answer[]
      hints?: string
    }) =>
      api.refinePrototype(id, {
        hints: iterateHints || undefined,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      setQuestions(null)
      setModalOpen(false)
      setAction(null)
      toast.success("Prototype regenerated")
    },
    onError: (err) => {
      setQuestions(null)
      setModalOpen(false)
      setAction(null)
      toast.error(`Regenerate failed: ${getErrorMessage(err)}`)
    },
  })

  // Primary iterate path: prompt-first. User types what to change and
  // Claude refines the current prototype directly — no Q&A detour.
  // Mirrors how v0/Lovable/Cursor handle iteration on an existing
  // artifact; Q&A is an opt-in fallback via "Need help framing?".
  const inlineIterateMutation = useMutation({
    mutationFn: ({ id, hints: iterateHints }: { id: string; hints: string }) =>
      api.refinePrototype(id, { hints: iterateHints }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      toast.success("Prototype regenerated")
    },
    onError: (err) => {
      toast.error(`Regenerate failed: ${getErrorMessage(err)}`)
    },
  })

  const stageMutation = useMutation({
    mutationFn: ({ id, stage }: { id: string; stage: string }) =>
      api.setPrototypeStage(id, stage),
    onSuccess: (_r, vars) => {
      queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      toast.success(`Prototype marked ${vars.stage}`)
    },
    onError: (err) => {
      toast.error(`Stage update failed: ${getErrorMessage(err)}`)
    },
  })

  const startCreate = () => {
    setAction({ kind: "create" })
    setQuestions(null)
    setModalOpen(true)
    createQuestionsMutation.mutate()
  }

  // "Need help framing?" escape hatch: carries the hints the user
  // typed in the inline form into the Q&A flow so Claude can build on
  // them rather than starting from scratch.
  const [iterateHintsForHelp, setIterateHintsForHelp] = useState("")

  const startIterate = (proto: Prototype, pendingHints: string) => {
    setAction({ kind: "iterate", id: proto.id })
    setQuestions(null)
    setIterateHintsForHelp(pendingHints)
    setModalOpen(true)
    iterateQuestionsMutation.mutate(proto)
  }

  const submitAnswers = (answers: Answer[]) => {
    if (!action) return
    if (action.kind === "create") createMutation.mutate(answers)
    else
      regenerateMutation.mutate({
        id: action.id,
        answers,
        hints: iterateHintsForHelp,
      })
  }

  const isGenerating =
    createQuestionsMutation.isPending ||
    createMutation.isPending ||
    iterateQuestionsMutation.isPending ||
    regenerateMutation.isPending ||
    inlineIterateMutation.isPending

  return (
    <Section title="Prototypes" icon={Box} accent="#E8C547" count={items.length}>
      {items.length === 0 && !isGenerating && (
        <p className="mb-3 text-[12px] text-muted-foreground">
          No prototypes generated from this spec yet.
        </p>
      )}

      {items.length > 0 && (
        <div className="mb-3 space-y-2">
          {items.map((p) => (
            <PrototypeRow
              key={p.id}
              proto={p}
              onInlineIterate={(promptHints) =>
                inlineIterateMutation.mutate({ id: p.id, hints: promptHints })
              }
              onRequestIterateHelp={(promptHints) => startIterate(p, promptHints)}
              onMarkFinal={() =>
                stageMutation.mutate({ id: p.id, stage: "final" })
              }
              stagePending={
                stageMutation.isPending &&
                stageMutation.variables?.id === p.id
              }
              iteratePending={
                (inlineIterateMutation.isPending &&
                  inlineIterateMutation.variables?.id === p.id) ||
                ((iterateQuestionsMutation.isPending ||
                  regenerateMutation.isPending) &&
                  action?.kind === "iterate" &&
                  action.id === p.id)
              }
            />
          ))}
        </div>
      )}

      {createMutation.isPending && (
        <div className="mb-3 flex items-center gap-2.5 rounded-md border border-dashed border-border bg-muted/20 px-3 py-3">
          <Loader2 className="h-4 w-4 animate-spin text-primary" />
          <div className="flex-1 min-w-0">
            <p className="text-[13px] font-medium text-foreground">
              Generating prototype...
            </p>
            <p className="text-[11px] text-muted-foreground">
              Claude is assembling the {format} output.
            </p>
          </div>
        </div>
      )}

      {showForm ? (
        <div className="rounded-md border border-border bg-muted/30 p-3">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div className="space-y-1.5">
              <Label className="text-[11px] font-normal text-muted-foreground">
                Format
              </Label>
              <Select value={format} onValueChange={setFormat}>
                <SelectTrigger className="h-8 rounded-sm border-border text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {protoFormats.map((f) => (
                    <SelectItem key={f} value={f}>
                      {f}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label className="text-[11px] font-normal text-muted-foreground">
                Hints
              </Label>
              <Input
                value={hints}
                onChange={(e) => setHints(e.target.value)}
                className="h-8 rounded-sm border-border text-xs"
                placeholder="Optional guidance..."
              />
            </div>
          </div>
          <div className="mt-3 flex items-center gap-2">
            <Button
              size="sm"
              onClick={startCreate}
              disabled={isGenerating}
              className="h-8 rounded-sm bg-primary text-[12px] font-normal text-primary-foreground hover:bg-[var(--stripe-purple-hover)]"
            >
              {createMutation.isPending ? (
                <>
                  <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  Generating...
                </>
              ) : (
                "Generate prototype"
              )}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowForm(false)}
              className="h-8 text-[12px]"
            >
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <Button
          variant="outline"
          size="sm"
          onClick={() => setShowForm(true)}
          className="h-8 gap-1.5 rounded-sm border-dashed text-[12px] font-normal"
        >
          <Plus className="h-3.5 w-3.5" />
          {items.length > 0 ? "Generate another prototype" : "Generate prototype"}
        </Button>
      )}

      <QuestionsModal
        open={modalOpen}
        onOpenChange={(next) => {
          if (!next && !createMutation.isPending && !regenerateMutation.isPending) {
            setModalOpen(false)
            setQuestions(null)
            setAction(null)
          }
        }}
        title={
          action?.kind === "iterate"
            ? "Iterate on prototype"
            : `Generate ${format} prototype`
        }
        description={
          action?.kind === "iterate"
            ? "Answer these so Claude can refine the existing prototype."
            : "Answer these so Claude can tailor the output."
        }
        questions={questions}
        loading={
          createQuestionsMutation.isPending ||
          iterateQuestionsMutation.isPending
        }
        submitting={createMutation.isPending || regenerateMutation.isPending}
        onSubmit={submitAnswers}
        onSkip={() => submitAnswers([])}
      />
    </Section>
  )
}

// PrototypeRow is one prototype in the hub with inline preview/open,
// iterate and mark-final actions. Final stage hides Mark Final and
// shows a terminal badge instead.
//
// Iterate expands an inline prompt input ("what do you want to
// change?") rather than opening a Q&A modal. Submit sends the hints
// straight to refinePrototype. A secondary "Need help framing?" link
// falls back to the Q&A flow via onRequestIterateHelp.
function PrototypeRow({
  proto,
  onInlineIterate,
  onRequestIterateHelp,
  onMarkFinal,
  stagePending,
  iteratePending,
}: {
  proto: Prototype
  onInlineIterate: (hints: string) => void
  onRequestIterateHelp: (hints: string) => void
  onMarkFinal: () => void
  stagePending: boolean
  iteratePending: boolean
}) {
  const isFinal = proto.stage === "final"
  const [iterating, setIterating] = useState(false)
  const [iterateHints, setIterateHints] = useState("")

  const submitInline = () => {
    const trimmed = iterateHints.trim()
    if (!trimmed) return
    onInlineIterate(trimmed)
    setIterating(false)
    setIterateHints("")
  }

  return (
    <div className="rounded-md border border-border bg-card px-3 py-2.5">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <Link
            to={`/prototypes/${proto.id}`}
            className="block truncate text-[13px] font-medium text-foreground hover:text-primary"
          >
            {proto.title}
          </Link>
          <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
            <Badge
              variant="outline"
              className="rounded-sm px-1.5 text-[10px] font-normal"
            >
              {proto.format}
            </Badge>
            <Badge className="rounded-sm bg-secondary px-1.5 py-px text-[10px] font-light text-secondary-foreground">
              {proto.stage ?? proto.status}
            </Badge>
            {isFinal && (
              <Badge className="rounded-sm bg-[rgba(21,190,83,0.2)] px-1.5 py-px text-[10px] font-medium text-[var(--stripe-success-text)]">
                Final
              </Badge>
            )}
          </div>
        </div>
        <span className="flex-shrink-0 text-[11px] tabular-nums text-muted-foreground">
          {formatDate(proto.created)}
        </span>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        {proto.has_html && (
          <a
            href={api.prototypeHTMLUrl(proto.id)}
            target="_blank"
            rel="noreferrer"
            className="inline-flex h-7 items-center gap-1 rounded-sm border border-border px-2 text-[11px] font-normal text-primary hover:border-primary/60"
            onClick={(e) => e.stopPropagation()}
          >
            <ExternalLink className="h-3 w-3" />
            Preview
          </a>
        )}
        <Button
          variant="outline"
          size="sm"
          onClick={() => setIterating((v) => !v)}
          disabled={iteratePending}
          className="h-7 gap-1.5 rounded-sm text-[11px] font-normal"
        >
          {iteratePending ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : (
            <RotateCcw className="h-3 w-3" />
          )}
          Iterate
        </Button>
        {!isFinal && (
          <Button
            variant="outline"
            size="sm"
            onClick={onMarkFinal}
            disabled={stagePending}
            className="h-7 gap-1.5 rounded-sm text-[11px] font-normal text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)] hover:bg-[rgba(21,190,83,0.1)]"
          >
            {stagePending ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <CheckCircle2 className="h-3 w-3" />
            )}
            Mark Final
          </Button>
        )}
      </div>

      {iterating && (
        <div className="mt-3 rounded-md border border-dashed border-border bg-muted/20 p-2.5">
          <Label className="text-[11px] font-normal text-muted-foreground">
            What do you want to change?
          </Label>
          <Input
            autoFocus
            value={iterateHints}
            onChange={(e) => setIterateHints(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault()
                submitInline()
              } else if (e.key === "Escape") {
                e.preventDefault()
                setIterating(false)
                setIterateHints("")
              }
            }}
            placeholder="e.g. make the header larger, add a pricing section"
            className="mt-1 h-8 rounded-sm border-border text-xs"
            disabled={iteratePending}
          />
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              onClick={submitInline}
              disabled={iteratePending || !iterateHints.trim()}
              className="h-7 rounded-sm bg-primary text-[11px] font-normal text-primary-foreground hover:bg-[var(--stripe-purple-hover)]"
            >
              {iteratePending ? (
                <>
                  <Loader2 className="mr-1.5 h-3 w-3 animate-spin" />
                  Applying...
                </>
              ) : (
                "Apply"
              )}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setIterating(false)
                setIterateHints("")
              }}
              disabled={iteratePending}
              className="h-7 text-[11px] font-normal"
            >
              Cancel
            </Button>
            <button
              type="button"
              onClick={() => {
                onRequestIterateHelp(iterateHints.trim())
                setIterating(false)
              }}
              disabled={iteratePending}
              className="ml-auto text-[11px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline disabled:opacity-50"
            >
              Need help framing?
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// PRDs section
// ---------------------------------------------------------------------------

function PRDsSection({
  thread,
  pitchPath,
  items,
}: {
  thread: ShapeUpThread
  pitchPath: string
  items: PRD[]
}) {
  const queryClient = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const requestBody = {
    pitch_path: pitchPath,
    project_ids: thread.projects ?? [],
  }

  const questionsMutation = useMutation({
    mutationFn: () => api.prdQuestions(requestBody),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch PRD questions: ${getErrorMessage(err)}`)
    },
  })

  const createMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.createPRD({
        ...requestBody,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: (prd) => {
      queryClient.invalidateQueries({ queryKey: ["prds"] })
      setQuestions(null)
      setModalOpen(false)
      toast.success("PRD synthesized")
      // Auto-expand the newly-created PRD so the user sees the result
      // without having to hunt for it — this is the "inline completion"
      // affordance: the freshly-synthesised PRD pops open in place.
      if (prd?.id) setExpandedId(prd.id)
    },
    onError: (err) => {
      setQuestions(null)
      setModalOpen(false)
      toast.error(`PRD synthesis failed: ${getErrorMessage(err)}`)
    },
  })

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      api.setPRDStatus(id, status),
    onSuccess: (_prd, vars) => {
      queryClient.invalidateQueries({ queryKey: ["prds"] })
      toast.success(`PRD marked ${vars.status}`)
    },
    onError: (err) => {
      toast.error(`Status update failed: ${getErrorMessage(err)}`)
    },
  })

  // Inline hints refine — mirrors the prototype iterate pattern (#9).
  // The user types what to change; Claude rewrites the PRD with the
  // previous body as a reference. Results overwrite the live file and
  // bump the version counter; prior versions are archived.
  const refineMutation = useMutation({
    mutationFn: ({ id, hints }: { id: string; hints: string }) =>
      api.refinePRD(id, { hints }),
    onSuccess: (prd) => {
      queryClient.invalidateQueries({ queryKey: ["prds"] })
      toast.success("PRD refined")
      if (prd?.id) {
        setExpandedId(prd.id)
        // Bust the vault-file cache for this PRD's path so the
        // expanded body re-reads from disk.
        if (prd.path) {
          queryClient.invalidateQueries({ queryKey: ["vault-file", prd.path] })
        }
      }
    },
    onError: (err) => {
      toast.error(`Refine failed: ${getErrorMessage(err)}`)
    },
  })

  const startGenerate = () => {
    setQuestions(null)
    setModalOpen(true)
    questionsMutation.mutate()
  }

  const isGenerating =
    questionsMutation.isPending || createMutation.isPending

  return (
    <Section title="PRD" icon={FileCheck} accent="#48D597" count={items.length}>
      {items.length === 0 && !isGenerating && (
        <p className="mb-3 text-[12px] text-muted-foreground">
          No PRD synthesized from this pitch yet.
        </p>
      )}

      {items.length > 0 && (
        <div className="mb-3 space-y-2">
          {items.map((prd) => (
            <PRDRow
              key={prd.id}
              prd={prd}
              expanded={expandedId === prd.id}
              onToggleExpand={() =>
                setExpandedId(expandedId === prd.id ? null : prd.id)
              }
              onInlineIterate={(hints) =>
                refineMutation.mutate({ id: prd.id, hints })
              }
              onMarkFinal={() =>
                statusMutation.mutate({ id: prd.id, status: "shipped" })
              }
              statusPending={
                statusMutation.isPending &&
                statusMutation.variables?.id === prd.id
              }
              iteratePending={
                refineMutation.isPending &&
                refineMutation.variables?.id === prd.id
              }
            />
          ))}
        </div>
      )}

      {/* Inline generation indicator — shows a placeholder card while
          Claude is synthesising so the user sees visible progress in
          the hub rather than waiting on a distant button spinner. */}
      {createMutation.isPending && (
        <div className="mb-3 flex items-center gap-2.5 rounded-md border border-dashed border-border bg-muted/20 px-3 py-3">
          <Loader2 className="h-4 w-4 animate-spin text-primary" />
          <div className="flex-1 min-w-0">
            <p className="text-[13px] font-medium text-foreground">
              Synthesizing PRD...
            </p>
            <p className="text-[11px] text-muted-foreground">
              This can take 30–90 seconds depending on context size.
            </p>
          </div>
        </div>
      )}

      <Button
        variant="outline"
        size="sm"
        onClick={startGenerate}
        disabled={isGenerating}
        className="h-8 gap-1.5 rounded-sm border-dashed text-[12px] font-normal"
      >
        {isGenerating ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <Plus className="h-3.5 w-3.5" />
        )}
        {items.length > 0 ? "Generate another PRD" : "Generate PRD"}
      </Button>

      <QuestionsModal
        open={modalOpen}
        onOpenChange={(next) => {
          if (!next && !createMutation.isPending) {
            setModalOpen(false)
            setQuestions(null)
          }
        }}
        title="Synthesize PRD"
        description="Answer these so Claude can tailor the PRD to your constraints."
        questions={questions}
        loading={questionsMutation.isPending}
        submitting={createMutation.isPending}
        onSubmit={(answers) => createMutation.mutate(answers)}
        onSkip={() => createMutation.mutate([])}
      />
    </Section>
  )
}

// PRDRow is a single expandable PRD card: meta row on top, markdown
// preview + actions when expanded. Mark-final is terminal (status →
// shipped) so once set the button disappears.
//
// Iterate expands an inline prompt input ("what do you want to
// change?") rather than regenerating from scratch. Submit sends the
// hints to refinePRD, which overwrites the live file with a new
// version informed by the previous body.
function PRDRow({
  prd,
  expanded,
  onToggleExpand,
  onInlineIterate,
  onMarkFinal,
  statusPending,
  iteratePending,
}: {
  prd: PRD
  expanded: boolean
  onToggleExpand: () => void
  onInlineIterate: (hints: string) => void
  onMarkFinal: () => void
  statusPending: boolean
  iteratePending: boolean
}) {
  const [iterating, setIterating] = useState(false)
  const [iterateHints, setIterateHints] = useState("")

  const submitInline = () => {
    const trimmed = iterateHints.trim()
    if (!trimmed) return
    onInlineIterate(trimmed)
    setIterating(false)
    setIterateHints("")
  }
  const { data: rawContent, isLoading } = useQuery<string>({
    queryKey: ["vault-file", prd.path],
    queryFn: () => api.getVaultFile(prd.path!),
    enabled: expanded && !!prd.path,
    staleTime: 30_000,
  })
  const body = rawContent ? stripFrontmatter(rawContent) : null
  const isFinal = prd.status === "shipped" || prd.status === "archived"

  return (
    <div className="rounded-md border border-border bg-card">
      <button
        type="button"
        onClick={onToggleExpand}
        className="flex w-full items-center gap-2 px-3 py-2.5 text-left transition-colors hover:bg-muted/40"
      >
        {expanded ? (
          <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
        )}
        <div className="min-w-0 flex-1">
          <div className="truncate text-[13px] font-medium text-foreground">
            {prd.title}
          </div>
          <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
            <Badge
              className={cn(
                "rounded-sm px-1.5 py-px text-[10px] font-light",
                prdStatusColors[prd.status] ?? "",
              )}
            >
              {prd.status}
            </Badge>
            <span className="text-[11px] tabular-nums text-muted-foreground">
              v{prd.version}
            </span>
            {prd.open_questions_count > 0 && (
              <span className="text-[11px] text-muted-foreground">
                · {prd.open_questions_count} open question
                {prd.open_questions_count === 1 ? "" : "s"}
              </span>
            )}
          </div>
        </div>
        <span className="flex-shrink-0 text-[11px] tabular-nums text-muted-foreground">
          {formatDate(prd.created)}
        </span>
      </button>

      {expanded && (
        <div className="border-t border-border">
          <div className="max-h-[420px] overflow-y-auto px-4 py-3">
            {isLoading ? (
              <div className="flex items-center gap-2 py-6 justify-center text-[12px] text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading PRD...
              </div>
            ) : body ? (
              <div className="prose prose-sm max-w-none dark:prose-invert prose-headings:text-foreground prose-p:text-foreground/90 prose-li:text-foreground/90 prose-strong:text-foreground prose-code:text-foreground/80 prose-code:bg-muted prose-code:px-1.5 prose-code:py-0.5 prose-code:rounded">
                <Markdown>{body}</Markdown>
              </div>
            ) : (
              <p className="py-4 text-center text-[12px] italic text-muted-foreground">
                No content found for this PRD.
              </p>
            )}
          </div>
          <div className="flex items-center gap-2 border-t border-border bg-muted/20 px-3 py-2">
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => {
                e.stopPropagation()
                setIterating((v) => !v)
              }}
              disabled={iteratePending}
              className="h-7 gap-1.5 rounded-sm text-[11px] font-normal"
            >
              <RotateCcw className="h-3 w-3" />
              Iterate
            </Button>
            {!isFinal && (
              <Button
                variant="outline"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation()
                  onMarkFinal()
                }}
                disabled={statusPending}
                className="h-7 gap-1.5 rounded-sm text-[11px] font-normal text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)] hover:bg-[rgba(21,190,83,0.1)]"
              >
                {statusPending ? (
                  <Loader2 className="h-3 w-3 animate-spin" />
                ) : (
                  <CheckCircle2 className="h-3 w-3" />
                )}
                Mark Final
              </Button>
            )}
            {isFinal && (
              <Badge className="rounded-sm bg-[rgba(21,190,83,0.2)] px-2 py-0.5 text-[10px] font-medium text-[var(--stripe-success-text)]">
                Final
              </Badge>
            )}
          </div>
          {iterating && (
            <div className="mx-3 mb-3 rounded-md border border-dashed border-border bg-muted/20 p-2.5">
              <Label className="text-[11px] font-normal text-muted-foreground">
                What do you want to change?
              </Label>
              <Input
                autoFocus
                value={iterateHints}
                onChange={(e) => setIterateHints(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.shiftKey) {
                    e.preventDefault()
                    submitInline()
                  } else if (e.key === "Escape") {
                    e.preventDefault()
                    setIterating(false)
                    setIterateHints("")
                  }
                }}
                placeholder="e.g. reframe rollout around mobile first, drop analytics goal"
                className="mt-1 h-8 rounded-sm border-border text-xs"
                disabled={iteratePending}
              />
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <Button
                  size="sm"
                  onClick={submitInline}
                  disabled={iteratePending || !iterateHints.trim()}
                  className="h-7 rounded-sm bg-primary text-[11px] font-normal text-primary-foreground hover:bg-[var(--stripe-purple-hover)]"
                >
                  {iteratePending ? (
                    <>
                      <Loader2 className="mr-1.5 h-3 w-3 animate-spin" />
                      Applying...
                    </>
                  ) : (
                    "Apply"
                  )}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setIterating(false)
                    setIterateHints("")
                  }}
                  disabled={iteratePending}
                  className="h-7 text-[11px] font-normal"
                >
                  Cancel
                </Button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Use cases section
// ---------------------------------------------------------------------------

function UseCasesSection({
  thread,
  prds,
  items,
}: {
  thread: ShapeUpThread
  prds: PRD[]
  items: UseCase[]
}) {
  const queryClient = useQueryClient()
  const [prdId, setPrdId] = useState<string>(prds[0]?.id ?? "")
  const [modalOpen, setModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)

  // Reset prdId when the prds list changes and the current selection
  // is no longer valid (e.g. the selected PRD was archived). Without
  // this the Select would keep rendering a stale id while the derived
  // sourcePath silently fell back to prds[0] — a quiet inconsistency.
  useEffect(() => {
    if (prdId && !prds.find((p) => p.id === prdId)) {
      setPrdId(prds[0]?.id ?? "")
    }
  }, [prds, prdId])

  const selectedPRD = prds.find((p) => p.id === prdId) ?? prds[0]
  const sourcePath = selectedPRD?.path ?? ""

  const questionsMutation = useMutation({
    mutationFn: () =>
      api.useCaseFromDocQuestions({ source_path: sourcePath }),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch use-case questions: ${getErrorMessage(err)}`)
    },
  })

  const createMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.generateFromDoc({
        source_path: sourcePath,
        project_ids: thread.projects ?? [],
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["usecases"] })
      setQuestions(null)
      setModalOpen(false)
      toast.success("Use cases generated")
    },
    onError: (err) => {
      setQuestions(null)
      setModalOpen(false)
      toast.error(`Use-case generation failed: ${getErrorMessage(err)}`)
    },
  })

  const startGenerate = () => {
    if (!sourcePath) return
    setQuestions(null)
    setModalOpen(true)
    questionsMutation.mutate()
  }

  const canGenerate = prds.length > 0 && !!sourcePath

  return (
    <Section
      title="Use Cases"
      icon={FileText}
      accent="#47B5E8"
      count={items.length}
    >
      {items.length === 0 ? (
        <p className="mb-3 text-[12px] text-muted-foreground">
          No use cases generated from this PRD yet.
        </p>
      ) : (
        <div className="mb-3 space-y-2">
          {items.map((uc) => {
            const open = expandedId === uc.id
            return (
              <div
                key={uc.id}
                className="rounded-md border border-border bg-card"
              >
                <button
                  type="button"
                  onClick={() => setExpandedId(open ? null : uc.id)}
                  className="flex w-full items-center gap-2 px-3 py-2 text-left transition-colors hover:bg-muted/40"
                >
                  {open ? (
                    <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
                  ) : (
                    <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                  )}
                  <span className="flex-1 truncate text-[13px] font-medium text-foreground">
                    {uc.title}
                  </span>
                  <span className="text-[11px] tabular-nums text-muted-foreground">
                    {uc.main_flow?.length ?? 0} step
                    {(uc.main_flow?.length ?? 0) === 1 ? "" : "s"}
                  </span>
                </button>
                {open && (
                  <div className="border-t border-border px-3 py-2.5 space-y-2">
                    {uc.actors?.length > 0 && (
                      <div className="text-[11px] text-muted-foreground">
                        <span className="font-medium text-foreground">
                          Actors:
                        </span>{" "}
                        {uc.actors.join(", ")}
                      </div>
                    )}
                    {uc.main_flow && uc.main_flow.length > 0 && (
                      <ol className="list-decimal space-y-0.5 pl-4 text-[12px] text-foreground/80">
                        {uc.main_flow.slice(0, 5).map((step, i) => (
                          <li key={i}>{step}</li>
                        ))}
                        {uc.main_flow.length > 5 && (
                          <li className="list-none text-[11px] italic text-muted-foreground">
                            + {uc.main_flow.length - 5} more step
                            {uc.main_flow.length - 5 === 1 ? "" : "s"}
                          </li>
                        )}
                      </ol>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        {prds.length > 1 && (
          <Select value={prdId} onValueChange={setPrdId}>
            <SelectTrigger className="h-8 w-[220px] rounded-sm border-border text-xs">
              <SelectValue placeholder="Pick a PRD..." />
            </SelectTrigger>
            <SelectContent>
              {prds.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.title} · v{p.version}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        <Button
          variant="outline"
          size="sm"
          onClick={startGenerate}
          disabled={
            !canGenerate ||
            questionsMutation.isPending ||
            createMutation.isPending
          }
          className="h-8 gap-1.5 rounded-sm border-dashed text-[12px] font-normal"
        >
          {createMutation.isPending ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Plus className="h-3.5 w-3.5" />
          )}
          Generate use cases from PRD
        </Button>
      </div>

      <QuestionsModal
        open={modalOpen}
        onOpenChange={(next) => {
          if (!next && !createMutation.isPending) {
            setModalOpen(false)
            setQuestions(null)
          }
        }}
        title="Generate use cases"
        description="Answer these so Claude can extract the right scenarios from the PRD."
        questions={questions}
        loading={questionsMutation.isPending}
        submitting={createMutation.isPending}
        onSubmit={(answers) => createMutation.mutate(answers)}
        onSkip={() => createMutation.mutate([])}
      />
    </Section>
  )
}

