import { useMemo, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { QuestionsModal } from "@/components/questions-modal"
import { api, getErrorMessage } from "@/lib/api"
import { Plus } from "lucide-react"
import type { Answer, Question, ShapeUpThread } from "@/types/api"

const statusColors: Record<string, string> = {
  draft: "bg-muted text-muted-foreground",
  review: "bg-secondary text-secondary-foreground",
  approved: "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)]",
  shipped: "bg-primary/20 text-primary",
  archived: "bg-muted text-muted-foreground",
}

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
  const [showForm, setShowForm] = useState(false)
  const [projectFilter, setProjectFilter] = useState<string>(ANY_PROJECT)
  const [threadId, setThreadId] = useState("")
  const [extraPaths, setExtraPaths] = useState("")
  const [extraTags, setExtraTags] = useState("")
  const [modalOpen, setModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)

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
    return (projects ?? []).filter((p) => ids.has(p.id))
  }, [projects, pitchReady])

  const filteredThreads = useMemo(() => {
    if (projectFilter === ANY_PROJECT) return pitchReady
    return pitchReady.filter((t) => (t.projects ?? []).includes(projectFilter))
  }, [pitchReady, projectFilter])

  const selectedThread = filteredThreads.find((t) => t.id === threadId)
  const currentPitchPath = selectedThread ? pitchPath(selectedThread) : ""

  const requestBody = () => ({
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
  })

  const questionsMutation = useMutation({
    mutationFn: () => api.prdQuestions(requestBody()),
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
        ...requestBody(),
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
      <PageHeader
        title="PRDs"
        description="Generated from pitch-complete product threads"
      >
        <Button
          onClick={() => setShowForm((prev) => !prev)}
          disabled={pitchReady.length === 0}
          title={
            pitchReady.length === 0
              ? "Advance a thread to the pitch stage first"
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
                ? "No product threads yet — create an idea in Product and advance it to the pitch stage before generating a PRD."
                : "No threads have reached the pitch stage yet. Advance one in Product → pitch to enable PRD synthesis."}
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
                        <SelectItem key={p.id} value={p.id}>
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
        <div className="rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Title</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Status</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Version</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Open Questions</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {prds?.map((prd) => (
                <TableRow key={prd.id}>
                  <TableCell className="text-sm font-light">{prd.title}</TableCell>
                  <TableCell>
                    <Badge className={`text-[10px] font-light rounded-sm px-1.5 py-px ${statusColors[prd.status] ?? ""}`}>
                      {prd.status}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground font-[feature-settings:'tnum']">
                    v{prd.version}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground font-[feature-settings:'tnum']">
                    {prd.open_questions_count}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground font-mono">
                    {new Date(prd.created).toLocaleDateString()}
                  </TableCell>
                </TableRow>
              ))}
              {prds?.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-sm text-muted-foreground py-8">No PRDs yet.</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
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
