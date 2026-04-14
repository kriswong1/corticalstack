import { useMemo, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useSearchParams } from "react-router-dom"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
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
import { Plus, ArrowRight, X } from "lucide-react"
import type {
  Answer,
  Artifact,
  Question,
  ShapeUpThread,
} from "@/types/api"

const stages = ["raw", "frame", "shape", "breadboard", "pitch"]

// STALLED_MS matches the backend's dashboard.StalledThreshold (7 days).
// A thread is stalled when its current-stage artifact's created time
// is older than this window — meaning the idea has sat in the current
// stage without advancing. Dashboard Product Pipeline badges deep-link
// here with /product?stage=X&stalled=true.
const STALLED_MS = 7 * 24 * 60 * 60 * 1000

// isThreadStalled returns true when the artifact at the thread's current
// stage is older than the stalled threshold. Returns false if the thread
// has no artifact matching its current stage (defensive — that shouldn't
// happen in normal data but we don't want to crash on it).
function isThreadStalled(t: ShapeUpThread, now: number): boolean {
  const current = t.artifacts.find((a) => a.stage === t.current_stage)
  if (!current) return false
  const created = new Date(current.created).getTime()
  if (!isFinite(created)) return false
  return now - created >= STALLED_MS
}

const stageColors: Record<string, string> = {
  raw: "bg-muted text-muted-foreground",
  frame: "bg-secondary text-secondary-foreground",
  shape: "bg-primary/20 text-primary",
  breadboard: "bg-primary/30 text-primary",
  pitch: "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)]",
}

export function ProductPage() {
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const [showForm, setShowForm] = useState(false)
  const [title, setTitle] = useState("")
  const [content, setContent] = useState("")
  const [projectIds, setProjectIds] = useState("")

  const stageFilter = searchParams.get("stage")
  const stalledFilter = searchParams.get("stalled") === "true"
  const hasFilter = !!stageFilter || stalledFilter

  const clearFilter = () => {
    const next = new URLSearchParams(searchParams)
    next.delete("stage")
    next.delete("stalled")
    setSearchParams(next, { replace: true })
  }

  const { data: threads, isLoading } = useQuery({
    queryKey: ["shapeup-threads"],
    queryFn: api.listThreads,
  })

  // Apply URL filters client-side. Cheaper than a new API endpoint since
  // thread counts are tiny and the list is already loaded.
  const visibleThreads = useMemo(() => {
    if (!threads) return threads
    if (!hasFilter) return threads
    const now = Date.now()
    return threads.filter((t) => {
      if (stageFilter && t.current_stage !== stageFilter) return false
      if (stalledFilter && !isThreadStalled(t, now)) return false
      return true
    })
  }, [threads, stageFilter, stalledFilter, hasFilter])

  const createMutation = useMutation({
    mutationFn: api.createIdea,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["shapeup-threads"] })
      setShowForm(false)
      setTitle("")
      setContent("")
      setProjectIds("")
      toast.success("Idea created")
    },
    onError: (err) => {
      toast.error(`Create idea failed: ${getErrorMessage(err)}`)
    },
  })

  return (
    <>
      <PageHeader title="Product" description="ShapeUp pipeline — Raw to Pitch">
        <Button
          onClick={() => setShowForm(!showForm)}
          className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
        >
          <Plus className="h-4 w-4" /> New Idea
        </Button>
      </PageHeader>

      {showForm && (
        <Card className="mb-6 rounded-md border-border shadow-stripe">
          <CardContent className="pt-6">
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (!title.trim() || !content.trim()) return
                createMutation.mutate({
                  title,
                  content,
                  project_ids: projectIds.split("\n").map((s) => s.trim()).filter(Boolean),
                })
              }}
            >
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Title</Label>
                  <Input value={title} onChange={(e) => setTitle(e.target.value)} className="border-border rounded-sm" />
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Projects (one per line)</Label>
                  <Input value={projectIds} onChange={(e) => setProjectIds(e.target.value)} className="border-border rounded-sm" />
                </div>
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Content</Label>
                <Textarea value={content} onChange={(e) => setContent(e.target.value)} rows={4} className="border-border rounded-sm" />
              </div>
              <Button
                type="submit"
                disabled={createMutation.isPending}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {createMutation.isPending ? "Creating..." : "Create Idea"}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {hasFilter && (
        <div className="mb-3 flex items-center gap-2 rounded-sm border border-border bg-muted/30 px-3 py-2">
          <span className="text-xs text-muted-foreground">Filtered:</span>
          {stageFilter && (
            <span className="rounded-sm bg-secondary px-1.5 py-0.5 text-[11px] font-normal text-secondary-foreground">
              stage: {stageFilter}
            </span>
          )}
          {stalledFilter && (
            <span className="rounded-sm bg-[var(--stripe-lemon)]/20 px-1.5 py-0.5 text-[11px] font-normal text-[var(--stripe-lemon)]">
              stalled &gt; 7 days
            </span>
          )}
          <span className="ml-2 text-xs text-muted-foreground">
            {visibleThreads?.length ?? 0} of {threads?.length ?? 0}
          </span>
          <button
            onClick={clearFilter}
            className="ml-auto inline-flex items-center gap-1 text-xs text-primary hover:underline"
          >
            <X className="h-3 w-3" /> Clear
          </button>
        </div>
      )}

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <div className="space-y-4">
          {visibleThreads?.map((thread) => (
            <ThreadCard key={thread.id} thread={thread} />
          ))}
          {visibleThreads?.length === 0 && (
            <p className="text-sm text-muted-foreground">
              {hasFilter
                ? "No threads match the current filter."
                : "No threads yet. Create an idea to start."}
            </p>
          )}
        </div>
      )}
    </>
  )
}

