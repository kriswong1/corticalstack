import { useMemo, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/page-header"
import { Breadcrumbs } from "@/components/layout/breadcrumbs"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { QuestionsModal } from "@/components/questions-modal"
import { ProductSubnav } from "@/components/product-subnav"
import { PipelineStageCards } from "@/components/shared/pipeline-stage-cards"
import { PipelineItemsTable } from "@/components/shared/pipeline-items-table"
import { api, getErrorMessage } from "@/lib/api"
import { Link, useNavigate } from "react-router-dom"
import { Plus, X } from "lucide-react"
import type { Answer, PRD, Question, ShapeUpThread } from "@/types/api"

const ANY_PROJECT = "__any__"

// PRDs can only be synthesized from a thread that has reached the pitch
// stage — that's when the product arc is "complete" enough to turn into
// requirements.
function isPitchReady(t: ShapeUpThread): boolean {
  return t.current_stage === "pitch"
}

function pitchPath(t: ShapeUpThread): string {
  const pitch = t.artifacts.find((a) => a.stage === "pitch")
  return pitch?.path ?? ""
}

export function PRDsPage() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [showForm, setShowForm] = useState(false)
  const [projectFilter, setProjectFilter] = useState<string>(ANY_PROJECT)
  const [threadId, setThreadId] = useState("")
  const [extraPaths, setExtraPaths] = useState("")
  const [extraTags, setExtraTags] = useState("")
  const [modalOpen, setModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)
  // Status-axis filter driven by the stage cards at the top —
  // mirrors the dashboard-card behavior so every pipeline surface
  // feels the same.
  const [stageFilter, setStageFilter] = useState<string | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const { data: prds, isLoading } = useQuery({
    queryKey: ["prds"],
    queryFn: api.listPRDs,
  })

  const { data: threads } = useQuery({
    queryKey: ["shapeup-threads"],
    queryFn: api.listThreads,
  })

  const { data: projects } = useQuery({
    queryKey: ["projects"],
    queryFn: api.listProjects,
  })

  const pitchReady = useMemo(
    () => (threads ?? []).filter(isPitchReady),
    [threads],
  )

  // Projects that actually have at least one pitch-ready thread — those are
  // the only ones worth offering in the filter.
  const projectsWithPitches = useMemo(() => {
    const ids = new Set<string>()
    for (const t of pitchReady) {
      for (const pid of t.projects ?? []) ids.add(pid)
    }
    return (projects ?? []).filter((p) => ids.has(p.uuid))
  }, [projects, pitchReady])

  const filteredThreads = useMemo(() => {
    if (projectFilter === ANY_PROJECT) return pitchReady
    return pitchReady.filter((t) => (t.projects ?? []).includes(projectFilter))
  }, [pitchReady, projectFilter])

  const selectedThread = filteredThreads.find((t) => t.id === threadId)
  const currentPitchPath = selectedThread ? pitchPath(selectedThread) : ""

  // Memoize the request body so mutation fns don't re-derive it every
  // render (cleaner React Query DevTools, no new array identity per
  // keystroke) and so both the questions and create mutations see the
  // same snapshot when the user clicks "Synthesize".
  const requestBody = useMemo(
    () => ({
      pitch_path: currentPitchPath,
      extra_context_paths: extraPaths
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean),
      extra_context_tags: extraTags
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean),
      project_ids: selectedThread?.projects ?? [],
    }),
    [currentPitchPath, extraPaths, extraTags, selectedThread],
  )

  const questionsMutation = useMutation({
    mutationFn: () => api.prdQuestions(requestBody),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    // Pre-flight questions call — fall through to empty list so the
    // user can still synthesize, but surface a toast so they know the
    // Q&A prompt won't appear and can retry by reopening.
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["prds"] })
      setShowForm(false)
      setThreadId("")
      setExtraPaths("")
      setExtraTags("")
      setQuestions(null)
      setModalOpen(false)
      toast.success("PRD synthesized")
    },
    onError: (err) => {
      // Close the Q&A modal on failure so the user isn't stuck on a
      // frozen spinner. They can reopen the form and retry.
      setQuestions(null)
      setModalOpen(false)
      toast.error(`PRD synthesis failed: ${getErrorMessage(err)}`)
    },
  })

  const canSynthesize = !!selectedThread && currentPitchPath !== ""

  const startSynthesize = () => {
    if (!canSynthesize) return
    setQuestions(null)
    setModalOpen(true)
    questionsMutation.mutate()
  }

  const noPitchThreads = (threads ?? []).length > 0 && pitchReady.length === 0
  const noThreadsAtAll = (threads ?? []).length === 0

  return (
    <>
      <Breadcrumbs
        items={[
          { label: "Dashboard", to: "/dashboard" },
          { label: "PRDs" },
        ]}
      />
      <ProductSubnav />
      <PageHeader
        title="PRDs"
        description="Generated from pitch-complete product specs"
      >
        <Button
          onClick={() => setShowForm((prev) => !prev)}
          disabled={pitchReady.length === 0}
          title={
            pitchReady.length === 0
              ? "Advance a spec to the pitch stage first"
              : undefined
          }
          className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Plus className="h-4 w-4" /> New PRD
        </Button>
      </PageHeader>

      {(noThreadsAtAll || noPitchThreads) && (
        <Card className="mb-6 rounded-md border-border bg-muted/30">
          <CardContent className="py-4">
            <p className="text-sm text-muted-foreground">
              {noThreadsAtAll
                ? "No product specs yet — create an idea in Product and advance it to the pitch stage before generating a PRD."
                : "No specs have reached the pitch stage yet. Advance one in Product → pitch to enable PRD synthesis."}
            </p>
          </CardContent>
        </Card>
      )}

      {showForm && pitchReady.length > 0 && (
        <Card className="mb-6 rounded-md border-border shadow-stripe">
          <CardContent className="pt-6">
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                startSynthesize()
              }}
            >
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">
                    Project
                  </Label>
                  <Select
                    value={projectFilter}
                    onValueChange={(v) => {
                      setProjectFilter(v)
                      setThreadId("")
                    }}
                  >
                    <SelectTrigger className="border-border rounded-sm">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={ANY_PROJECT}>All projects</SelectItem>
                      {projectsWithPitches.map((p) => (
                        <SelectItem key={p.uuid} value={p.uuid}>
                          {p.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">
                    Pitch
                  </Label>
                  <Select value={threadId} onValueChange={setThreadId}>
                    <SelectTrigger className="border-border rounded-sm">
                      <SelectValue placeholder="Pick a pitch..." />
                    </SelectTrigger>
                    <SelectContent>
                      {filteredThreads.length === 0 ? (
                        <div className="px-2 py-1.5 text-xs text-muted-foreground">
                          No pitches in this project yet
                        </div>
                      ) : (
                        filteredThreads.map((t) => {
                          const label =
                            (t.projects ?? []).length > 0
                              ? `${t.title} · ${(t.projects ?? []).join(", ")}`
                              : t.title
                          return (
                            <SelectItem key={t.id} value={t.id}>
                              {label}
                            </SelectItem>
                          )
                        })
                      )}
                    </SelectContent>
                  </Select>
                  {selectedThread && (
                    <p className="text-[11px] text-muted-foreground font-mono">
                      {currentPitchPath}
                    </p>
                  )}
                </div>
              </div>

              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">
                  Extra Context Paths (one per line, optional)
                </Label>
                <Input
                  value={extraPaths}
                  onChange={(e) => setExtraPaths(e.target.value)}
                  className="border-border rounded-sm"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">
                  Extra Tags (comma-separated, optional)
                </Label>
                <Input
                  value={extraTags}
                  onChange={(e) => setExtraTags(e.target.value)}
                  className="border-border rounded-sm"
                />
              </div>

              <Button
                type="submit"
                disabled={
                  createMutation.isPending ||
                  questionsMutation.isPending ||
                  !canSynthesize
                }
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {createMutation.isPending ? "Synthesizing..." : "Synthesize PRD"}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <>
          {/* Status distribution — shared pipeline-stage cards.
              Clicking a card filters the items table; hover shows
              the PRD titles currently in that status. */}
          <PipelineStageCards
            type="prd"
            items={(prds ?? []).map((prd) => ({
              id: prd.id,
              title: prd.title,
              stage: prd.status,
            }))}
            selectedStage={stageFilter}
            onSelectStage={(s) => {
              setStageFilter(s)
              setSelected(new Set())
            }}
          />

          <Card className="rounded-[14px] border-border shadow-stripe mt-5">
            <CardHeader className="pb-3 flex flex-row items-center justify-between gap-3 space-y-0">
              <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
                PRDs
                {stageFilter && (
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    filtered by {stageFilter}
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
              <PRDItemsTable
                prds={prds ?? []}
                threads={threads ?? []}
                stageFilter={stageFilter}
                selected={selected}
                onToggleItem={(idv) =>
                  setSelected((prev) => {
                    const next = new Set(prev)
                    if (next.has(idv)) next.delete(idv)
                    else next.add(idv)
                    return next
                  })
                }
                onToggleAll={(ids, allSel) =>
                  setSelected((prev) => {
                    const next = new Set(prev)
                    if (allSel) for (const x of ids) next.delete(x)
                    else for (const x of ids) next.add(x)
                    return next
                  })
                }
                onView={(prd) => navigate(`/prds/${prd.id}`)}
              />
            </CardContent>
          </Card>
        </>
      )}

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
    </>
  )
}

// PRDItemsTable adapts PRD data (id, title, status, source_thread,
// version, open_questions_count, created) to the shared
// PipelineItemsTable contract. The shared component owns the base
// columns (Title / Stage / Updated) and checkboxes; this wrapper
// adds the Source Thread column that PRDs share with Prototypes.
function PRDItemsTable({
  prds,
  threads,
  stageFilter,
  selected,
  onToggleItem,
  onToggleAll,
  onView,
}: {
  prds: PRD[]
  threads: ShapeUpThread[]
  stageFilter: string | null
  selected: Set<string>
  onToggleItem: (id: string) => void
  onToggleAll: (ids: string[], allSelected: boolean) => void
  onView: (prd: PRD) => void
}) {
  const tableItems = prds
    .filter((prd) => !stageFilter || prd.status === stageFilter)
    .map((prd) => ({
      id: prd.id,
      title: prd.title,
      stage: prd.status,
      updated: prd.created,
      _prd: prd,
    }))

  const ids = tableItems.map((i) => i.id)
  const allSelected =
    tableItems.length > 0 && ids.every((id) => selected.has(id))

  return (
    <PipelineItemsTable
      type="prd"
      items={tableItems}
      selected={selected}
      onToggleItem={onToggleItem}
      onToggleAll={() => onToggleAll(ids, allSelected)}
      allSelected={allSelected}
      // "View" opens an in-place preview dialog so the rendered PRD
      // body is one click away — previously it jumped to the source
      // thread's hub, which dead-ended at /prds when a PRD had no
      // source_thread.
      onViewItem={(item) => {
        const prd = (item as typeof item & { _prd: PRD })._prd
        onView(prd)
      }}
      emptyMessage={
        stageFilter
          ? `No PRDs in the ${stageFilter} status.`
          : "No PRDs yet."
      }
      extraColumns={[
        {
          header: "Source Spec",
          cell: (item) => {
            const prd = (item as typeof item & { _prd: PRD })._prd
            const thread = prd.source_thread
              ? threads.find((t) => t.id === prd.source_thread)
              : undefined
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
      ]}
    />
  )
}
