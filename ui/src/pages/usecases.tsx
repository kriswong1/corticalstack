import { useMemo, useRef, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/page-header"
import { Breadcrumbs } from "@/components/layout/breadcrumbs"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { QuestionsModal } from "@/components/questions-modal"
import { ProductSubnav } from "@/components/product-subnav"
import { PipelineStageCards } from "@/components/shared/pipeline-stage-cards"
import { PipelineItemsTable } from "@/components/shared/pipeline-items-table"
import { Link } from "react-router-dom"
import { Plus, X } from "lucide-react"
import { api, getErrorMessage } from "@/lib/api"
import type { Answer, Question, PRD, ShapeUpThread, UseCase } from "@/types/api"
import { ProjectFilter, useProjectFilter } from "@/components/filters/project-filter"

type FlowKind = "doc" | "text"

// Use cases have no backend stage field, so the stage-card axis groups
// by primary actor — the first entry in `actors[]`. Actors are always
// present (generator guarantees a value) and usually a small set
// (Customer, Admin, System...), so they make a natural filter axis
// that mirrors how stages work for other pipelines.
const UNASSIGNED_ACTOR = "__unassigned__"

// Reusable palette for data-driven stages (no canonical stageColors
// entry for dynamic actor names). A stable hash of the actor string
// picks a color so the same actor keeps the same color across reloads.
const actorPalette = [
  "#9B8AFF", // purple
  "#47B5E8", // sky
  "#E85B9B", // pink
  "#48D597", // green
  "#E8C547", // amber
  "#F97316", // orange
  "#14B8A6", // teal
  "#EC4899", // rose
  "#8B5CF6", // violet
  "#10B981", // emerald
]

function hashActor(actor: string): number {
  let h = 0
  for (let i = 0; i < actor.length; i++) {
    h = (h * 31 + actor.charCodeAt(i)) | 0
  }
  return Math.abs(h)
}

function actorColor(actor: string): string {
  if (actor === UNASSIGNED_ACTOR) return "#8B8FA3"
  return actorPalette[hashActor(actor) % actorPalette.length]
}

function actorLabel(actor: string): string {
  if (actor === UNASSIGNED_ACTOR) return "Unassigned"
  return actor
}

function primaryActor(uc: UseCase): string {
  return uc.actors?.[0]?.trim() || UNASSIGNED_ACTOR
}

export function UseCasesPage() {
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [sourcePath, setSourcePath] = useState("")
  const [docHint, setDocHint] = useState("")
  const [description, setDescription] = useState("")
  const [actorsHint, setActorsHint] = useState("")
  const [modalOpen, setModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)
  const [activeFlow, setActiveFlow] = useState<FlowKind>("text")
  // Mirror `activeFlow` into a ref so `submit` and the QuestionsModal
  // callbacks always read the current value, even if the user opens
  // one flow, closes the modal, opens the other flow, and submits
  // before the first mutation settles. Without the ref the closure
  // would capture whichever value was current at the render when the
  // mutation was kicked off.
  const activeFlowRef = useRef<FlowKind>("text")
  activeFlowRef.current = activeFlow

  // Actor-axis filter driven by the stage cards. Mirrors the
  // dashboard-card behavior so every pipeline surface feels the same.
  const [stageFilter, setStageFilter] = useState<string | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  // Page-level project filter — URL ?project=<uuid>.
  const [projectFilter, setProjectFilter] = useProjectFilter()

  const { data: useCases, isLoading } = useQuery({
    queryKey: ["usecases"],
    queryFn: api.listUseCases,
  })

  // Fetched to resolve use-case source paths back to the owning product
  // thread (if the source is a PRD, we link via the PRD's source_thread;
  // if the source is a pitch/shape/breadboard artifact, we match on the
  // artifact path directly).
  const { data: prds } = useQuery<PRD[]>({
    queryKey: ["prds"],
    queryFn: api.listPRDs,
  })
  const { data: threads } = useQuery<ShapeUpThread[]>({
    queryKey: ["shapeup-threads"],
    queryFn: api.listThreads,
  })

  // Build a path → thread lookup covering both PRD paths and artifact
  // paths so source-path resolution is a single map hit per row.
  const pathToThread = useMemo(() => {
    const m = new Map<string, ShapeUpThread>()
    for (const t of threads ?? []) {
      for (const a of t.artifacts) {
        if (a.path) m.set(a.path, t)
      }
    }
    for (const prd of prds ?? []) {
      if (!prd.path || !prd.source_thread) continue
      const t = (threads ?? []).find((x) => x.id === prd.source_thread)
      if (t) m.set(prd.path, t)
    }
    return m
  }, [threads, prds])

  // Derive the ordered list of actor "stages" from the data so the
  // stage-cards row expands as new actors appear. Actors are sorted
  // alphabetically with "Unassigned" always last for predictability.
  const actorStages = useMemo(() => {
    const set = new Set<string>()
    for (const uc of useCases ?? []) set.add(primaryActor(uc))
    const list = [...set].filter((a) => a !== UNASSIGNED_ACTOR).sort()
    if (set.has(UNASSIGNED_ACTOR)) list.push(UNASSIGNED_ACTOR)
    return list
  }, [useCases])

  const stageItems = useMemo(
    () =>
      (useCases ?? []).map((uc) => ({
        id: uc.id,
        title: uc.title,
        stage: primaryActor(uc),
      })),
    [useCases],
  )

  const questionsMutation = useMutation({
    mutationFn: (kind: FlowKind) =>
      kind === "doc"
        ? api.useCaseFromDocQuestions({ source_path: sourcePath, hint: docHint })
        : api.useCaseFromTextQuestions({ description, actors_hint: actorsHint }),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch use-case questions: ${getErrorMessage(err)}`)
    },
  })

  const fromDocMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.generateFromDoc({
        source_path: sourcePath,
        hint: docHint,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["usecases"] })
      setSourcePath("")
      setDocHint("")
      setQuestions(null)
      setModalOpen(false)
      setShowForm(false)
      toast.success("Use cases generated")
    },
    onError: (err) => {
      setQuestions(null)
      setModalOpen(false)
      toast.error(`Use-case generation failed: ${getErrorMessage(err)}`)
    },
  })

  const fromTextMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.generateFromText({
        description,
        actors_hint: actorsHint,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["usecases"] })
      setDescription("")
      setActorsHint("")
      setQuestions(null)
      setModalOpen(false)
      setShowForm(false)
      toast.success("Use cases generated")
    },
    onError: (err) => {
      setQuestions(null)
      setModalOpen(false)
      toast.error(`Use-case generation failed: ${getErrorMessage(err)}`)
    },
  })

  const startDoc = () => {
    if (!sourcePath.trim()) return
    setActiveFlow("doc")
    setQuestions(null)
    setModalOpen(true)
    questionsMutation.mutate("doc")
  }

  const startText = () => {
    if (!description.trim()) return
    setActiveFlow("text")
    setQuestions(null)
    setModalOpen(true)
    questionsMutation.mutate("text")
  }

  const submit = (answers: Answer[]) => {
    if (activeFlowRef.current === "doc") fromDocMutation.mutate(answers)
    else fromTextMutation.mutate(answers)
  }

  const submitting =
    activeFlow === "doc"
      ? fromDocMutation.isPending
      : fromTextMutation.isPending

  // Filtered items and selection bookkeeping for the shared table.
  // Project filter applies first, then the actor (stage) filter.
  const visibleItems = (useCases ?? [])
    .filter((uc) =>
      projectFilter ? (uc.projects ?? []).includes(projectFilter) : true,
    )
    .filter((uc) =>
      stageFilter ? primaryActor(uc) === stageFilter : true,
    )
  const visibleIds = visibleItems.map((uc) => uc.id)
  const allSelected =
    visibleItems.length > 0 && visibleIds.every((id) => selected.has(id))

  function toggleItem(id: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleAll() {
    setSelected((prev) => {
      const next = new Set(prev)
      if (allSelected) for (const id of visibleIds) next.delete(id)
      else for (const id of visibleIds) next.add(id)
      return next
    })
  }

  return (
    <>
      <Breadcrumbs
        items={[
          { label: "Dashboard", to: "/dashboard" },
          { label: "Use Cases" },
        ]}
      />
      <ProductSubnav />
      <PageHeader
        title="Use Cases"
        description="Generated use case specifications"
      >
        <Button
          onClick={() => setShowForm(!showForm)}
          className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
        >
          {showForm ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
          {showForm ? "Cancel" : "New Use Case"}
        </Button>
      </PageHeader>

      {showForm && (
        <Card className="mb-5 rounded-[14px] border-border shadow-stripe">
          <CardContent className="pt-6">
            <Tabs defaultValue="from-text">
              <TabsList className="mb-4">
                <TabsTrigger value="from-text">From Text</TabsTrigger>
                <TabsTrigger value="from-doc">From Document</TabsTrigger>
              </TabsList>
              <TabsContent value="from-text">
                <form
                  className="space-y-3"
                  onSubmit={(e) => {
                    e.preventDefault()
                    startText()
                  }}
                >
                  <div className="space-y-2">
                    <Label className="text-[var(--stripe-label)] text-sm font-normal">
                      Description
                    </Label>
                    <Textarea
                      value={description}
                      onChange={(e) => setDescription(e.target.value)}
                      rows={3}
                      className="border-border rounded-sm"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label className="text-[var(--stripe-label)] text-sm font-normal">
                      Actors Hint
                    </Label>
                    <Input
                      value={actorsHint}
                      onChange={(e) => setActorsHint(e.target.value)}
                      className="border-border rounded-sm"
                    />
                  </div>
                  <Button
                    type="submit"
                    disabled={
                      fromTextMutation.isPending ||
                      questionsMutation.isPending ||
                      !description.trim()
                    }
                    className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
                  >
                    {fromTextMutation.isPending ? "Generating..." : "Generate"}
                  </Button>
                </form>
              </TabsContent>
              <TabsContent value="from-doc">
                <form
                  className="space-y-3"
                  onSubmit={(e) => {
                    e.preventDefault()
                    startDoc()
                  }}
                >
                  <div className="space-y-2">
                    <Label className="text-[var(--stripe-label)] text-sm font-normal">
                      Source Path
                    </Label>
                    <Input
                      value={sourcePath}
                      onChange={(e) => setSourcePath(e.target.value)}
                      placeholder="notes/..."
                      className="border-border rounded-sm"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label className="text-[var(--stripe-label)] text-sm font-normal">
                      Hint
                    </Label>
                    <Input
                      value={docHint}
                      onChange={(e) => setDocHint(e.target.value)}
                      className="border-border rounded-sm"
                    />
                  </div>
                  <Button
                    type="submit"
                    disabled={
                      fromDocMutation.isPending ||
                      questionsMutation.isPending ||
                      !sourcePath.trim()
                    }
                    className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
                  >
                    {fromDocMutation.isPending ? "Generating..." : "Generate"}
                  </Button>
                </form>
              </TabsContent>
            </Tabs>
          </CardContent>
        </Card>
      )}

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <>
          {/* Primary-actor distribution — stage cards grouped by the
              first actor on each use case. Clicking a card filters
              the items table below. */}
          <PipelineStageCards
            type="usecase"
            items={stageItems}
            stages={actorStages}
            labelFor={actorLabel}
            colorFor={actorColor}
            accent="#9B8AFF"
            selectedStage={stageFilter}
            onSelectStage={(s) => {
              setStageFilter(s)
              setSelected(new Set())
            }}
          />

          <div className="flex items-center gap-3 mt-4">
            <span className="text-xs text-muted-foreground">Filter:</span>
            <ProjectFilter
              value={projectFilter}
              onChange={setProjectFilter}
              className="h-7 w-48 text-xs border-border rounded-sm"
            />
          </div>

          <Card className="rounded-[14px] border-border shadow-stripe mt-5">
            <CardHeader className="pb-3 flex flex-row items-center justify-between gap-3 space-y-0">
              <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
                Use Cases
                {stageFilter && (
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    filtered by {actorLabel(stageFilter)}
                  </span>
                )}
                {selected.size > 0 && (
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    · {selected.size} selected
                  </span>
                )}
              </CardTitle>
              {stageFilter && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setStageFilter(null)}
                  className="h-7 px-2 text-xs gap-1"
                >
                  <X className="h-3 w-3" /> Clear filter
                </Button>
              )}
            </CardHeader>
            <CardContent className="p-0">
              <PipelineItemsTable
                type="usecase"
                items={visibleItems.map((uc) => ({
                  id: uc.id,
                  title: uc.title,
                  stage: primaryActor(uc),
                  updated: uc.created,
                  _uc: uc,
                }))}
                selected={selected}
                onToggleItem={toggleItem}
                onToggleAll={toggleAll}
                allSelected={allSelected}
                // Use cases don't have detail pages yet — dead-end the
                // View link at the use cases list so users don't get a
                // 404. When detail pages land, wire this via routeFor.
                viewLinkFor={() => "/usecases"}
                colorForStage={actorColor}
                labelForStage={actorLabel}
                emptyMessage={
                  (useCases ?? []).length === 0
                    ? "No use cases yet."
                    : `No use cases for ${actorLabel(stageFilter ?? "")}.`
                }
                extraColumns={[
                  {
                    header: "Source Spec",
                    cell: (item) => {
                      const uc = (item as typeof item & { _uc: UseCase })._uc
                      const thread = uc.source
                        ?.map((s) => (s.path ? pathToThread.get(s.path) : undefined))
                        .find(Boolean)
                      if (!thread) {
                        return <span className="text-xs text-muted-foreground">—</span>
                      }
                      return (
                        <Link
                          to={`/product/${thread.id}`}
                          className="text-xs text-primary hover:underline"
                        >
                          {thread.title}
                        </Link>
                      )
                    },
                  },
                  {
                    header: "Steps",
                    cell: (item) => {
                      const uc = (item as typeof item & { _uc: UseCase })._uc
                      return (
                        <span className="text-xs text-muted-foreground tabular-nums">
                          {uc.main_flow?.length ?? 0}
                        </span>
                      )
                    },
                  },
                  {
                    header: "Tags",
                    cell: (item) => {
                      const uc = (item as typeof item & { _uc: UseCase })._uc
                      if (!uc.tags?.length) {
                        return <span className="text-xs text-muted-foreground">—</span>
                      }
                      return (
                        <div className="flex flex-wrap gap-1">
                          {uc.tags.slice(0, 3).map((tag) => (
                            <Badge
                              key={tag}
                              variant="outline"
                              className="text-[10px] font-normal rounded-sm px-1"
                            >
                              {tag}
                            </Badge>
                          ))}
                          {uc.tags.length > 3 && (
                            <span className="text-[10px] text-muted-foreground">
                              +{uc.tags.length - 3}
                            </span>
                          )}
                        </div>
                      )
                    },
                  },
                ]}
              />
            </CardContent>
          </Card>
        </>
      )}

      <QuestionsModal
        open={modalOpen}
        onOpenChange={(next) => {
          if (!next && !submitting) {
            setModalOpen(false)
            setQuestions(null)
          }
        }}
        title="Generate use cases"
        description="Answer these so Claude can extract the right scenarios."
        questions={questions}
        loading={questionsMutation.isPending}
        submitting={submitting}
        onSubmit={submit}
        onSkip={() => submit([])}
      />
    </>
  )
}