function ThreadCard({ thread }: { thread: ShapeUpThread }) {
  const queryClient = useQueryClient()
  const [targetStage, setTargetStage] = useState("")
  const [hints, setHints] = useState("")
  const [modalOpen, setModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)

  const questionsMutation = useMutation({
    mutationFn: (stage: string) => api.shapeupQuestions(thread.id, stage),
    onSuccess: (resp) => {
      setQuestions(resp.questions ?? [])
    },
    onError: (err) => {
      // If asking fails, fall through to no Q&A so the user can still
      // advance, but surface a toast so they know the prompt was
      // skipped for a reason.
      setQuestions([])
      toast.error(`Failed to fetch advance questions: ${getErrorMessage(err)}`)
    },
  })

  const advanceMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.advanceThread(thread.id, {
        target_stage: targetStage,
        hints,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: async (newArtifact: Artifact) => {
      queryClient.setQueryData<ShapeUpThread[]>(["shapeup-threads"], (old) => {
        if (!old) return old
        return old.map((t) =>
          t.id === thread.id
            ? { ...t, current_stage: newArtifact.stage, artifacts: [...t.artifacts, newArtifact] }
            : t,
        )
      })
      await queryClient.refetchQueries({ queryKey: ["shapeup-threads"] })
      setTargetStage("")
      setHints("")
      setQuestions(null)
      setModalOpen(false)
      toast.success(`Advanced to ${newArtifact.stage}`)
    },
    onError: (err) => {
      // Close the Q&A modal on failure so the user isn't stuck staring
      // at a frozen "Advancing..." spinner. Retry by reopening the
      // advance form.
      setQuestions(null)
      setModalOpen(false)
      toast.error(`Advance failed: ${getErrorMessage(err)}`)
    },
  })

  const startAdvance = () => {
    if (!targetStage) return
    setQuestions(null)
    setModalOpen(true)
    questionsMutation.mutate(targetStage)
  }

  const currentIdx = stages.indexOf(thread.current_stage)
  const nextStages = stages.slice(currentIdx + 1)

  return (
    <Card className="rounded-md border-border shadow-stripe-elevated">
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base font-light text-foreground">{thread.title}</CardTitle>
          <Badge className={`text-[10px] font-light rounded-sm px-1.5 py-px ${stageColors[thread.current_stage] ?? ""}`}>
            {thread.current_stage}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-1 mb-3">
          {stages.map((s, i) => (
            <span key={s} className="flex items-center gap-1">
              <span className={`text-xs ${stages.indexOf(thread.current_stage) >= i ? "text-primary font-normal" : "text-muted-foreground font-light"}`}>
                {s}
              </span>
              {i < stages.length - 1 && <ArrowRight className="h-3 w-3 text-muted-foreground" />}
            </span>
          ))}
        </div>

        {thread.projects && thread.projects.length > 0 && (
          <div className="flex flex-wrap gap-1 mb-3">
            {thread.projects.map((pid) => (
              <Badge key={pid} variant="outline" className="text-[10px] font-normal rounded-sm px-1">
                {pid}
              </Badge>
            ))}
          </div>
        )}

        <p className="text-xs text-muted-foreground mb-3">
          {thread.artifacts.length} artifact(s)
        </p>

        {nextStages.length > 0 && (
          <div className="flex items-end gap-3 border-t border-border pt-3">
            <div className="space-y-1">
              <Label className="text-xs text-[var(--stripe-label)]">Advance to</Label>
              <Select value={targetStage} onValueChange={setTargetStage}>
                <SelectTrigger className="h-7 w-32 text-xs border-border rounded-sm">
                  <SelectValue placeholder="Stage..." />
                </SelectTrigger>
                <SelectContent>
                  {nextStages.map((s) => (
                    <SelectItem key={s} value={s}>{s}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex-1 space-y-1">
              <Label className="text-xs text-[var(--stripe-label)]">Hints</Label>
              <Input
                value={hints}
                onChange={(e) => setHints(e.target.value)}
                placeholder="Optional guidance..."
                className="h-7 text-xs border-border rounded-sm"
              />
            </div>
            <Button
              size="sm"
              onClick={startAdvance}
              disabled={!targetStage || advanceMutation.isPending || questionsMutation.isPending}
              className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-xs h-7"
            >
              {advanceMutation.isPending ? "Advancing..." : "Advance"}
            </Button>
          </div>
        )}
      </CardContent>

      <QuestionsModal
        open={modalOpen}
        onOpenChange={(next) => {
          if (!next && !advanceMutation.isPending) {
            setModalOpen(false)
            setQuestions(null)
          }
        }}
        title={`Advance to ${targetStage}`}
        description="Answer these so Claude can produce a better draft."
        questions={questions}
        loading={questionsMutation.isPending}
        submitting={advanceMutation.isPending}
        onSubmit={(answers) => advanceMutation.mutate(answers)}
        onSkip={() => advanceMutation.mutate([])}
      />
    </Card>
  )
}
